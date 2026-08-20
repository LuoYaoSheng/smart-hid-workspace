package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"smart-hid-controlhub/internal/device"
	"smart-hid-controlhub/internal/storage"
)

// CommandTopic / AckTopic / StatusTopic 构造 MQTT topic。
// 镜像 TS SMART_HID_TOPIC 模板（严禁 retained command）。
func CommandTopic(deviceID string) string {
	return fmt.Sprintf("smart-hid/v1/devices/%s/command", deviceID)
}
func AckTopic(deviceID string) string {
	return fmt.Sprintf("smart-hid/v1/devices/%s/ack", deviceID)
}
func StatusTopic(deviceID string) string {
	return fmt.Sprintf("smart-hid/v1/devices/%s/status", deviceID)
}

// deviceFromAckTopic 从 ack topic 提取 device_id；前缀/格式不符返回 ""。
func deviceFromAckTopic(topic string) string {
	const prefix = "smart-hid/v1/devices/"
	const suffix = "/ack"
	if !strings.HasPrefix(topic, prefix) || !strings.HasSuffix(topic, suffix) {
		return ""
	}
	dev := strings.TrimSuffix(strings.TrimPrefix(topic, prefix), suffix)
	if dev == "" || strings.Contains(dev, "/") {
		return ""
	}
	return dev
}

// SendError 是 Send 的失败返回，Kind 决定 HTTP 映射：
// validation_failed→400 / request_id_conflict→409 / internal→500。
type SendError struct {
	Kind    string
	Message string
	Fields  []*ValidationError
}

func (e *SendError) Error() string {
	if e.Message != "" {
		return e.Kind + ": " + e.Message
	}
	return e.Kind
}

const (
	ErrKindValidation = "validation_failed"
	ErrKindConflict   = "request_id_conflict"
	ErrKindInternal   = "internal"
)

func validationErr(fields ...*ValidationError) *SendError {
	return &SendError{Kind: ErrKindValidation, Fields: fields}
}

// execution 是一个 request_id 的服务端执行生命周期（幂等注册表项）。
// 同 request_id 在任意时刻至多存在一个 execution：后到的相同指纹请求
// join 等待同一结果；不同指纹请求得到 request_id_conflict。
type execution struct {
	fingerprint string
	deviceID    string

	done    chan struct{} // finish 时关闭（恰好一次）
	mu      sync.Mutex    // 保护以下字段
	result  *SmartHidAck  // 终态 ACK（nil = 以非终态收尾，即 202 语义）
	settled bool
}

// finish 幂等收尾：第一个调用者生效。ack=nil 表示未获终态（202 语义）。
func (e *execution) finish(ack *SmartHidAck) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.settled {
		return
	}
	e.result = ack
	e.settled = true
	close(e.done)
}

// snapshot 必须在 done 关闭后调用（happens-before 保证可见性）。
func (e *execution) snapshot() (ack *SmartHidAck, terminal bool) {
	return e.result, e.result != nil
}

// Engine 是 Command Engine：接收 HTTP 请求 → 校验 → 幂等登记 → publish → 等 ACK。
type Engine struct {
	mqttClient pahomqtt.Client
	devices    *device.Manager
	db         *storage.Store
	log        *slog.Logger
	ackObs     func(*SmartHidAck) // 可选：终态 ACK 观察者（实时事件通道用；nil = 无）

	mu    sync.Mutex
	execs map[string]*execution // request_id -> 在途执行（终态/超时后移除）
}

// New 构造 Engine。
func New(client pahomqtt.Client, dm *device.Manager, db *storage.Store, log *slog.Logger) *Engine {
	return &Engine{
		mqttClient: client,
		devices:    dm,
		db:         db,
		log:        log,
		execs:      make(map[string]*execution),
	}
}

// WithAckObserver 注入终态 ACK 观察者（每条终态 ACK 调用一次；不得阻塞）。
func (e *Engine) WithAckObserver(fn func(*SmartHidAck)) *Engine {
	e.ackObs = fn
	return e
}

