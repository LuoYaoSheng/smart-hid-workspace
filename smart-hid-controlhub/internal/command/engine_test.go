package command

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"smart-hid-controlhub/internal/device"
	"smart-hid-controlhub/internal/protocol"
	"smart-hid-controlhub/internal/storage"
)

// ---------- 测试桩：paho MQTT ----------

// mockClient 通过嵌入 pahomqtt.Client 接口实现"只需 Publish"的部分 mock。
// Engine 只调用 Publish，其它方法若被调用会 panic（nil 接口）——测试中不会触发。
type mockClient struct {
	pahomqtt.Client
	publishCalls []publishCall
	publishFn    func(topic string, qos byte, retained bool, payload any) pahomqtt.Token
}

type publishCall struct {
	topic    string
	qos      byte
	retained bool
	payload  []byte
}

func (m *mockClient) Publish(topic string, qos byte, retained bool, payload any) pahomqtt.Token {
	var b []byte
	switch v := payload.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		b, _ = json.Marshal(v)
	}
	m.publishCalls = append(m.publishCalls, publishCall{topic, qos, retained, b})
	if m.publishFn != nil {
		return m.publishFn(topic, qos, retained, payload)
	}
	return neverToken() // 默认永不完成（publish 卡住）
}

// mockToken 实现 paho Token，可控制 WaitTimeout/Error 行为。
type mockToken struct {
	completed chan struct{}
	err       error
}

func (t *mockToken) Wait() bool {
	<-t.completed
	return true
}
func (t *mockToken) WaitTimeout(d time.Duration) bool {
	select {
	case <-t.completed:
		return true
	case <-time.After(d):
		return false
	}
}
func (t *mockToken) Done() <-chan struct{} { return t.completed }
func (t *mockToken) Error() error {
	select {
	case <-t.completed:
	default:
	}
	return t.err
}

// instantToken 立即完成的成功 token。
func instantToken() pahomqtt.Token {
	c := make(chan struct{})
	close(c)
	return &mockToken{completed: c}
}

// neverToken 永不完成的 token（用于 publish 超时场景）。
func neverToken() pahomqtt.Token {
	return &mockToken{completed: make(chan struct{})}
}

// mockMessage 实现 paho Message。
type mockMessage struct {
	topic   string
	payload []byte
}

func (m *mockMessage) Duplicate() bool   { return false }
func (m *mockMessage) Qos() byte         { return 0 }
func (m *mockMessage) Retained() bool    { return false }
func (m *mockMessage) Topic() string     { return m.topic }
func (m *mockMessage) MessageID() uint16 { return 0 }
func (m *mockMessage) Payload() []byte   { return m.payload }
func (m *mockMessage) Ack()              {}

// ---------- 测试辅助 ----------

func newTestEngine(t *testing.T, client pahomqtt.Client) (*Engine, *device.Manager, *storage.Store) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := storage.New(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	dm, err := device.New(store, log)
	if err != nil {
		t.Fatalf("new device manager: %v", err)
	}
	return New(client, dm, store, log), dm, store
}

func readyStatus(devID string) *protocol.SmartHidStatus {
	return &protocol.SmartHidStatus{
		Protocol: protocol.ProtocolVersion, DeviceID: devID, Online: true,
		BootID: "boot-1", USBHIDReady: true, Firmware: "1.0.0", Timestamp: 1700000000,
	}
}

func ackJSON(reqID, devID string, status AckStatus, code, execMs int) []byte {
	a := SmartHidAck{
		Protocol: ProtocolVersion, RequestID: reqID, DeviceID: devID,
		BootID: "boot-1", Status: status, Code: code, ExecutionMs: execMs,
	}
	b, _ := json.Marshal(a)
	return b
}

// ---------- Topic 格式 ----------

