// Package api 需求与反馈测试（FB-1）。
// 覆盖：公开提交（happy/honeypot/校验/限频）+ admin 列表过滤与状态流转 + 公开路线图可见性。
//
// 注意：公开端点每 IP 限频 5 次/小时，httptest 请求全部来自 127.0.0.1——
// 每个测试用独立 newTestServer（独立限频器），且单测试内 POST /feedback 不超过 5 次
// （TestFeedback_RateLimit 除外，它专门打满配额）。
package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// postFeedback 提交一条反馈，返回 (status, feedback_id)。
func postFeedback(t *testing.T, ts *httptest.Server, category, title, body, contact string) (int, string) {
	t.Helper()
	code, j, _ := doReq(t, ts, "POST", "/api/v1/feedback",
		map[string]string{"category": category, "title": title, "body": body, "contact": contact}, "")
	id, _ := j["feedback_id"].(string)
	return code, id
}

// TestFeedback_HappyPath 匿名提交成功：201 + fb_ 前缀 + admin 列表可见。
func TestFeedback_HappyPath(t *testing.T) {
	ts, _, _ := newTestServer(t)

	code, id := postFeedback(t, ts, "feature", "希望支持多设备同时控制", "目前一次只能控制一台设备，希望能同时切换多台。", "me@example.com")
	if code != 201 {
		t.Fatalf("expected 201, got %d", code)
	}
	if !strings.HasPrefix(id, "fb_") || len(id) != 3+22 {
		t.Fatalf("expected fb_<22hex>, got %q", id)
	}
}

// TestFeedback_Honeypot 隐藏域非空 → 假 201 但不落库。
func TestFeedback_Honeypot(t *testing.T) {
	ts, _, st := newTestServer(t)
	_, adminTok := adminToken(t, ts, st)

	code, j, _ := doReq(t, ts, "POST", "/api/v1/feedback",
		map[string]string{"category": "feature", "title": "正常标题", "body": "正常内容", "website": "http://spam.example"}, "")
	if code != 201 {
		t.Fatalf("honeypot should fake 201, got %d", code)
	}
	if id, _ := j["feedback_id"].(string); !strings.HasPrefix(id, "fb_") {
		t.Fatalf("honeypot response should look identical, got %v", j)
	}

	// admin 列表应为空
	c, j, _ := doReq(t, ts, "GET", "/api/v1/admin/feedback", nil, adminTok)
	if c != 200 || j["total"] != float64(0) {
		t.Fatalf("honeypot must not persist: %d %v", c, j)
	}
}

// TestFeedback_Validation 服务端权威校验：类目白名单 + 长度上下限（rune 计数）。
func TestFeedback_Validation(t *testing.T) {
	ts, _, _ := newTestServer(t)

	cases := []struct {
		label       string
		category    string
		title, body string
		wantCode    int
	}{
		{"bad category", "spam", "标题三个字", "正文至少五个字", 400},
		{"empty title", "feature", "  ", "正文至少五个字", 400},
		{"title too short", "feature", "两字", "正文至少五个字", 400},
		{"title too long", "feature", strings.Repeat("长", 81), "正文至少五个字", 400},
		{"body too long", "feature", "标题三个字", strings.Repeat("字", 2001), 400},
	}
	// 共 5 次 POST，恰好打满但不超过限频配额（第 6 次才 429）。
	for _, tc := range cases {
		code, _ := postFeedback(t, ts, tc.category, tc.title, tc.body, "")
		if code != tc.wantCode {
			t.Fatalf("%s: expected %d, got %d", tc.label, tc.wantCode, code)
		}
	}
}

// TestFeedback_RateLimit 同 IP 第 6 次提交 → 429。
func TestFeedback_RateLimit(t *testing.T) {
	ts, _, _ := newTestServer(t)

	for i := 1; i <= 5; i++ {
		code, _ := postFeedback(t, ts, "other", "第几条标题", "凑够五个字的正文内容", "")
		if code != 201 {
			t.Fatalf("post %d: expected 201, got %d", i, code)
		}
	}
	code, j, _ := doReq(t, ts, "POST", "/api/v1/feedback",
		map[string]string{"category": "other", "title": "第六条标题", "body": "凑够五个字的正文内容"}, "")
	if code != 429 {
		t.Fatalf("6th post: expected 429, got %d", code)
	}
	if e, _ := j["error"].(string); e != "rate_limited" {
		t.Fatalf("expected error=rate_limited, got %v", j)
	}
}