// Fingerprint 计算 request_id 的幂等指纹：device_id + type + action + canonical payload。
// payload 经 JSON 解码为 map 后重新序列化，Go 对 map 按键排序输出 → 键序无关；
// 数组顺序保留（hotkey 键序有语义）。ttl_ms/target_boot_id 不参与（重试允许调整时序参数）。
func Fingerprint(cmd *SmartHidCommand) string {
	payloadJSON := "null"
	if cmd.Payload != nil {
		if b, err := json.Marshal(cmd.Payload); err == nil {
			payloadJSON = string(b)
		}
	}
	sum := sha256.Sum256([]byte(cmd.DeviceID + "|" + string(cmd.Type) + "|" + cmd.Action + "|" + payloadJSON))
	return hex.EncodeToString(sum[:])
}

// isTerminalCommandStatus DB status 字符串是否为终态。
func isTerminalCommandStatus(status string) bool {
	switch AckStatus(status) {
	case AckExecuted, AckRejected, AckExpired, AckDuplicate:
		return true
	}
	return false
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// HandleAck 是订阅 ack topic 的回调：校验 ACK 绑定关系 → 唤醒等待者 → 持久化终态。
//
// 信任边界：ACK 不得仅凭 request_id 被认领。必须同时满足
// topic device_id == ack.device_id == 注册 execution 的期望 device_id（三方一致），
// 且 protocol/status 合法；否则记 warning 并丢弃（不落库、不唤醒任何等待者）。
func (e *Engine) HandleAck(_ pahomqtt.Client, msg pahomqtt.Message) {
	topicDev := deviceFromAckTopic(msg.Topic())
	var ack SmartHidAck
	if err := json.Unmarshal(msg.Payload(), &ack); err != nil {
		e.log.Warn("ack unmarshal failed", "err", err, "topic", msg.Topic())
		return
	}
	if topicDev == "" {
		e.log.Warn("ack topic malformed, dropped", "topic", msg.Topic())
		return
	}
	if ack.RequestID == "" {
		e.log.Warn("ack missing request_id, dropped", "topic", msg.Topic())
		return
	}
	if ack.DeviceID != topicDev {
		e.log.Warn("ack device_id != topic device, dropped",
			"topic_device", topicDev, "ack_device", ack.DeviceID, "request_id", ack.RequestID)
		return
	}
	if ack.Protocol != ProtocolVersion {
		e.log.Warn("ack protocol mismatch, dropped", "protocol", ack.Protocol, "request_id", ack.RequestID)
		return
	}
	if !isValidAckStatus(ack.Status) {
		e.log.Warn("ack invalid status, dropped", "status", ack.Status, "request_id", ack.RequestID)
		return
	}

	e.mu.Lock()
	ex := e.execs[ack.RequestID]
	e.mu.Unlock()
	if ex != nil && ex.deviceID != ack.DeviceID {
		// 注册中的 execution 期望另一台设备：三方绑定失败，拒绝认领。
		e.log.Warn("ack from unexpected device for in-flight request, dropped",
			"expected_device", ex.deviceID, "ack_device", ack.DeviceID, "request_id", ack.RequestID)
		return
	}

	e.log.Debug("ack received", "request_id", ack.RequestID, "status", ack.Status, "code", ack.Code, "exec_ms", ack.ExecutionMs)

	if ack.Status.IsTerminal() {
		// 持久化终态（等待者可能已超时离开 —— 落库供后续重放/查询）。
		if _, err := e.db.DB.Exec(
			`UPDATE commands SET status=?, code=?, execution_ms=?, acked_at=? WHERE request_id=?`,
			string(ack.Status), ack.Code, ack.ExecutionMs, time.Now().Unix(), ack.RequestID,
		); err != nil {
			e.log.Error("persist terminal ack failed", "err", err, "request_id", ack.RequestID)
		}
		if e.ackObs != nil {
			e.ackObs(&ack)
		}
		if ex != nil {
			ex.finish(&ack)
		}
		return
	}
	// 中间态：不落库、不收尾，仅调试日志。
	e.log.Debug("intermediate ack ignored", "request_id", ack.RequestID, "status", ack.Status)
}

func isValidAckStatus(s AckStatus) bool {
	switch s {
	case AckReceived, AckExecuting, AckExecuted, AckRejected, AckExpired, AckDuplicate:
		return true
	}
	return false
}

// Send 执行闭环：校验 → 幂等登记（join / conflict / replay）→ 设备检查 →
// 持久化（错误不再静默）→ publish → 等 ACK。
//
// 返回 (ack, terminal, err)：
//   - err == nil 且 terminal == true：终态结果（executed/rejected/expired/duplicate）
//   - err == nil 且 terminal == false：命令已 publish 但 TTL 内无终态（HTTP 202 语义）
//   - err != nil：SendError（Kind 决定 HTTP 映射）
func (e *Engine) Send(ctx context.Context, cmd *SmartHidCommand) (*SmartHidAck, bool, *SendError) {
	// 1. 校验（envelope + payload 深度）
	if errs := ValidateCommand(cmd); HasErrors(errs) {
		return nil, false, validationErr(errs...)
	}
	if errs := ValidatePayload(cmd); HasErrors(errs) {
		return nil, false, &SendError{Kind: ErrKindValidation, Message: "invalid command payload", Fields: errs}
	}
	fp := Fingerprint(cmd)

	// 2. 幂等登记：在途 execution → join（同指纹）或 conflict（异指纹）
	e.mu.Lock()
	if ex, ok := e.execs[cmd.RequestID]; ok {
		e.mu.Unlock()
		if ex.fingerprint != fp {
			return nil, false, &SendError{Kind: ErrKindConflict,
				Message: "request_id already in flight with a different command"}
		}
		return e.joinExecution(ctx, ex)
	}
	ex := &execution{fingerprint: fp, deviceID: cmd.DeviceID, done: make(chan struct{})}
	e.execs[cmd.RequestID] = ex
	e.mu.Unlock()
	owner := true
	defer func() {
		if owner {
			e.mu.Lock()
			delete(e.execs, cmd.RequestID)
			e.mu.Unlock()
		}
	}()

	// 3. 设备就绪检查
	online, usbReady, ok := e.devices.IsReady(cmd.DeviceID)
	if !ok {
		return nil, false, validationErr(&ValidationError{"device", fmt.Sprintf("unknown device %s", cmd.DeviceID)})
	}
	if !online {
		return nil, false, validationErr(&ValidationError{"device", fmt.Sprintf("device %s offline", cmd.DeviceID)})
	}
	if !usbReady {
		return nil, false, validationErr(&ValidationError{"device", fmt.Sprintf("device %s USB HID not ready", cmd.DeviceID)})
	}

	// 4. 持久化（错误不再静默：UNIQUE → 重放/冲突判定；其他错误 → 500，不 publish）
	if err := e.insertCommand(cmd, fp); err != nil {
		if isUniqueConstraintErr(err) {
			return e.replayFromDB(cmd, fp)
		}
		e.log.Error("persist command failed", "err", err, "request_id", cmd.RequestID)
		return nil, false, &SendError{Kind: ErrKindInternal, Message: "persist command failed"}
	}

	// 5. publish command（QoS1, retain=false —— 严禁 retained command）
	payload, _ := json.Marshal(cmd)
	token := e.mqttClient.Publish(CommandTopic(cmd.DeviceID), 1, false, payload)
	if !token.WaitTimeout(time.Duration(cmd.TTLMs) * time.Millisecond) {
		ex.finish(nil)
		return nil, false, validationErr(&ValidationError{"mqtt", "publish timeout"})
	}
	if err := token.Error(); err != nil {
		ex.finish(nil)
		return nil, false, validationErr(&ValidationError{"mqtt", err.Error()})
	}
	e.log.Info("command published", "request_id", cmd.RequestID, "device_id", cmd.DeviceID, "type", cmd.Type, "action", cmd.Action)

	// 6. 等终态（超时 = TTL → 202 语义收尾；晚到 ACK 仍会落库）
	timeout := time.Duration(cmd.TTLMs) * time.Millisecond
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			ex.finish(nil)
			return nil, false, nil
		}
		select {
		case <-ex.done:
			ack, terminal := ex.snapshot()
			return ack, terminal, nil
		case <-ctx.Done():
			// 客户端放弃等待：命令已发出，按 202 语义收尾（不撤销执行）。
			ex.finish(nil)
			return nil, false, validationErr(&ValidationError{"client", "request cancelled"})
		case <-time.After(remaining):
			ex.finish(nil)
			return nil, false, nil
		}
	}
}

