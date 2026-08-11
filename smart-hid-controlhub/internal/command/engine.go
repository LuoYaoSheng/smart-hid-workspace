package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

// Engine 是 Command Engine：接收 HTTP 请求 → 校验 → publish → 等 ACK。
type Engine struct {
	mqttClient pahomqtt.Client
	devices    *device.Manager
	db         *storage.Store
	log        *slog.Logger

	mu       sync.RWMutex
	pending  map[string]chan *SmartHidAck // request_id -> ack chan
}

// New 构造 Engine。
func New(client pahomqtt.Client, dm *device.Manager, db *storage.Store, log *slog.Logger) *Engine {
	return &Engine{
		mqttClient: client,
		devices:    dm,
		db:         db,
		log:        log,
		pending:    make(map[string]chan *SmartHidAck),
	}
}

// HandleAck 是订阅 ack topic 的回调。解析 ACK，路由到对应 request_id 的 pending chan。
func (e *Engine) HandleAck(_ pahomqtt.Client, msg pahomqtt.Message) {
	var ack SmartHidAck
	if err := json.Unmarshal(msg.Payload(), &ack); err != nil {
		e.log.Warn("ack unmarshal failed", "err", err, "topic", msg.Topic(), "payload", string(msg.Payload()))
		return
	}
	e.log.Debug("ack received", "request_id", ack.RequestID, "status", ack.Status, "code", ack.Code, "exec_ms", ack.ExecutionMs)

	e.mu.RLock()
	ch, ok := e.pending[ack.RequestID]
	e.mu.RUnlock()
	if ok {
		// 非阻塞发，超时 goroutine 已退出则丢弃
		select {
		case ch <- &ack:
		default:
			e.log.Debug("ack dropped: no waiter or chan full", "request_id", ack.RequestID)
		}
	}

	// persist ACK 终态
	if ack.Status.IsTerminal() {
		_, _ = e.db.DB.Exec(
			`UPDATE commands SET status=?, code=?, execution_ms=?, acked_at=? WHERE request_id=?`,
			string(ack.Status), ack.Code, ack.ExecutionMs, time.Now().Unix(), ack.RequestID,
		)
	}
}

// Send 执行闭环：校验 → 检查设备 → publish command → 等 ACK。
// 返回 (ack, terminal) —— terminal=false 表示 TTL 内未收到终态（调用方按 202 处理）。
func (e *Engine) Send(ctx context.Context, cmd *SmartHidCommand) (*SmartHidAck, bool, []*ValidationError) {
	// 1. 校验
	if errs := ValidateCommand(cmd); HasErrors(errs) {
		return nil, false, errs
	}

	// 2. 设备就绪检查
	online, usbReady, ok := e.devices.IsReady(cmd.DeviceID)
	if !ok {
		// 设备未知：拒绝（Phase 1 无配对，仅 status 注册）
		return nil, false, []*ValidationError{{"device", fmt.Sprintf("unknown device %s", cmd.DeviceID)}}
	}
	if !online {
		return nil, false, []*ValidationError{{"device", fmt.Sprintf("device %s offline", cmd.DeviceID)}}
	}
	if !usbReady {
		return nil, false, []*ValidationError{{"device", fmt.Sprintf("device %s USB HID not ready", cmd.DeviceID)}}
	}

	// 3. 记录命令到 SQLite（status=received）
	_, _ = e.db.DB.Exec(
		`INSERT INTO commands(request_id, device_id, type, action, ttl_ms, status)
		 VALUES(?,?,?,?,?,?)`,
		cmd.RequestID, cmd.DeviceID, string(cmd.Type), cmd.Action, cmd.TTLMs, string(AckReceived),
	)

	// 4. 注册 pending ack chan
	ackCh := make(chan *SmartHidAck, 4)
	e.mu.Lock()
	e.pending[cmd.RequestID] = ackCh
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.pending, cmd.RequestID)
		e.mu.Unlock()
	}()

	// 5. publish command（QoS1, retain=false —— 严禁 retained command）
	payload, _ := json.Marshal(cmd)
	token := e.mqttClient.Publish(CommandTopic(cmd.DeviceID), 1, false, payload)
	if !token.WaitTimeout(time.Duration(cmd.TTLMs) * time.Millisecond) {
		return nil, false, []*ValidationError{{"mqtt", "publish timeout"}}
	}
	if err := token.Error(); err != nil {
		return nil, false, []*ValidationError{{"mqtt", err.Error()}}
	}
	e.log.Info("command published", "request_id", cmd.RequestID, "device_id", cmd.DeviceID, "type", cmd.Type, "action", cmd.Action)

	// 6. 等 ACK（超时 = TTL）
	timeout := time.Duration(cmd.TTLMs) * time.Millisecond
	deadline := time.Now().Add(timeout)
	var latest *SmartHidAck
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		select {
		case ack := <-ackCh:
			latest = ack
			if ack.Status.IsTerminal() {
				return ack, true, nil
			}
			// 中间态（received/executing），继续等
		case <-ctx.Done():
			return latest, false, []*ValidationError{{"client", "request cancelled"}}
		case <-time.After(remaining):
			// 超时
		}
	}

	// TTL 内未收到终态 —— 命令已发出，但 ack 未到
	if latest != nil {
		// 收到过中间态，但仍未终态
		return latest, false, nil
	}
	return nil, false, nil
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