func TestTopics(t *testing.T) {
	devID := "HID-ABCD1234"
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"command", CommandTopic(devID), "smart-hid/v1/devices/HID-ABCD1234/command"},
		{"ack", AckTopic(devID), "smart-hid/v1/devices/HID-ABCD1234/ack"},
		{"status", StatusTopic(devID), "smart-hid/v1/devices/HID-ABCD1234/status"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s topic = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// ---------- HandleAck ----------

func TestHandleAck_IntermediateIgnored_TerminalSettles(t *testing.T) {
	eng, _, _ := newTestEngine(t, nil)
	ex := &execution{fingerprint: "fp", deviceID: "HID-ABCD1234", done: make(chan struct{})}
	eng.mu.Lock()
	eng.execs["req-1"] = ex
	eng.mu.Unlock()

	// 中间态：不落库、不收尾。
	eng.HandleAck(nil, &mockMessage{
		topic:   AckTopic("HID-ABCD1234"),
		payload: ackJSON("req-1", "HID-ABCD1234", AckExecuting, 0, 0),
	})
	select {
	case <-ex.done:
		t.Fatal("intermediate ack must not settle execution")
	default:
	}

	// 终态：收尾。
	eng.HandleAck(nil, &mockMessage{
		topic:   AckTopic("HID-ABCD1234"),
		payload: ackJSON("req-1", "HID-ABCD1234", AckExecuted, 0, 7),
	})
	select {
	case <-ex.done:
	default:
		t.Fatal("terminal ack should settle execution")
	}
	ack, terminal := ex.snapshot()
	if !terminal || ack == nil || ack.Status != AckExecuted {
		t.Fatalf("expected executed terminal, got ack=%+v terminal=%v", ack, terminal)
	}
}

func TestHandleAck_MalformedJSONNoPanic(t *testing.T) {
	eng, _, _ := newTestEngine(t, nil)
	// 坏 JSON 不应 panic，也不应投递到任何 chan。
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("HandleAck panicked on malformed JSON: %v", r)
		}
	}()
	eng.HandleAck(nil, &mockMessage{topic: AckTopic("HID-ABCD1234"), payload: []byte("not-json{")})
}

func TestHandleAck_UnknownRequestIDDropped(t *testing.T) {
	eng, _, _ := newTestEngine(t, nil)
	// pending 里没有 req-x，ACK 应被静默丢弃（不 panic）。
	eng.HandleAck(nil, &mockMessage{
		topic:   AckTopic("HID-ABCD1234"),
		payload: ackJSON("req-x", "HID-ABCD1234", AckExecuted, 0, 5),
	})
}

func seedCommandRow(t *testing.T, store *storage.Store, reqID, devID, status string) {
	t.Helper()
	// 先确保 devices 表有对应行（commands.device_id 是 FK）。
	_, _ = store.DB.Exec(
		`INSERT INTO devices(device_id, boot_id, online, usb_hid_ready, firmware, last_seen_at)
		 VALUES(?,?,?,?,?,0)
		 ON CONFLICT(device_id) DO NOTHING`,
		devID, "boot-1", 0, 0, "",
	)
	_, err := store.DB.Exec(
		`INSERT INTO commands(request_id, device_id, type, action, ttl_ms, status) VALUES(?,?,?,?,?,?)`,
		reqID, devID, "keyboard", "tap", 3000, status,
	)
	if err != nil {
		t.Fatalf("seed command: %v", err)
	}
}

func TestHandleAck_TerminalPersistsToDB(t *testing.T) {
	eng, _, store := newTestEngine(t, nil)
	// 预置一条 received 命令行（模拟 Send 步骤 3 已写入）。
	seedCommandRow(t, store, "req-1", "HID-ABCD1234", string(AckReceived))

	eng.HandleAck(nil, &mockMessage{
		topic:   AckTopic("HID-ABCD1234"),
		payload: ackJSON("req-1", "HID-ABCD1234", AckExecuted, 0, 12),
	})

	var st string
	if qerr := store.DB.QueryRow(
		`SELECT status FROM commands WHERE request_id=?`, "req-1",
	).Scan(&st); qerr != nil {
		t.Fatalf("query persisted status: %v", qerr)
	}
	if st != string(AckExecuted) {
		t.Fatalf("DB status = %q, want %q", st, AckExecuted)
	}
}

func TestHandleAck_NonTerminalNotPersisted(t *testing.T) {
	eng, _, store := newTestEngine(t, nil)
	seedCommandRow(t, store, "req-1", "HID-ABCD1234", string(AckReceived))
	// executing 是中间态，HandleAck 不应改写 DB。
	eng.HandleAck(nil, &mockMessage{
		topic:   AckTopic("HID-ABCD1234"),
		payload: ackJSON("req-1", "HID-ABCD1234", AckExecuting, 0, 0),
	})
	var st string
	_ = store.DB.QueryRow(`SELECT status FROM commands WHERE request_id=?`, "req-1").Scan(&st)
	if st != string(AckReceived) {
		t.Fatalf("intermediate ack should not overwrite DB; got %q, want %q", st, AckReceived)
	}
}

