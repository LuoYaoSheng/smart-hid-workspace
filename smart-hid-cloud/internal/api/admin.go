// Package api admin 子系统（CL-5a 起，CL-5b 充实）。
//
// 所有 admin endpoint 挂在 /api/v1/admin/* 下，经 AdminAuthMiddleware（JWT + role==admin）保护。
// 跨用户查询 / 状态操作 / 套餐维护 / 激活码生成。
//
// V1 scope：看 + 禁用/吊销 license + 退款订单 + 套餐上下架/新建 + 激活码生成/作废。
// 不做：编辑用户邮箱密码、用户禁用（JWT 无状态，已签发 token 仍有效，价值低）。
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"smart-hid-cloud/internal/store"
)

// registerAdminRoutes 注册所有 /api/v1/admin/* 路由。
func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/admin/stats", s.handleAdminStats)
	mux.HandleFunc("/api/v1/admin/users", s.handleAdminUsers)
	mux.HandleFunc("/api/v1/admin/users/", s.handleAdminUserDetail) // /users/{id}
	mux.HandleFunc("/api/v1/admin/orders", s.handleAdminOrders)
	mux.HandleFunc("/api/v1/admin/orders/", s.handleAdminOrderAction) // /orders/{id}/refund
	mux.HandleFunc("/api/v1/admin/licenses", s.handleAdminLicenses)
	mux.HandleFunc("/api/v1/admin/licenses/", s.handleAdminLicenseAction) // /licenses/{id}/{disable|revoke|re-enable}
	mux.HandleFunc("/api/v1/admin/plans", s.handleAdminPlans)
	mux.HandleFunc("/api/v1/admin/plans/", s.handleAdminPlanAction) // /plans/{id}/{activate|deactivate}
	mux.HandleFunc("/api/v1/admin/activation-codes", s.handleAdminActivationCodes)
	mux.HandleFunc("/api/v1/admin/activation-codes/", s.handleAdminActivationCodeAction) // /{code}/revoke
	mux.HandleFunc("/api/v1/admin/feedback", s.handleAdminFeedback)                      // FB-1 反馈列表
	mux.HandleFunc("/api/v1/admin/feedback/", s.handleAdminFeedbackAction)               // /{id}/status
}

// ----- Stats -----

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	st, err := s.store.GetAdminStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// ----- Users -----

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	users, err := s.store.ListAllUsers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	if users == nil {
		users = []store.User{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users, "total": len(users)})
}

func (s *Server) handleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	userID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing user_id"})
		return
	}
	u, err := s.store.GetUserByID(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "user not found"})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// ----- Orders -----

func (s *Server) handleAdminOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	orders, err := s.store.ListAllOrders()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	if orders == nil {
		orders = []store.Order{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": orders, "total": len(orders)})
}

func (s *Server) handleAdminOrderAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/orders/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing order_id"})
		return
	}
	orderID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if action != "refund" || r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST /orders/{id}/refund only"})
		return
	}
	if err := s.store.RefundOrder(orderID); err != nil {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "order not paid or already refunded"})
		return
	}
	s.log.Info("order refunded (admin)", "order_id", orderID, "admin", UserIDFromCtx(r.Context()))
	writeJSON(w, http.StatusOK, map[string]any{"order_id": orderID, "status": "refunded"})
}

// ----- Licenses -----

func (s *Server) handleAdminLicenses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	lics, err := s.store.ListAllLicenses()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	if lics == nil {
		lics = []store.LicenseRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"licenses": lics, "total": len(lics)})
}

