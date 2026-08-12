package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"smart-hid-controlhub/internal/apikey"
	"smart-hid-controlhub/internal/command"
	"smart-hid-controlhub/internal/device"
	"smart-hid-controlhub/internal/protocol"
	"smart-hid-controlhub/internal/settings"
	"smart-hid-controlhub/internal/storage"
)

// testAPIKey 必须符合生产 key 格式：chk_ + 64 hex chars（见 apikey.KeyPrefix / keyBytes）。
const testAPIKey = "chk_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// ---------- mock MQTT（仅 Publish，供 engine.Send 用） ----------

type mockClient struct {
	pahomqtt.Client
	publishFn func(topic string, qos byte, retained bool, payload any) pahomqtt.Token
}

func (m *mockClient) Publish(topic string, qos byte, retained bool, payload any) pahomqtt.Token {
	if m.publishFn != nil {
		return m.publishFn(topic, qos, retained, payload)
	}
	return &mockToken{completed: make(chan struct{})} // 默认永不完成
}

type mockToken struct {
	completed chan struct{}
	err       error
}

func (t *mockToken) Wait() bool { <-t.completed; return true }
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

func instantToken() pahomqtt.Token {
	c := make(chan struct{})
	close(c)
	return &mockToken{completed: c}
}

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

// ---------- 测试装配 ----------

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestServer 装配一个真实 engine（temp DB + 给定 mqtt client）+ httptest.Server。
// 返回的 base URL 即测试请求目标；dm/store/engine 供预置数据与注入 ACK。
func newTestServer(t *testing.T, client pahomqtt.Client) (base string, dm *device.Manager, store *storage.Store, engine *command.Engine) {
	t.Helper()
	log := silentLog()
	store, err := storage.New(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	dm, err = device.New(store, log)
	if err != nil {
		t.Fatalf("new device manager: %v", err)
	}
	engine = command.New(client, dm, store, log)
	// CH-P2：API key 持久化（apikey.Store）；seed 一个测试 key
	keys := apikey.New(store.DB, log)
	if err := keys.InsertTesting(testAPIKey, "test"); err != nil {
		t.Fatalf("seed api key: %v", err)
	}
	// CH-P4：settings store
	setStore := settings.New(store.DB)
	// CH-P5：pairing manager（nil 即不启用配对路由，简化测试）
	// CH-P6：trial manager（nil 即不启用 usage 路由）
	// CL-3a：license manager（nil 即不启用 license 路由）
	srv := New(engine, dm, keys, setStore, nil, nil, nil, log)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts.URL, dm, store, engine
}

func readyStatus(devID string) *protocol.SmartHidStatus {
	return &protocol.SmartHidStatus{
		Protocol: protocol.ProtocolVersion, DeviceID: devID, Online: true,
		BootID: "boot-1", USBHIDReady: true, Firmware: "1.0.0", Timestamp: 1700000000,
	}
}

// req 发一个请求；auth=true 带 Bearer testAPIKey。返回解析后的 JSON + 状态码。
func req(t *testing.T, base, method, path string, body any, auth bool) (status int, bodyJSON map[string]any, bodyText string) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	httpReq, err := http.NewRequest(method, base+path, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if auth {
		httpReq.Header.Set("Authorization", "Bearer "+testAPIKey)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	status = resp.StatusCode
	bodyText = string(raw)
	_ = json.Unmarshal(raw, &bodyJSON)
	return
}

// reqRaw 发原始字符串 body（用于坏 JSON 测试）。
func reqRaw(t *testing.T, base, method, path, body string, auth bool) (status int, bodyJSON map[string]any, bodyText string) {
	t.Helper()
	httpReq, _ := http.NewRequest(method, base+path, strings.NewReader(body))
	if auth {
		httpReq.Header.Set("Authorization", "Bearer "+testAPIKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	status = resp.StatusCode
	bodyText = string(raw)
	_ = json.Unmarshal(raw, &bodyJSON)
	return
}

// ---------- /health ----------

func TestHealth_NoAuth(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	status, j, _ := req(t, base, "GET", "/api/v1/health", nil, false)
	if status != 200 {
		t.Fatalf("health status = %d, want 200", status)
	}
	if j["status"] != "ok" {
		t.Errorf("health status field = %v, want ok", j["status"])
	}
	if j["protocol"] != command.ProtocolVersion {
		t.Errorf("health protocol = %v", j["protocol"])
	}
}

// ---------- 鉴权 ----------

func TestAuth_MissingBearer(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	status, j, _ := req(t, base, "GET", "/api/v1/devices", nil, false)
	if status != 401 {
		t.Fatalf("status = %d, want 401", status)
	}
	if j["error"] != "unauthorized" {
		t.Errorf("error = %v, want unauthorized", j["error"])
	}
}

func TestAuth_WrongKey(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	httpReq, _ := http.NewRequest("GET", base+"/api/v1/devices", nil)
	httpReq.Header.Set("Authorization", "Bearer wrong-key")
	resp, _ := http.DefaultClient.Do(httpReq)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// ---------- 设备列表/详情 ----------

func TestDevices_List(t *testing.T) {
	base, dm, _, _ := newTestServer(t, &mockClient{})
	dm.UpsertStatus(readyStatus("HID-AAAA1111"))

	status, j, _ := req(t, base, "GET", "/api/v1/devices", nil, true)
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	devs, ok := j["devices"].([]any)
	if !ok || len(devs) != 1 {
		t.Fatalf("devices = %v, want 1 entry", j["devices"])
	}
	d0 := devs[0].(map[string]any)
	if d0["device_id"] != "HID-AAAA1111" || d0["online"] != true {
		t.Errorf("device entry = %v", d0)
	}
	if j["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", j["total"])
	}
}

func TestDevices_DetailFound(t *testing.T) {
	base, dm, _, _ := newTestServer(t, &mockClient{})
	dm.UpsertStatus(readyStatus("HID-AAAA1111"))
	status, j, _ := req(t, base, "GET", "/api/v1/devices/HID-AAAA1111", nil, true)
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	if j["device_id"] != "HID-AAAA1111" {
		t.Errorf("device_id = %v", j["device_id"])
	}
}

func TestDevices_DetailNotFound(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	status, j, _ := req(t, base, "GET", "/api/v1/devices/HID-BBBB2222", nil, true)
	if status != 404 {
		t.Fatalf("status = %d, want 404", status)
	}
	if j["error"] != "not_found" {
		t.Errorf("error = %v", j["error"])
	}
}

// ---------- 发送命令 ----------

func validCommandBody(deviceID string) map[string]any {
	return map[string]any{
		"protocol": "1.0", "request_id": "req-test-1", "device_id": deviceID,
		"target_boot_id": "boot-1", "type": "keyboard", "action": "tap",
		"ttl_ms": 3000, "payload": map[string]any{"key": "ENTER", "hold_ms": 40},
	}
}

func TestSendCommand_Executed200(t *testing.T) {
	client := &mockClient{}
	base, dm, _, engine := newTestServer(t, client)
	dm.UpsertStatus(readyStatus("HID-AAAA1111"))

	// publish 后异步投递 executed ack（通过 engine.HandleAck 路由到 pending chan）。
	ackPayload, _ := json.Marshal(command.SmartHidAck{
		Protocol: command.ProtocolVersion, RequestID: "req-test-1",
		DeviceID: "HID-AAAA1111", BootID: "boot-1", Status: command.AckExecuted, Code: 0, ExecutionMs: 7,
	})
	client.publishFn = func(string, byte, bool, any) pahomqtt.Token {
		go func() {
			time.Sleep(5 * time.Millisecond)
			engine.HandleAck(nil, &mockMessage{topic: command.AckTopic("HID-AAAA1111"), payload: ackPayload})
		}()
		return instantToken()
	}

	status, j, _ := req(t, base, "POST", "/api/v1/devices/HID-AAAA1111/commands", validCommandBody("HID-AAAA1111"), true)
	if status != 200 {
		t.Fatalf("status = %d, want 200; body=%v", status, j)
	}
	if j["status"] != "executed" {
		t.Errorf("status field = %v, want executed", j["status"])
	}
	if j["code"].(float64) != 0 {
		t.Errorf("code = %v, want 0", j["code"])
	}
}

func TestSendCommand_NoAuth(t *testing.T) {
	base, dm, _, _ := newTestServer(t, &mockClient{})
	dm.UpsertStatus(readyStatus("HID-AAAA1111"))
	status, _, _ := req(t, base, "POST", "/api/v1/devices/HID-AAAA1111/commands", validCommandBody("HID-AAAA1111"), false)
	if status != 401 {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestSendCommand_ValidationFailed400(t *testing.T) {
	base, dm, _, _ := newTestServer(t, &mockClient{})
	dm.UpsertStatus(readyStatus("HID-AAAA1111"))
	body := validCommandBody("HID-AAAA1111")
	body["ttl_ms"] = 5 // 越界

	status, j, _ := req(t, base, "POST", "/api/v1/devices/HID-AAAA1111/commands", body, true)
	if status != 400 {
		t.Fatalf("status = %d, want 400", status)
	}
	if j["error"] != "validation_failed" {
		t.Errorf("error = %v", j["error"])
	}
	fields, ok := j["fields"].([]any)
	if !ok || len(fields) == 0 {
		t.Fatalf("fields missing: %v", j["fields"])
	}
	if fields[0].(map[string]any)["field"] != "ttl_ms" {
		t.Errorf("first field = %v, want ttl_ms", fields[0])
	}
}

func TestSendCommand_UnknownDevice400(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	// 不注册任何设备
	status, j, _ := req(t, base, "POST", "/api/v1/devices/HID-AAAA1111/commands", validCommandBody("HID-AAAA1111"), true)
	if status != 400 {
		t.Fatalf("status = %d, want 400", status)
	}
	if j["error"] != "validation_failed" {
		t.Errorf("error = %v", j["error"])
	}
	fields := j["fields"].([]any)
	if fields[0].(map[string]any)["field"] != "device" {
		t.Errorf("field = %v, want device", fields[0])
	}
}

func TestSendCommand_OfflineDevice400(t *testing.T) {
	base, dm, _, _ := newTestServer(t, &mockClient{})
	dm.UpsertStatus(&protocol.SmartHidStatus{
		Protocol: protocol.ProtocolVersion, DeviceID: "HID-AAAA1111", Online: false,
		BootID: "boot-1", USBHIDReady: true, Firmware: "1.0.0", Timestamp: 1,
	})
	status, j, _ := req(t, base, "POST", "/api/v1/devices/HID-AAAA1111/commands", validCommandBody("HID-AAAA1111"), true)
	if status != 400 {
		t.Fatalf("status = %d, want 400 (device offline)", status)
	}
	if j["error"] != "validation_failed" {
		t.Errorf("error = %v", j["error"])
	}
}

func TestSendCommand_BadJSON(t *testing.T) {
	base, dm, _, _ := newTestServer(t, &mockClient{})
	dm.UpsertStatus(readyStatus("HID-AAAA1111"))
	status, j, _ := reqRaw(t, base, "POST", "/api/v1/devices/HID-AAAA1111/commands", "{not json", true)
	if status != 400 {
		t.Fatalf("status = %d, want 400", status)
	}
	if j["error"] != "bad_request" {
		t.Errorf("error = %v, want bad_request", j["error"])
	}
}

func TestSendCommand_PathDeviceIDMismatch(t *testing.T) {
	base, dm, _, _ := newTestServer(t, &mockClient{})
	dm.UpsertStatus(readyStatus("HID-AAAA1111"))
	// body device_id 与 path 不同
	body := validCommandBody("HID-ZZZZ9999")
	status, j, _ := req(t, base, "POST", "/api/v1/devices/HID-AAAA1111/commands", body, true)
	if status != 400 {
		t.Fatalf("status = %d, want 400", status)
	}
	if j["error"] != "bad_request" {
		t.Errorf("error = %v, want bad_request", j["error"])
	}
}

// ---------- 命令查询 ----------

func TestCommandQuery_NotFound(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	status, j, _ := req(t, base, "GET", "/api/v1/commands/never-existed", nil, true)
	if status != 404 {
		t.Fatalf("status = %d, want 404", status)
	}
	if j["error"] != "not_found" {
		t.Errorf("error = %v", j["error"])
	}
}

func TestCommandQuery_Found(t *testing.T) {
	base, _, store, _ := newTestServer(t, &mockClient{})
	// 直接 seed 一条命令行（含 FK 设备行）
	seedDeviceRow(t, store, "HID-AAAA1111")
	_, err := store.DB.Exec(
		`INSERT INTO commands(request_id, device_id, type, action, ttl_ms, status, code, execution_ms)
		 VALUES(?,?,?,?,?,?,?,?)`,
		"req-seeded", "HID-AAAA1111", "keyboard", "tap", 3000, "executed", 0, 12,
	)
	if err != nil {
		t.Fatalf("seed command: %v", err)
	}
	status, j, _ := req(t, base, "GET", "/api/v1/commands/req-seeded", nil, true)
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	if j["status"] != "executed" || j["code"].(float64) != 0 {
		t.Errorf("unexpected query result: %v", j)
	}
	if j["execution_ms"].(float64) != 12 {
		t.Errorf("execution_ms = %v, want 12", j["execution_ms"])
	}
}

func seedDeviceRow(t *testing.T, store *storage.Store, devID string) {
	t.Helper()
	_, err := store.DB.Exec(
		`INSERT INTO devices(device_id, boot_id, online, usb_hid_ready, firmware, last_seen_at)
		 VALUES(?,?,?,?,?,0) ON CONFLICT(device_id) DO NOTHING`,
		devID, "boot-1", 0, 0, "",
	)
	if err != nil {
		t.Fatalf("seed device: %v", err)
	}
}

// ---------- 方法约束 ----------

func TestMethodNotAllowed(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	// POST /devices 不允许（列表只 GET）
	status, j, _ := req(t, base, "POST", "/api/v1/devices", nil, true)
	if status != 405 {
		t.Fatalf("POST /devices status = %d, want 405", status)
	}
	if j["error"] != "method_not_allowed" {
		t.Errorf("error = %v", j["error"])
	}
	// GET /devices/{id}/commands 不允许（发命令只 POST）
	status, _, _ = req(t, base, "GET", "/api/v1/devices/HID-AAAA1111/commands", nil, true)
	if status != 405 {
		t.Fatalf("GET .../commands status = %d, want 405", status)
	}
}