// ---------- Send ----------

func TestSend_HappyPath(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	// publish 成功后异步投递 executed ack，模拟设备回 ACK。
	client.publishFn = func(topic string, qos byte, retained bool, payload any) pahomqtt.Token {
		go func() {
			time.Sleep(5 * time.Millisecond)
			eng.HandleAck(nil, &mockMessage{
				topic:   AckTopic("HID-ABCD1234"),
				payload: ackJSON("req-1", "HID-ABCD1234", AckExecuted, 0, 8),
			})
		}()
		return instantToken()
	}

	cmd := validCmd() // request_id=req-abc123
	cmd.RequestID = "req-1"
	ack, terminal, serr := eng.Send(context.Background(), cmd)

	if serr != nil {
		t.Fatalf("unexpected send error: %v", serr)
	}
	if !terminal {
		t.Fatal("expected terminal=true, got false")
	}
	if ack == nil || ack.Status != AckExecuted {
		t.Fatalf("expected executed ack, got %+v", ack)
	}

	// publish 调用契约：topic 正确、QoS1、retain=false（严禁 retained command）
	if len(client.publishCalls) != 1 {
		t.Fatalf("expected exactly 1 publish, got %d", len(client.publishCalls))
	}
	pc := client.publishCalls[0]
	if pc.topic != CommandTopic("HID-ABCD1234") {
		t.Errorf("publish topic = %q, want %q", pc.topic, CommandTopic("HID-ABCD1234"))
	}
	if pc.qos != 1 {
		t.Errorf("publish qos = %d, want 1", pc.qos)
	}
	if pc.retained {
		t.Error("command must NEVER be published retained")
	}
	// payload 应是合法 command JSON
	var sent SmartHidCommand
	if err := json.Unmarshal(pc.payload, &sent); err != nil {
		t.Fatalf("publish payload not valid command JSON: %v", err)
	}
	if sent.RequestID != "req-1" || sent.Action != "tap" {
		t.Errorf("unexpected published command: %+v", sent)
	}

	// 在途 execution 应在 Send 返回后清理
	eng.mu.Lock()
	_, stillInFlight := eng.execs["req-1"]
	eng.mu.Unlock()
	if stillInFlight {
		t.Error("in-flight execution should be cleaned up after Send returns")
	}

	// 命令应已持久化到 commands 表
	var dbStatus string
	_ = eng.db.DB.QueryRow(`SELECT status FROM commands WHERE request_id=?`, "req-1").Scan(&dbStatus)
	if dbStatus != string(AckExecuted) {
		t.Errorf("DB command status = %q, want %q", dbStatus, AckExecuted)
	}
}

func TestSend_IntermediateThenTerminal(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	client.publishFn = func(topic string, qos byte, retained bool, payload any) pahomqtt.Token {
		go func() {
			// 先投中间态 received，再投终态 executed
			eng.HandleAck(nil, &mockMessage{topic: AckTopic("HID-ABCD1234"), payload: ackJSON("r1", "HID-ABCD1234", AckReceived, 0, 0)})
			time.Sleep(5 * time.Millisecond)
			eng.HandleAck(nil, &mockMessage{topic: AckTopic("HID-ABCD1234"), payload: ackJSON("r1", "HID-ABCD1234", AckExecuted, 0, 9)})
		}()
		return instantToken()
	}

	cmd := validCmd()
	cmd.RequestID = "r1"
	ack, terminal, serr := eng.Send(context.Background(), cmd)
	if serr != nil || !terminal || ack == nil || ack.Status != AckExecuted {
		t.Fatalf("expected executed terminal, got ack=%+v terminal=%v serr=%v", ack, terminal, serr)
	}
}

func TestSend_ValidationFailureSkipsPublish(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	cmd := validCmd()
	cmd.TTLMs = 5 // 非法 TTL
	ack, terminal, serr := eng.Send(context.Background(), cmd)

	if ack != nil || terminal {
		t.Fatal("invalid command should not produce ack/terminal")
	}
	if serr == nil {
		t.Fatal("expected send error")
	}
	hasField(t, serr.Fields, "ttl_ms")
	if len(client.publishCalls) != 0 {
		t.Fatalf("invalid command must not trigger publish, got %d calls", len(client.publishCalls))
	}
}

