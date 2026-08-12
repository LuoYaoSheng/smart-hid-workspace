// Package api: CL-6c 激活码在线激活 + 刷新的本地端点集成测试。
//
// 用真实 licmgr（测试 Ed25519 公钥）+ mock cloud httptest server（用配对私钥签 License）
// 验证完整链路：本地 API → cloud.Client → HTTP → mock cloud → licmgr.Import → 验签生效。
package api

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"smart-hid-controlhub/internal/apikey"
	"smart-hid-controlhub/internal/cloud"
	"smart-hid-controlhub/internal/command"
	"smart-hid-controlhub/internal/device"
	licmgr "smart-hid-controlhub/internal/license"
	"smart-hid-controlhub/internal/settings"
	"smart-hid-controlhub/internal/storage"
	cloudlic "smart-hid-cloud/pkg/license"
)

// signTestLicense 用 priv 给 deviceID 签一个有效 License（立即生效 + 365 天）。
func signTestLicense(t *testing.T, priv ed25519.PrivateKey, deviceID string) string {
	t.Helper()
	now := time.Now().Unix()
	payload := cloudlic.Payload{
		LicenseID: "lic_test1", AccountID: "acc_test", PlanID: "plan_test", DeviceID: deviceID,
		IssuedAt: now, ValidFrom: now, ExpiresAt: now + 365*86400,
		Features: []string{"hid_control"}, LicenseVersion: cloudlic.Version,
	}
	signed, err := cloudlic.Sign(payload, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, _ := cloudlic.Encode(signed)
	return string(raw)
}

// newLicenseCloudTestServer 装配：mock cloud（用 priv 签 License）+ 本地 api.Server（用 pub 验签）。
// mock cloud 把 consume/refresh 的 device_id 透传到签名 License。
func newLicenseCloudTestServer(t *testing.T, priv ed25519.PrivateKey, pub ed25519.PublicKey) string {
	t.Helper()
	log := silentLog()

	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/activation/consume":
			var req struct {
				DeviceID string `json:"device_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			_, _ = w.Write([]byte(signTestLicense(t, priv, req.DeviceID)))
		case "/api/v1/license/refresh":
			_, _ = w.Write([]byte(signTestLicense(t, priv, "HID-AAAAAAAA")))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(mockCloud.Close)

	store, err := storage.New(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	dm, err := device.New(store, log)
	if err != nil {
		t.Fatalf("device mgr: %v", err)
	}
	keys := apikey.New(store.DB, log)
	if err := keys.InsertTesting(testAPIKey, "test"); err != nil {
		t.Fatalf("seed api key: %v", err)
	}
	licenseMgr := licmgr.NewWithPublicKey(store.DB, pub, log)
	engine := command.New(&mockClient{}, dm, store, log)

	srv := New(engine, dm, keys, settings.New(store.DB), nil, nil, licenseMgr, log).
		WithCloudClient(cloud.New(mockCloud.URL+"/api/v1", log))
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts.URL
}

// TestLicense_ActivateByCode 在线激活：mock cloud 消费 → 本地导入 → ACTIVE。
func TestLicense_ActivateByCode(t *testing.T) {
	priv, pub, err := cloudlic.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	base := newLicenseCloudTestServer(t, priv, pub)

	st, _, bodyText := req(t, base, "POST", "/api/v1/license/activate-code",
		map[string]string{"code": "ABCD12345678", "device_id": "HID-AAAAAAAA"}, true)
	if st != 200 {
		t.Fatalf("activate-by-code: expected 200, got %d %s", st, bodyText)
	}
	var resp struct {
		Activated bool   `json:"activated"`
		DeviceID  string `json:"device_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal([]byte(bodyText), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Activated || resp.Status != "ACTIVE" {
		t.Fatalf("activate response: %+v", resp)
	}
}

// TestLicense_Refresh 在线刷新：激活后刷新 → 仍 ACTIVE。
func TestLicense_Refresh(t *testing.T) {
	priv, pub, err := cloudlic.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	base := newLicenseCloudTestServer(t, priv, pub)

	// 先激活（device_id 必须与 mock cloud refresh 签名的一致：HID-AAAAAAAA）
	if st, _, _ := req(t, base, "POST", "/api/v1/license/activate-code",
		map[string]string{"code": "ABCD12345678", "device_id": "HID-AAAAAAAA"}, true); st != 200 {
		t.Fatalf("activate prerequisite: expected 200")
	}
	st, _, bodyText := req(t, base, "POST", "/api/v1/license/refresh",
		map[string]string{"device_id": "HID-AAAAAAAA"}, true)
	if st != 200 {
		t.Fatalf("refresh: expected 200, got %d %s", st, bodyText)
	}
}

// TestLicense_OfflineMode503 cloud 未配置 → activate 返 503。
func TestLicense_OfflineMode503(t *testing.T) {
	log := silentLog()
	store, err := storage.New(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	dm, _ := device.New(store, log)
	keys := apikey.New(store.DB, log)
	_ = keys.InsertTesting(testAPIKey, "test")
	pub, _, _ := cloudlic.GenerateKeypair()
	srv := New(command.New(&mockClient{}, dm, store, log), dm, keys,
		settings.New(store.DB), nil, nil, licmgr.NewWithPublicKey(store.DB, pub, log), log)
	// 不调 WithCloudClient → cloudCli=nil（离线模式）
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	httpReq, _ := http.NewRequest("POST", ts.URL+"/api/v1/license/activate-code",
		strings.NewReader(`{"code":"X","device_id":"HID-AAAAAAAA"}`))
	httpReq.Header.Set("Authorization", "Bearer "+testAPIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("offline activate: expected 503, got %d", resp.StatusCode)
	}
}