// joinExecution 等待在途 execution 的同一结果（同 request_id 同指纹的并发请求）。
func (e *Engine) joinExecution(ctx context.Context, ex *execution) (*SmartHidAck, bool, *SendError) {
	select {
	case <-ex.done:
		ack, terminal := ex.snapshot()
		return ack, terminal, nil
	case <-ctx.Done():
		return nil, false, validationErr(&ValidationError{"client", "request cancelled"})
	}
}

// replayFromDB 处理 DB 中已存在同 request_id 行的重放请求。
func (e *Engine) replayFromDB(cmd *SmartHidCommand, fp string) (*SmartHidAck, bool, *SendError) {
	rowFP, status, code, execMs, found := e.loadCommandRow(cmd.RequestID)
	if !found {
		// UNIQUE 冲突但行不可见（并发窗口极小）——按内部错误处理，不 publish。
		e.log.Error("unique conflict but row not found", "request_id", cmd.RequestID)
		return nil, false, &SendError{Kind: ErrKindInternal, Message: "command store inconsistent"}
	}
	// 指纹不同 → 409；旧行 fingerprint=''（升级前数据）视为未知内容，不判冲突。
	if rowFP != "" && rowFP != fp {
		return nil, false, &SendError{Kind: ErrKindConflict,
			Message: "request_id already used with a different command"}
	}
	if isTerminalCommandStatus(status) {
		// 终态重放：直接返回既有结果，不再执行 HID。
		ack := &SmartHidAck{
			Protocol:    ProtocolVersion,
			RequestID:   cmd.RequestID,
			DeviceID:    cmd.DeviceID,
			Status:      AckStatus(status),
			Code:        code,
			ExecutionMs: execMs,
		}
		e.log.Info("terminal replay served", "request_id", cmd.RequestID, "status", status)
		return ack, true, nil
	}
	// 非终态（received/executing）：命令此前已 publish，可能实际执行中——
	// 不重新执行，按 202 语义返回。
	e.log.Info("non-terminal replay served as accepted", "request_id", cmd.RequestID, "status", status)
	return nil, false, nil
}