func TestSend_UnknownDevice(t *testing.T) {
	client := &mockClient{}
	eng, _, _ := newTestEngine(t, client)

	cmd := validCmd() // device HID-ABCD1234 未注册
	_, terminal, serr := eng.Send(context.Background(), cmd)
	if terminal {
		t.Fatal("unknown device should not be terminal")
	}
	hasField(t, serr.Fields, "device")
	if len(client.publishCalls) != 0 {
		t.Fatal("unknown device must not trigger publish")
	}
}

func TestSend_OfflineDevice(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(&protocol.SmartHidStatus{
		Protocol: protocol.ProtocolVersion, DeviceID: "HID-ABCD1234",
		Online: false, BootID: "boot-1", USBHIDReady: true, Firmware: "1.0.0", Timestamp: 1,
	})

	cmd := validCmd()
	_, terminal, serr := eng.Send(context.Background(), cmd)
	if terminal {
		t.Fatal("offline device should not be terminal")
	}
	hasField(t, serr.Fields, "device")
	if len(client.publishCalls) != 0 {
		t.Fatal("offline device must not trigger publish")
	}
}

func TestSend_USBNotReady(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(&protocol.SmartHidStatus{
		Protocol: protocol.ProtocolVersion, DeviceID: "HID-ABCD1234",
		Online: true, BootID: "boot-1", USBHIDReady: false, Firmware: "1.0.0", Timestamp: 1,
	})

	cmd := validCmd()
	_, terminal, serr := eng.Send(context.Background(), cmd)
	if terminal {
		t.Fatal("usb-not-ready device should not be terminal")
	}
	hasField(t, serr.Fields, "device")
	if len(client.publishCalls) != 0 {
		t.Fatal("usb-not-ready device must not trigger publish")
	}
}

func TestSend_PublishTimeout(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	// publishFn 返回永不完成的 token；TTL 内 WaitTimeout 超时 → publish timeout 错误。
	client.publishFn = func(string, byte, bool, any) pahomqtt.Token { return neverToken() }

	cmd := validCmd()
	cmd.TTLMs = TTLMsMin // 100ms，快速失败
	_, terminal, serr := eng.Send(context.Background(), cmd)
	if terminal {
		t.Fatal("publish timeout should not be terminal")
	}
	hasField(t, serr.Fields, "mqtt")
}

func TestSend_NoAckWithinTTL(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	// publish 成功但不投递任何 ack → TTL 内无终态 → (nil, false, nil)，HTTP 层映射 202。
	client.publishFn = func(string, byte, bool, any) pahomqtt.Token { return instantToken() }

	cmd := validCmd()
	cmd.RequestID = "noack-1"
	cmd.TTLMs = TTLMsMin // 100ms
	start := time.Now()
	ack, terminal, serr := eng.Send(context.Background(), cmd)
	elapsed := time.Since(start)

	if serr != nil {
		t.Fatalf("no-ack should not produce errors: %v", serr)
	}
	if terminal {
		t.Error("no-ack within TTL should be terminal=false")
	}
	if ack != nil {
		t.Errorf("no-ack should return nil ack, got %+v", ack)
	}
	// 应至少等满 TTL（100ms），不能提前返回
	if elapsed < 90*time.Millisecond {
		t.Errorf("Send returned in %v, should wait ~TTL(100ms)", elapsed)
	}
}

func TestSend_ClientContextCancelled(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	// publish 成功后取消 context，验证 ctx.Done() 分支返回 cancelled 错误。
	ctx, cancel := context.WithCancel(context.Background())
	client.publishFn = func(string, byte, bool, any) pahomqtt.Token {
		go cancel() // publish 后立即取消
		return instantToken()
	}

	cmd := validCmd()
	cmd.TTLMs = 5000 // 长 TTL，确保是 cancel 而非超时触发返回
	_, terminal, serr := eng.Send(ctx, cmd)
	if terminal {
		t.Fatal("cancelled request should not be terminal")
	}
	hasField(t, serr.Fields, "client")
}