func (s *Server) handleAdminLicenseAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/licenses/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing license_id"})
		return
	}
	licenseID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}

	// REVOKED 不可逆：先查当前状态
	cur, err := s.store.GetLicense(licenseID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "license not found"})
		return
	}
	if cur.Status == "REVOKED" {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "license is REVOKED (irreversible)"})
		return
	}

	var target string
	switch action {
	case "extend":
		// 续期：重签同 license_id，延长 addDays 天（CL-6a）。仅 ACTIVE 可续期。
		if cur.Status != "ACTIVE" {
			writeJSON(w, http.StatusConflict, errBody{"conflict", "extend requires ACTIVE license (current: " + cur.Status + ")"})
			return
		}
		var req struct {
			AddDays int `json:"add_days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
			return
		}
		if req.AddDays <= 0 {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "add_days must be positive"})
			return
		}
		signed, err := s.reissueLicense(cur, req.AddDays)
		if err != nil {
			s.log.Error("admin extend license", "license_id", licenseID, "err", err)
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
			return
		}
		s.log.Info("license extended (admin)", "license_id", licenseID,
			"add_days", req.AddDays, "new_expires_at", signed.Payload.ExpiresAt,
			"admin", UserIDFromCtx(r.Context()))
		writeJSON(w, http.StatusOK, map[string]any{
			"license_id": licenseID,
			"status":     "ACTIVE",
			"expires_at": signed.Payload.ExpiresAt,
			"issued_at":  signed.Payload.IssuedAt,
		})
		return
	case "disable":
		target = "DISABLED"
	case "revoke":
		target = "REVOKED"
	case "re-enable":
		target = "ACTIVE"
	default:
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "unknown action: " + action})
		return
	}
	if err := s.store.SetLicenseStatus(licenseID, target); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	s.log.Info("license status changed (admin)", "license_id", licenseID,
		"from", cur.Status, "to", target, "admin", UserIDFromCtx(r.Context()))
	writeJSON(w, http.StatusOK, map[string]any{"license_id": licenseID, "status": target})
}

// ----- Plans -----

func (s *Server) handleAdminPlans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		plans, err := s.store.ListPlans(false) // 全部，含 inactive
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
			return
		}
		if plans == nil {
			plans = []store.Plan{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"plans": plans, "total": len(plans)})
	case http.MethodPost:
		var p store.Plan
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
			return
		}
		if p.PlanID == "" || p.Name == "" || p.DurationDays <= 0 {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "plan_id, name, duration_days required"})
			return
		}
		// 新建默认 active
		if err := s.store.UpsertPlan(p); err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, p)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET/POST only"})
	}
}

func (s *Server) handleAdminPlanAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/plans/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing plan_id"})
		return
	}
	planID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	var active bool
	switch action {
	case "activate":
		active = true
	case "deactivate":
		active = false
	default:
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "unknown action: " + action})
		return
	}
	if err := s.store.SetPlanActive(planID, active); err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "plan not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan_id": planID, "active": active})
}

// ----- Activation Codes -----

func (s *Server) handleAdminActivationCodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		codes, err := s.store.ListActivationCodes()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
			return
		}
		if codes == nil {
			codes = []store.ActivationCode{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"codes": codes, "total": len(codes)})
	case http.MethodPost:
		var req struct {
			UserID   string `json:"user_id"`
			DeviceID string `json:"device_id"`
			PlanID   string `json:"plan_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
			return
		}
		if req.UserID == "" || req.PlanID == "" {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "user_id, plan_id required (device_id optional — empty = generic code, binds at consume)"})
			return
		}
		plan, err := s.store.GetPlan(req.PlanID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "plan not found"})
			return
		}
		// 激活码有效期 90 天（license 本身在被消费激活时才计算有效期）
		expiresAt := time.Now().Unix() + 90*86400
		code, err := s.store.CreateActivationCode(req.UserID, req.DeviceID, req.PlanID, plan.Features, expiresAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
			return
		}
		s.log.Info("activation code created (admin)", "code", code.Code,
			"license_id", code.LicenseID, "admin", UserIDFromCtx(r.Context()))
		writeJSON(w, http.StatusCreated, code)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET/POST only"})
	}
}

func (s *Server) handleAdminActivationCodeAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/activation-codes/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing code"})
		return
	}
	code := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if action != "revoke" || r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST /activation-codes/{code}/revoke only"})
		return
	}
	if err := s.store.RevokeActivationCode(code); err != nil {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "code already used or not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "revoked": true})
}
