package api

import (
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
	pm := pairing.New(store.DB, "127.0.0.1", 17891, pairing.DefaultTTLSec, silentLog())

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
