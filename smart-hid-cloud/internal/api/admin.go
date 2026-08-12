// Package api admin 子系统（CL-5a 起）。
//
// 所有 admin endpoint 挂在 /api/v1/admin/* 下，经 AdminAuthMiddleware（JWT + role==admin）保护。
// CL-5a：仅 stats 占位验证鉴权链路；CL-5b 起充实用户/订单/License/套餐/激活码管理。
package api

import "net/http"

// registerAdminRoutes 注册所有 /api/v1/admin/* 路由。
func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/admin/stats", s.handleAdminStats)
}

// handleAdminStats GET /api/v1/admin/stats —— CL-5a 占位，证明鉴权链路通；CL-5b 填真实统计。
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"admin": UserIDFromCtx(r.Context()),
		"note":  "CL-5a placeholder; full stats arrive in CL-5b",
	})
}