func (e *Engine) insertCommand(cmd *SmartHidCommand, fp string) error {
	_, err := e.db.DB.Exec(
		`INSERT INTO commands(request_id, device_id, type, action, ttl_ms, status, fingerprint)
		 VALUES(?,?,?,?,?,?,?)`,
		cmd.RequestID, cmd.DeviceID, string(cmd.Type), cmd.Action, cmd.TTLMs, string(AckReceived), fp,
	)
	return err
}

// loadCommandRow 读取命令行（fingerprint/status/code/execution_ms）。
func (e *Engine) loadCommandRow(requestID string) (fp, status string, code int, execMs int, found bool) {
	var ms *int
	err := e.db.DB.QueryRow(
		`SELECT fingerprint, status, code, execution_ms FROM commands WHERE request_id=?`, requestID,
	).Scan(&fp, &status, &code, &ms)
	if err != nil {
		return "", "", 0, 0, false
	}
	if ms != nil {
		execMs = *ms
	}
	return fp, status, code, execMs, true
}

// QueryCommand 从 SQLite 查询命令状态（供 GET /commands/{request_id}）。
func (e *Engine) QueryCommand(requestID string) (status string, code int, execMs sqlNullInt, found bool, err error) {
	row := e.db.DB.QueryRow(
		`SELECT status, code, execution_ms FROM commands WHERE request_id=?`, requestID,
	)
	var execMsNullable *int
	if err := row.Scan(&status, &code, &execMsNullable); err != nil {
		return "", 0, sqlNullInt{}, false, nil // not found 不算 error
	}
	execMs = sqlNullInt{Valid: execMsNullable != nil}
	if execMsNullable != nil {
		execMs.Int = *execMsNullable
	}
	return status, code, execMs, true, nil
}

// sqlNullInt 兼容 sql.NullInt64 的简化封装（避免在每个调用点引入 database/sql）。
type sqlNullInt struct {
	Int   int
	Valid bool
}
