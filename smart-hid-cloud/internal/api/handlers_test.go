// Package api: e2e 测试（CL-2c）。
// 覆盖完整 License 闭环：注册→登录→套餐→订单→支付→激活→下载→验签。
// 用临时 Ed25519 keypair + 临时 SQLite，httptest.NewServer 跑真实 HTTP 链路。
package api

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"smart-hid-cloud/internal/storage"
	"smart-hid-cloud/internal/store"
	"smart-hid-cloud/pkg/license"
)

var emailCounter int64

// newTestServer 装配临时 server（随机 keypair + temp DB + 默认 plan）。
// 返回 httptest.Server + 公钥（用于客户端验签）+ store（用于直接 seed/检查）。
func newTestServer(t *testing.T) (*httptest.Server, ed25519.PublicKey, *store.Store) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	dbPath := filepath.Join(t.TempDir(), "test.db")
	storageStore, err := storage.New(dbPath, log)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = storageStore.Close() })

	bizStore := store.New(storageStore.DB)
	if err := bizStore.SeedPlans([]store.Plan{
		{PlanID: "plan_test", Name: "Test", PriceCents: 100, Currency: "CNY",
			DurationDays: 365, Features: []string{"hid_control"}, Active: true},
	}); err != nil {
		t.Fatal(err)
	}

	priv, pub, err := license.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	srv := New(bizStore, []byte("test-jwt-secret"), priv, log)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts, pub, bizStore
}

// doReq 辅助：发 JSON 请求，返回状态码 + 解析后 body（map）+ 原始 body。
func doReq(t *testing.T, ts *httptest.Server, method, path string, body any, token string) (int, map[string]any, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ts.URL+path, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var j map[string]any
	_ = json.Unmarshal(raw, &j)
	return resp.StatusCode, j, raw
}

func uniqueEmail() string {
	n := atomic.AddInt64(&emailCounter, 1)
	return fmt.Sprintf("test-%d-%d@example.com", time.Now().UnixNano(), n)
}

