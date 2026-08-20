package pairing

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"smart-hid-controlhub/internal/netaddr"
)

// 用 httptest 直接拿 handler，避免占用真实端口 17892。
func newDeviceTestServer(t *testing.T, m *Manager) *httptest.Server {
	t.Helper()
	srv := NewDeviceServer(m, netaddr.New(""), "127.0.0.1:0", silentLogger())
	ts := httptest.NewServer(http.HandlerFunc(srv.handleDevice))
	t.Cleanup(ts.Close)
	return ts
}

func TestDeviceServer_HappyPath(t *testing.T) {
	m, _ := newMgr(t)
	token, _, _ := m.CreateSession()
	ts := newDeviceTestServer(t, m)

	body, _ := json.Marshal(DeviceReq{
		Token: token, DeviceID: "HID-ABCD1234", BootID: "boot-1", Firmware: "1.0.0",
	})
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var result PairingResult
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result.MQTTUsername != "dev_HID-ABCD1234" {
		t.Errorf("username = %s", result.MQTTUsername)
	}
	if result.MQTTCredential == "" {
		t.Errorf("credential empty")
	}
}

func TestDeviceServer_BadToken(t *testing.T) {
	m, _ := newMgr(t)
	ts := newDeviceTestServer(t, m)

	body, _ := json.Marshal(DeviceReq{
		Token: "nonexistent-token", DeviceID: "HID-ABCD1234", BootID: "boot-1",
	})
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (token not found → pairing_token_invalid)", resp.StatusCode)
	}
}

func TestDeviceServer_MissingFields(t *testing.T) {
	m, _ := newMgr(t)
	ts := newDeviceTestServer(t, m)

	body, _ := json.Marshal(DeviceReq{Token: "x"}) // 缺 device_id
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestDeviceServer_MethodNotAllowed(t *testing.T) {
	m, _ := newMgr(t)
	ts := newDeviceTestServer(t, m)

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", resp.StatusCode)
	}
}

func TestQRPayload_Format(t *testing.T) {
	m, _ := newMgr(t)
	qr := m.QRPayload("abc123", "192.168.1.10", 17892)
	want := "shid://pair?token=abc123&host=192.168.1.10&port=17892"
	if qr != want {
		t.Errorf("QR = %q, want %q", qr, want)
	}
}

var errNoRoute = errors.New("no route")

func mustCIDR(t *testing.T, cidr string) []net.IPNet {
	t.Helper()
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	ipnet.IP = ip
	return []net.IPNet{*ipnet}
}

// TestPairingDoesNotConsumeTokenWhenAdvertiseResolutionFails（spec M1-G3 §11）：
// endpoint 解析失败 → 503 + token 保持 pending（用户修好 advertise_host 后原 token 可重试）。
func TestPairingDoesNotConsumeTokenWhenAdvertiseResolutionFails(t *testing.T) {
	m, _ := newMgr(t)
	// 多网卡且无显式配置且路由推导失败 → resolver 明确失败
	badRes := netaddr.New("").
		WithSnapshots(func() ([]netaddr.InterfaceSnapshot, error) {
			return []netaddr.InterfaceSnapshot{
				{Name: "en0", Up: true, Addrs: mustCIDR(t, "192.168.1.20/24")},
				{Name: "utun4", Up: true, Addrs: mustCIDR(t, "100.101.1.5/32")},
			}, nil
		}).
		WithDialer(func(net.IP) (net.IP, error) { return nil, errNoRoute })
	srv := NewDeviceServer(m, badRes, "127.0.0.1:0", silentLogger())

	token, _, err := m.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(DeviceReq{Token: token, DeviceID: "HID-AAAA0000", BootID: "boot-1"})
	rec := httptest.NewRecorder()
	srv.handleDevice(rec, httptest.NewRequest(http.MethodPost, "/api/v1/pairing/device", bytes.NewReader(req)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("http = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	// token 必须未被消费
	sess, err := m.GetSession(token)
	if err != nil || sess == nil {
		t.Fatalf("get session: %v %v", sess, err)
	}
	if sess.Status != "pending" {
		t.Fatalf("token status = %s, want pending (must NOT be consumed on endpoint failure)", sess.Status)
	}

	// 修复后（显式 advertise_host）同一 token 配对成功
	goodSrv := NewDeviceServer(m, netaddr.New("192.168.1.8"), "127.0.0.1:0", silentLogger())
	rec2 := httptest.NewRecorder()
	goodSrv.handleDevice(rec2, httptest.NewRequest(http.MethodPost, "/api/v1/pairing/device", bytes.NewReader(req)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("retry http = %d, want 200; body=%s", rec2.Code, rec2.Body.String())
	}
	var res PairingResult
	if err := json.Unmarshal(rec2.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.MQTTHost != "192.168.1.8" {
		t.Fatalf("mqtt_host = %q, want explicit 192.168.1.8", res.MQTTHost)
	}
}

// TestDeviceServer_AdvertiseFromRequestPath 设备请求实际到达的本地地址成为
// mqtt_host（不再使用启动期静态值/首网卡猜测）。
func TestDeviceServer_AdvertiseFromRequestPath(t *testing.T) {
	m, _ := newMgr(t)
	res := netaddr.New("").WithDialer(func(net.IP) (net.IP, error) { return nil, errNoRoute })
	srv := NewDeviceServer(m, res, "0.0.0.0:0", silentLogger())
	ts := httptest.NewServer(http.HandlerFunc(srv.handleDevice))
	t.Cleanup(ts.Close)

	token, _, err := m.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(DeviceReq{Token: token, DeviceID: "HID-BBBB0001", BootID: "boot-9"})
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(req))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("http = %d, body=%s", resp.StatusCode, body)
	}
	var res2 PairingResult
	if err := json.Unmarshal(body, &res2); err != nil {
		t.Fatal(err)
	}
	// httptest server 从 127.0.0.1 访问 → LocalAddr 环回 + peer 环回 → 环回合法（本机测试场景）
	if res2.MQTTHost != "127.0.0.1" {
		t.Fatalf("mqtt_host = %q, want 127.0.0.1 (loopback peer, local test)", res2.MQTTHost)
	}
}