// TestFeedback_AdminFlow 完整分诊流：提交 → 过滤 new → planned(带备注) → shipped → roadmap 可见
// → rejected → roadmap 不可见；非法 status 400、未知 id 404。
func TestFeedback_AdminFlow(t *testing.T) {
	ts, _, st := newTestServer(t)
	_, adminTok := adminToken(t, ts, st)

	// 1. 提交
	_, id := postFeedback(t, ts, "bug", "配对偶发失败", "特定顺序重启 ControlHub 和固件后，配对偶尔直接失败。", "dev@example.com")

	// 2. admin 按 status=new 过滤，能看到（含审计字段）
	c, j, _ := doReq(t, ts, "GET", "/api/v1/admin/feedback?status=new", nil, adminTok)
	if c != 200 || j["total"] != float64(1) {
		t.Fatalf("list new: %d %v", c, j)
	}
	items := j["items"].([]any)
	item := items[0].(map[string]any)
	if item["feedback_id"] != id {
		t.Fatalf("expected %s, got %v", id, item["feedback_id"])
	}
	if item["client_ip"] != "127.0.0.1" {
		t.Fatalf("expected client_ip captured, got %v", item["client_ip"])
	}
	if ua, _ := item["user_agent"].(string); ua == "" {
		t.Fatalf("expected user_agent captured")
	}

	// 3. set planned + 备注
	c, _, _ = doReq(t, ts, "POST", "/api/v1/admin/feedback/"+id+"/status",
		map[string]string{"status": "planned", "admin_note": "已排入 Phase 4"}, adminTok)
	if c != 200 {
		t.Fatalf("set planned: %d", c)
	}

	// 4. planned 列表含备注；roadmap 已可见
	c, j, _ = doReq(t, ts, "GET", "/api/v1/admin/feedback?status=planned", nil, adminTok)
	if c != 200 || j["total"] != float64(1) {
		t.Fatalf("list planned: %d %v", c, j)
	}
	if got := j["items"].([]any)[0].(map[string]any)["admin_note"]; got != "已排入 Phase 4" {
		t.Fatalf("admin_note: got %v", got)
	}
	c, j, _ = doReq(t, ts, "GET", "/api/v1/feedback/roadmap", nil, "")
	if c != 200 || j["total"] != float64(1) {
		t.Fatalf("roadmap after planned: %d %v", c, j)
	}

	// 5. set shipped → roadmap 仍可见；set rejected → 不可见
	_, _, _ = doReq(t, ts, "POST", "/api/v1/admin/feedback/"+id+"/status",
		map[string]string{"status": "shipped", "admin_note": "v0.2.0 已实现"}, adminTok)
	_, j, _ = doReq(t, ts, "GET", "/api/v1/feedback/roadmap", nil, "")
	if j["total"] != float64(1) {
		t.Fatalf("roadmap after shipped: %v", j)
	}
	if note := j["items"].([]any)[0].(map[string]any)["admin_note"]; note != "v0.2.0 已实现" {
		t.Fatalf("roadmap note: got %v", note)
	}
	_, _, _ = doReq(t, ts, "POST", "/api/v1/admin/feedback/"+id+"/status",
		map[string]string{"status": "rejected", "admin_note": "与现有能力重复"}, adminTok)
	_, j, _ = doReq(t, ts, "GET", "/api/v1/feedback/roadmap", nil, "")
	if j["total"] != float64(0) {
		t.Fatalf("roadmap after rejected should be empty: %v", j)
	}

	// 6. 非法 status / 未知 id
	c, _, _ = doReq(t, ts, "POST", "/api/v1/admin/feedback/"+id+"/status",
		map[string]string{"status": "wow"}, adminTok)
	if c != 400 {
		t.Fatalf("invalid status: expected 400, got %d", c)
	}
	c, _, _ = doReq(t, ts, "POST", "/api/v1/admin/feedback/fb_unknown0000000000000000/status",
		map[string]string{"status": "planned"}, adminTok)
	if c != 404 {
		t.Fatalf("unknown id: expected 404, got %d", c)
	}
}

// TestFeedback_AdminAuth admin 端点鉴权：无 token 401、普通用户 403。
func TestFeedback_AdminAuth(t *testing.T) {
	ts, _, _ := newTestServer(t)

	if c, _, _ := doReq(t, ts, "GET", "/api/v1/admin/feedback", nil, ""); c != 401 {
		t.Fatalf("no token: expected 401, got %d", c)
	}
	_, userTok := registerUser(t, ts)
	if c, _, _ := doReq(t, ts, "GET", "/api/v1/admin/feedback", nil, userTok); c != 403 {
		t.Fatalf("non-admin: expected 403, got %d", c)
	}
}

// TestFeedback_RoadmapEmpty 空库时 roadmap 返回 items=[]（非 null）。
func TestFeedback_RoadmapEmpty(t *testing.T) {
	ts, _, _ := newTestServer(t)
	c, j, _ := doReq(t, ts, "GET", "/api/v1/feedback/roadmap", nil, "")
	if c != 200 {
		t.Fatalf("expected 200, got %d", c)
	}
	items, ok := j["items"].([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("expected empty array, got %v", j["items"])
	}
}
