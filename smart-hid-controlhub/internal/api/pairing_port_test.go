package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"smart-hid-controlhub/internal/apikey"
	"smart-hid-controlhub/internal/device"
	"smart-hid-controlhub/internal/pairing"
	"smart-hid-controlhub/internal/storage"
)

// QR 载荷（shid://pair?...&port=）里的端口必须来自 config.pairing.port
// （WithPairingPort 注入），而非硬编码 DefaultPairingPort。
func TestPairingQRPort_FromConfig(t *testing.T) {
	store, err := storage.New(filepath.Join(t.TempDir(), "p.db"), silentLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	keys := apikey.New(store.DB, silentLog())
	if err := keys.InsertTesting(testAPIKey, "test"); err != nil {
		t.Fatal(err)
	}
	dm, err := device.New(store, silentLog())
	if err != nil {
		t.Fatal(err)
	}
	pm := pairing.New(store.DB, 17891, pairing.DefaultTTLSec, silentLog())

	srv := New(nil, dm, keys, nil, pm, silentLog()).WithPairingPort(28792)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/pairing/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", res.StatusCode)
	}
	body := make([]byte, 4096)
	n, _ := res.Body.Read(body)
	got := string(body[:n])
	if !strings.Contains(got, "port=28792") {
		t.Fatalf("qr payload 缺少配置端口 28792：%s", got)
	}
	if strings.Contains(got, "port=17892") {
		t.Fatalf("qr payload 仍是硬编码端口 17892：%s", got)
	}
}

// 配对面板闭环：创建 → QR PNG（200 image/png）→ 取消（200）→ 状态 cancelled
// → 再取消（404 幂等边界）→ QR（404）。
func TestPairingSessionQRAndCancel(t *testing.T) {
	store, err := storage.New(filepath.Join(t.TempDir(), "p.db"), silentLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	keys := apikey.New(store.DB, silentLog())
	if err := keys.InsertTesting(testAPIKey, "test"); err != nil {
		t.Fatal(err)
	}
	dm, err := device.New(store, silentLog())
	if err != nil {
		t.Fatal(err)
	}
	pm := pairing.New(store.DB, 17891, pairing.DefaultTTLSec, silentLog())

	srv := New(nil, dm, keys, nil, pm, silentLog()).WithPairingPort(28792)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	// 创建
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/pairing/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if created.Token == "" {
		t.Fatal("empty token")
	}

	// QR PNG：200 + image/png + 非空
	qrReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/pairing/sessions/"+created.Token+"/qr.png", nil)
	qrReq.Header.Set("Authorization", "Bearer "+testAPIKey)
	qrRes, err := http.DefaultClient.Do(qrReq)
	if err != nil {
		t.Fatal(err)
	}
	png, _ := io.ReadAll(qrRes.Body)
	qrRes.Body.Close()
	if qrRes.StatusCode != 200 || qrRes.Header.Get("Content-Type") != "image/png" || len(png) < 100 {
		t.Fatalf("qr.png = (%d, %s, %d bytes)", qrRes.StatusCode, qrRes.Header.Get("Content-Type"), len(png))
	}

	// 取消：200
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/pairing/sessions/"+created.Token, nil)
	delReq.Header.Set("Authorization", "Bearer "+testAPIKey)
	delRes, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	delRes.Body.Close()
	if delRes.StatusCode != 200 {
		t.Fatalf("DELETE = %d, want 200", delRes.StatusCode)
	}

	// 状态 = cancelled
	stReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/pairing/sessions/"+created.Token, nil)
	stReq.Header.Set("Authorization", "Bearer "+testAPIKey)
	stRes, err := http.DefaultClient.Do(stReq)
	if err != nil {
		t.Fatal(err)
	}
	var sess struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(stRes.Body).Decode(&sess)
	stRes.Body.Close()
	if sess.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", sess.Status)
	}

	// 再取消：404（不再 pending）
	delReq2, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/pairing/sessions/"+created.Token, nil)
	delReq2.Header.Set("Authorization", "Bearer "+testAPIKey)
	delRes2, err := http.DefaultClient.Do(delReq2)
	if err != nil {
		t.Fatal(err)
	}
	delRes2.Body.Close()
	if delRes2.StatusCode != 404 {
		t.Fatalf("second DELETE = %d, want 404", delRes2.StatusCode)
	}

	// QR：404（不再 pending）
	qrReq2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/pairing/sessions/"+created.Token+"/qr.png", nil)
	qrReq2.Header.Set("Authorization", "Bearer "+testAPIKey)
	qrRes2, err := http.DefaultClient.Do(qrReq2)
	if err != nil {
		t.Fatal(err)
	}
	qrRes2.Body.Close()
	if qrRes2.StatusCode != 404 {
		t.Fatalf("qr after cancel = %d, want 404", qrRes2.StatusCode)
	}
}
