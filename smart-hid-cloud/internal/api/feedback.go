// Package api 需求与反馈 endpoints（FB-1）。
//
// 公开（无 JWT）：
//
//	POST /api/v1/feedback            匿名提交（honeypot + 每 IP 限频 + 长度硬上限）
//	GET  /api/v1/feedback/roadmap    已采纳条目（planned/shipped），供路线图页展示
//
// Admin（JWT + role==admin）：
//
//	GET  /api/v1/admin/feedback      列表（可按状态过滤）
//	POST /api/v1/admin/feedback/{id}/status   改状态 + 备注
//
// 设计源：docs/feedback.md。
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"smart-hid-cloud/internal/store"
)

// 反馈状态机（admin 自由流转）与类目白名单。
var (
	feedbackStatuses   = map[string]bool{"new": true, "planned": true, "shipped": true, "rejected": true}
	feedbackCategories = map[string]bool{"feature": true, "bug": true, "other": true}
)

// 字段长度硬上限（rune 计数；前端提示与服务端权威一致，但服务端才是边界）。
const (
	feedbackTitleMax   = 80
	feedbackBodyMin    = 5
	feedbackBodyMax    = 2000
	feedbackContactMax = 120
	feedbackUAMax      = 250
)

// ----- 公开端点 -----

// feedbackReq 公开提交 body。Website 是 honeypot 隐藏域：正常用户永远为空。
type feedbackReq struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Contact  string `json:"contact"`
	Website  string `json:"website"` // honeypot：非空 = bot
}

// handleFeedbackCreate POST /api/v1/feedback（公开，匿名可提交）。
func (s *Server) handleFeedbackCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}

	// 限频（方法守卫后立刻检查，解码失败也计数——垃圾请求同样消耗配额）
	ip := clientIP(r)
	if !s.fbLimiter.allow(ip, time.Now().Unix()) {
		writeJSON(w, http.StatusTooManyRequests, errBody{"rate_limited", "提交太频繁，请 1 小时后再试"})
		return
	}

	var req feedbackReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
		return
	}

	// honeypot：非空视为 bot。返与真实成功无差别的 201（不暴露检测）。
	if strings.TrimSpace(req.Website) != "" {
		s.log.Warn("feedback honeypot triggered", "ip", ip, "ua", r.Header.Get("User-Agent"))
		writeJSON(w, http.StatusCreated, map[string]any{"feedback_id": fakeFeedbackID(), "status": "new"})
		return
	}

	// 校验（trim + rune 计数）
	req.Category = strings.TrimSpace(req.Category)
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	req.Contact = strings.TrimSpace(req.Contact)
	if !feedbackCategories[req.Category] {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "category must be feature|bug|other"})
		return
	}
	if n := utf8.RuneCountInString(req.Title); n < 3 || n > feedbackTitleMax {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "title must be 3-80 characters"})
		return
	}
	if n := utf8.RuneCountInString(req.Body); n < feedbackBodyMin || n > feedbackBodyMax {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "body must be 5-2000 characters"})
		return
	}
	if utf8.RuneCountInString(req.Contact) > feedbackContactMax {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "contact too long (max 120)"})
		return
	}

	ua := r.Header.Get("User-Agent")
	if utf8.RuneCountInString(ua) > feedbackUAMax {
		ua = string([]rune(ua)[:feedbackUAMax])
	}

	fb, err := s.store.CreateFeedback(req.Category, req.Title, req.Body, req.Contact, ip, ua)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	s.log.Info("feedback created", "feedback_id", fb.FeedbackID, "category", fb.Category, "ip", ip)
	writeJSON(w, http.StatusCreated, map[string]any{"feedback_id": fb.FeedbackID, "status": fb.Status})
}

// handleFeedbackRoadmap GET /api/v1/feedback/roadmap（公开）。
// 仅返回 admin 甄别过的 planned/shipped 条目（不含 contact/ip/ua）。
func (s *Server) handleFeedbackRoadmap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	items, err := s.store.ListPublicFeedback()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	if items == nil {
		items = []store.PublicFeedback{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// ----- Admin 端点 -----

// handleAdminFeedback GET /api/v1/admin/feedback?status=new|planned|shipped|rejected（可省略 = 全部）。
func (s *Server) handleAdminFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && !feedbackStatuses[status] {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "invalid status filter"})
		return
	}
	items, err := s.store.ListFeedback(status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	if items == nil {
		items = []store.Feedback{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handleAdminFeedbackAction POST /api/v1/admin/feedback/{id}/status，body {status, admin_note?}。
func (s *Server) handleAdminFeedbackAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/feedback/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[1] != "status" || r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST /admin/feedback/{id}/status only"})
		return
	}
	id := parts[0]

	var req struct {
		Status    string `json:"status"`
		AdminNote string `json:"admin_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
		return
	}
	if !feedbackStatuses[req.Status] {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "status must be new|planned|shipped|rejected"})
		return
	}
	if utf8.RuneCountInString(req.AdminNote) > 500 {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "admin_note too long (max 500)"})
		return
	}

	if err := s.store.SetFeedbackStatus(id, req.Status, strings.TrimSpace(req.AdminNote)); err != nil {
		if err == store.ErrNotFound {
			writeJSON(w, http.StatusNotFound, errBody{"not_found", "feedback not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	s.log.Info("feedback status set", "feedback_id", id, "status", req.Status,
		"admin", UserIDFromCtx(r.Context()))
	writeJSON(w, http.StatusOK, map[string]any{"feedback_id": id, "status": req.Status})
}

// fakeFeedbackID 生成格式合法但不落库的假 ID（honeypot 响应与真实响应不可区分）。
func fakeFeedbackID() string {
	b := make([]byte, 11)
	if _, err := rand.Read(b); err != nil {
		return "fb_0000000000000000000000"
	}
	return "fb_" + hex.EncodeToString(b)
}
