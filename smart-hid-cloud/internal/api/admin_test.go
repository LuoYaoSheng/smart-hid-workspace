// Package api admin 鉴权测试（CL-5a）。
// 覆盖：无 token 401、普通用户 403、promote 后重新登录拿到 admin token 才能访问。
package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"smart-hid-cloud/internal/store"
)

// registerUser 注册一个普通用户，返回 email + 登录 token（role=user）。
func registerUser(t *testing.T, ts *httptest.Server) (email, token string) {
	t.Helper()
	email = uniqueEmail()
	_, j, _ := doReq(t, ts, "POST", "/api/v1/auth/register",
		map[string]string{"email": email, "password": "testpass123"}, "")
	token, _ = j["token"].(string)
	if token == "" {
		t.Fatalf("register %s: no token", email)
	}
	return email, token
}

// loginUser 重新登录拿新 token（promote 后用，新 token 带最新 role）。
func loginUser(t *testing.T, ts *httptest.Server, email string) string {
	t.Helper()
	_, j, _ := doReq(t, ts, "POST", "/api/v1/auth/login",
		map[string]string{"email": email, "password": "testpass123"}, "")
	tok, _ := j["token"].(string)
	if tok == "" {
		t.Fatalf("login %s: no token", email)
	}
	return tok
}

// TestAdmin_NoToken 无 token 调 /admin/* 返 401。
func TestAdmin_NoToken(t *testing.T) {
	ts, _, _ := newTestServer(t)
	code, _, _ := doReq(t, ts, "GET", "/api/v1/admin/stats", nil, "")
	if code != 401 {
		t.Fatalf("expected 401 without token, got %d", code)
	}
}

// TestAdmin_NonAdminForbidden 普通用户（role=user）调 /admin/* 返 403。
func TestAdmin_NonAdminForbidden(t *testing.T) {
	ts, _, _ := newTestServer(t)
	_, tok := registerUser(t, ts)
	code, _, _ := doReq(t, ts, "GET", "/api/v1/admin/stats", nil, tok)
	if code != 403 {
		t.Fatalf("expected 403 for non-admin, got %d", code)
	}
}

// TestAdmin_PromoteThenAllowed：promote 后必须重新登录拿带 role 的新 token；
// 旧 token（role=""）仍 403，新 token（role=admin）200。
func TestAdmin_PromoteThenAllowed(t *testing.T) {
	ts, _, st := newTestServer(t)
	email, oldTok := registerUser(t, ts)

	// promote（模拟 config.admin_email 启动行为）
	if err := st.PromoteAdmin(email); err != nil {
		t.Fatalf("promote: %v", err)
	}

	// 旧 token 仍 403（JWT 无状态，role 不随 DB 变化）
	if code, _, _ := doReq(t, ts, "GET", "/api/v1/admin/stats", nil, oldTok); code != 403 {
		t.Fatalf("old token after promote should still 403, got %d", code)
	}

	// 重新登录拿新 token（role=admin）
	newTok := loginUser(t, ts, email)
	code, j, _ := doReq(t, ts, "GET", "/api/v1/admin/stats", nil, newTok)
	if code != 200 {
		t.Fatalf("expected 200 for admin, got %d", code)
	}
	if j["users_total"] == nil {
		t.Fatalf("expected stats payload with users_total, got %v", j)
	}
}

// adminToken 注册一个用户并 promote 成 admin，返回 (adminUserID, adminToken)。
func adminToken(t *testing.T, ts *httptest.Server, st *store.Store) (string, string) {
	t.Helper()
	email, _ := registerUser(t, ts)
	if err := st.PromoteAdmin(email); err != nil {
		t.Fatalf("promote: %v", err)
	}
	// 拿 user_id（从 me）
	u, _ := st.ListAllUsers()
	var uid string
	for _, x := range u {
		if x.Email == email {
			uid = x.UserID
		}
	}
	return uid, loginUser(t, ts, email)
}

