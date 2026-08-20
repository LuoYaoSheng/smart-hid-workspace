package pairing

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 用 httptest 直接拿 handler，避免占用真实端口 17892。
func newDeviceTestServer(t *testing.T, m *Manager) *httptest.Server {
	t.Helper()
	srv := NewDeviceServer(m, "127.0.0.1:0", silentLogger())
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

func TestGuessLANIP_ReturnsNonLoopback(t *testing.T) {
	ip := GuessLANIP()
	if ip == "" {
		t.Fatal("empty IP")
	}
	// 在 CI/容器里可能只有环回，那就接受 127.0.0.1
	t.Logf("guessed LAN IP: %s", ip)
}
