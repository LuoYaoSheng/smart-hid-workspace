// Package api admin 鉴权测试（CL-5a）。
// 覆盖：无 token 401、普通用户 403、promote 后重新登录拿到 admin token 才能访问。
package api

import (
	"net/http/httptest"
	"testing"
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
	if j["ok"] != true {
		t.Fatalf("expected ok=true, got %v", j["ok"])
	}
}