// TestAdmin_LicenseStatusOps 禁用→恢复→吊销→不可逆。
func TestAdmin_LicenseStatusOps(t *testing.T) {
	ts, _, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)

	// 预置一个 UNUSED license（直接用 store，绕过用户侧流程）
	lic, err := st.CreateLicense(uid, "plan_test", "", []string{"hid_control"})
	if err != nil {
		t.Fatal(err)
	}
	id := lic.LicenseID

	must := func(code int, j map[string]any, label string) {
		if code != 200 {
			t.Fatalf("%s: expected 200, got %d %v", label, code, j)
		}
	}

	// disable
	c, j, _ := doReq(t, ts, "POST", "/api/v1/admin/licenses/"+id+"/disable", nil, adminTok)
	must(c, j, "disable")
	if j["status"] != "DISABLED" {
		t.Fatalf("disable: expected DISABLED, got %v", j["status"])
	}
	// re-enable
	c, j, _ = doReq(t, ts, "POST", "/api/v1/admin/licenses/"+id+"/re-enable", nil, adminTok)
	must(c, j, "re-enable")
	if j["status"] != "ACTIVE" {
		t.Fatalf("re-enable: expected ACTIVE, got %v", j["status"])
	}
	// revoke
	c, j, _ = doReq(t, ts, "POST", "/api/v1/admin/licenses/"+id+"/revoke", nil, adminTok)
	must(c, j, "revoke")
	if j["status"] != "REVOKED" {
		t.Fatalf("revoke: expected REVOKED, got %v", j["status"])
	}
	// REVOKED 不可逆：再 disable 应 409
	c, _, _ = doReq(t, ts, "POST", "/api/v1/admin/licenses/"+id+"/disable", nil, adminTok)
	if c != 409 {
		t.Fatalf("revoked-then-disable: expected 409, got %d", c)
	}
}

// TestAdmin_RefundOrder 支付订单可退款，重复退款 409。
func TestAdmin_RefundOrder(t *testing.T) {
	ts, _, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)

	ord, err := st.CreateOrder(uid, "plan_test", 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MarkOrderPaid(ord.OrderID); err != nil {
		t.Fatal(err)
	}

	c, j, _ := doReq(t, ts, "POST", "/api/v1/admin/orders/"+ord.OrderID+"/refund", nil, adminTok)
	if c != 200 || j["status"] != "refunded" {
		t.Fatalf("refund: expected 200 refunded, got %d %v", c, j)
	}
	// 重复退款 409
	c, _, _ = doReq(t, ts, "POST", "/api/v1/admin/orders/"+ord.OrderID+"/refund", nil, adminTok)
	if c != 409 {
		t.Fatalf("re-refund: expected 409, got %d", c)
	}
}

// TestAdmin_ActivationCode 生成激活码（12 字符 + 绑定 license）+ 列表 + 作废。
func TestAdmin_ActivationCode(t *testing.T) {
	ts, _, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)

	// 预置 device（激活码要绑 device）
	if err := st.CreateDevice(uid, "HID-TEST0001", ""); err != nil {
		t.Fatal(err)
	}

	// 生成
	c, j, _ := doReq(t, ts, "POST", "/api/v1/admin/activation-codes",
		map[string]string{"user_id": uid, "device_id": "HID-TEST0001", "plan_id": "plan_test"}, adminTok)
	if c != 201 {
		t.Fatalf("create code: expected 201, got %d %v", c, j)
	}
	code, _ := j["code"].(string)
	licID, _ := j["license_id"].(string)
	if len(code) != 12 {
		t.Fatalf("code length: expected 12, got %d (%q)", len(code), code)
	}
	if licID == "" {
		t.Fatalf("expected license_id bound, got empty")
	}
	if strings.ContainsAny(code, "ILOU") {
		t.Fatalf("code should be Crockford base32 (no I/L/O/U): %q", code)
	}

	// 列表含该码
	c, j, _ = doReq(t, ts, "GET", "/api/v1/admin/activation-codes", nil, adminTok)
	if c != 200 {
		t.Fatalf("list codes: expected 200, got %d", c)
	}
	codes, _ := j["codes"].([]any)
	if len(codes) != 1 {
		t.Fatalf("expected 1 code, got %d", len(codes))
	}

	// 作废
	c, _, _ = doReq(t, ts, "POST", "/api/v1/admin/activation-codes/"+code+"/revoke", nil, adminTok)
	if c != 200 {
		t.Fatalf("revoke: expected 200, got %d", c)
	}
	// 重复作废 409
	c, _, _ = doReq(t, ts, "POST", "/api/v1/admin/activation-codes/"+code+"/revoke", nil, adminTok)
	if c != 409 {
		t.Fatalf("re-revoke: expected 409, got %d", c)
	}
}