// TestE2E_FullLicenseFlow 完整闭环：注册→登录→套餐→注册设备→订单→支付→激活→下载→验签。
func TestE2E_FullLicenseFlow(t *testing.T) {
	ts, pub, _ := newTestServer(t)

	// 1. 注册
	email := uniqueEmail()
	status, j, _ := doReq(t, ts, "POST", "/api/v1/auth/register",
		map[string]string{"email": email, "password": "testpass12345"}, "")
	if status != 201 {
		t.Fatalf("register status=%d body=%v", status, j)
	}
	token, _ := j["token"].(string)
	if token == "" {
		t.Fatal("no token")
	}

	// 2. /users/me 验证 token
	status, j, _ = doReq(t, ts, "GET", "/api/v1/users/me", nil, token)
	if status != 200 || j["email"] != email {
		t.Errorf("/users/me status=%d email=%v", status, j["email"])
	}

	// 3. 列套餐
	status, j, _ = doReq(t, ts, "GET", "/api/v1/plans", nil, "")
	if status != 200 {
		t.Errorf("plans status=%d", status)
	}

	// 4. 注册设备
	status, _, _ = doReq(t, ts, "POST", "/api/v1/devices",
		map[string]string{"device_id": "HID-AAAA1111", "display_name": "测试"}, token)
	if status != 201 {
		t.Errorf("create device status=%d", status)
	}

	// 5. 创建订单
	status, j, _ = doReq(t, ts, "POST", "/api/v1/orders",
		map[string]string{"plan_id": "plan_test"}, token)
	if status != 201 {
		t.Fatalf("create order status=%d body=%v", status, j)
	}
	orderID, _ := j["order_id"].(string)

	// 6. Mock 支付
	status, j, _ = doReq(t, ts, "POST", "/api/v1/orders/"+orderID+"/pay-callback", map[string]any{}, token)
	if status != 200 {
		t.Fatalf("pay status=%d body=%v", status, j)
	}
	lic, _ := j["license"].(map[string]any)
	licenseID, _ := lic["license_id"].(string)
	if licenseID == "" {
		t.Fatal("no license_id after pay")
	}

	// 7. 激活前：license 应为 UNUSED
	status, j, _ = doReq(t, ts, "GET", "/api/v1/licenses/"+licenseID, nil, token)
	if j["status"] != "UNUSED" {
		t.Errorf("license status pre-activate = %v", j["status"])
	}

	// 8. 激活
	status, j, _ = doReq(t, ts, "POST", "/api/v1/licenses/"+licenseID+"/activate",
		map[string]string{"device_id": "HID-AAAA1111"}, token)
	if status != 200 {
		t.Fatalf("activate status=%d body=%v", status, j)
	}
	payload, _ := j["payload"].(map[string]any)
	if payload["device_id"] != "HID-AAAA1111" {
		t.Errorf("payload device_id = %v", payload["device_id"])
	}
	if payload["license_id"] != licenseID {
		t.Errorf("payload license_id mismatch: got %v want %s", payload["license_id"], licenseID)
	}
	sig, _ := j["signature"].(string)
	if sig == "" {
		t.Error("no signature")
	}

	// 9. 下载 .license
	status, _, raw := doReq(t, ts, "GET", "/api/v1/licenses/"+licenseID+"/download", nil, token)
	if status != 200 {
		t.Fatalf("download status=%d", status)
	}
	lic2, err := license.Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// 10. VerifyFull 通过
	if err := license.VerifyFull(lic2, pub, "HID-AAAA1111", time.Now().Unix()); err != nil {
		t.Errorf("verify full: %v", err)
	}
	// 11. 错设备验签拒绝
	if err := license.VerifyFull(lic2, pub, "HID-WRONG01", time.Now().Unix()); err == nil {
		t.Error("wrong device passed verify")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	ts, _, _ := newTestServer(t)
	email := uniqueEmail()
	// 第一次
	doReq(t, ts, "POST", "/api/v1/auth/register",
		map[string]string{"email": email, "password": "testpass12345"}, "")
	// 第二次同 email
	status, j, _ := doReq(t, ts, "POST", "/api/v1/auth/register",
		map[string]string{"email": email, "password": "testpass12345"}, "")
	if status != 409 {
		t.Errorf("duplicate register status=%d body=%v", status, j)
	}
}

func TestLogin_BadPassword(t *testing.T) {
	ts, _, _ := newTestServer(t)
	email := uniqueEmail()
	doReq(t, ts, "POST", "/api/v1/auth/register",
		map[string]string{"email": email, "password": "testpass12345"}, "")
	status, _, _ := doReq(t, ts, "POST", "/api/v1/auth/login",
		map[string]string{"email": email, "password": "wrong-password"}, "")
	if status != 401 {
		t.Errorf("bad login status=%d, want 401", status)
	}
}

func TestProtectedEndpoints_RequireAuth(t *testing.T) {
	ts, _, _ := newTestServer(t)
	for _, p := range []string{"/api/v1/devices", "/api/v1/orders", "/api/v1/licenses", "/api/v1/users/me"} {
		status, _, _ := doReq(t, ts, "GET", p, nil, "")
		if status != 401 {
			t.Errorf("GET %s without token status=%d, want 401", p, status)
		}
	}
}

func TestActivate_NotYourLicense(t *testing.T) {
	ts, _, _ := newTestServer(t)
	// 用户 A 创建 license
	emailA := uniqueEmail()
	_, jA, _ := doReq(t, ts, "POST", "/api/v1/auth/register",
		map[string]string{"email": emailA, "password": "testpass12345"}, "")
	tokA, _ := jA["token"].(string)
	doReq(t, ts, "POST", "/api/v1/devices",
		map[string]string{"device_id": "HID-AAAA1111"}, tokA)
	_, jOrd, _ := doReq(t, ts, "POST", "/api/v1/orders",
		map[string]string{"plan_id": "plan_test"}, tokA)
	_, jPay, _ := doReq(t, ts, "POST", "/api/v1/orders/"+jOrd["order_id"].(string)+"/pay-callback",
		map[string]any{}, tokA)
	licA := jPay["license"].(map[string]any)["license_id"].(string)

	// 用户 B 尝试激活 A 的 license
	emailB := uniqueEmail()
	_, jB, _ := doReq(t, ts, "POST", "/api/v1/auth/register",
		map[string]string{"email": emailB, "password": "testpass12345"}, "")
	tokB, _ := jB["token"].(string)
	status, _, _ := doReq(t, ts, "POST", "/api/v1/licenses/"+licA+"/activate",
		map[string]string{"device_id": "HID-AAAA1111"}, tokB)
	if status != 403 {
		t.Errorf("cross-user activate status=%d, want 403", status)
	}
}

// 引用 os 避免 unused（部分测试可能未直接用）
var _ = os.DevNull
