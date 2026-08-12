// Package api: 业务 endpoint（CL-2b）。
//
// 路由组织：
//   公开（无需 JWT）：/api/v1/health, /api/v1/auth/{register,login}, /api/v1/plans
//   受保护（需 JWT）：/api/v1/devices, /api/v1/orders, /api/v1/licenses, /api/v1/users/me
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"smart-hid-cloud/internal/auth"
	"smart-hid-cloud/internal/store"
	"smart-hid-cloud/pkg/license"
)

// deviceIDPattern 与 ControlHub / 协议 schema 一致。
var deviceIDPattern = regexp.MustCompile(`^HID-[A-Z0-9]{8}$`)

// emailPattern 简易 email 格式。
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// ---------- Auth ----------

type authReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResp struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Token  string `json:"token"`
}

// handleRegister POST /api/v1/auth/register
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	var req authReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !emailPattern.MatchString(req.Email) {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "invalid email"})
		return
	}
	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "password >= 8 chars"})
		return
	}
	user, err := s.store.CreateUser(req.Email, req.Password)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeJSON(w, http.StatusConflict, errBody{"duplicate", "email already registered"})
			return
		}
		s.log.Error("create user", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	tok, _ := auth.Sign(auth.Claims{UserID: user.UserID, Role: user.Role}, s.jwtSecret)
	writeJSON(w, http.StatusCreated, authResp{UserID: user.UserID, Email: user.Email, Token: tok})
}

// handleLogin POST /api/v1/auth/login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	var req authReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	user, err := s.store.VerifyPassword(req.Email, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errBody{"unauthorized", "invalid credentials"})
		return
	}
	tok, _ := auth.Sign(auth.Claims{UserID: user.UserID, Role: user.Role}, s.jwtSecret)
	writeJSON(w, http.StatusOK, authResp{UserID: user.UserID, Email: user.Email, Token: tok})
}

// ---------- Plans ----------

// handleListPlans GET /api/v1/plans
func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	plans, err := s.store.ListPlans(true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	if plans == nil {
		plans = []store.Plan{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans, "total": len(plans)})
}

// ---------- Devices ----------

type deviceReq struct {
	DeviceID    string `json:"device_id"`
	DisplayName string `json:"display_name"`
}

// handleDevices POST 注册 / GET 列表，按 method 分发。
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())
	switch r.Method {
	case http.MethodPost:
		var req deviceReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
			return
		}
		if !deviceIDPattern.MatchString(req.DeviceID) {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "device_id must match ^HID-[A-Z0-9]{8}$"})
			return
		}
		if err := s.store.CreateDevice(userID, req.DeviceID, req.DisplayName); err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"device_id": req.DeviceID, "user_id": userID, "display_name": req.DisplayName,
		})
	case http.MethodGet:
		devs, err := s.store.ListDevicesByUser(userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
			return
		}
		if devs == nil {
			devs = []store.Device{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": devs, "total": len(devs)})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET/POST only"})
	}
}

// ---------- Orders ----------

type orderReq struct {
	PlanID string `json:"plan_id"`
}

// handleOrders POST 创建 / GET 列表。
func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())
	switch r.Method {
	case http.MethodPost:
		var req orderReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
			return
		}
		plan, err := s.store.GetPlan(req.PlanID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, errBody{"not_found", "plan not found"})
			return
		}
		if !plan.Active {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "plan not active"})
			return
		}
		order, err := s.store.CreateOrder(userID, plan.PlanID, plan.PriceCents)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, order)
	case http.MethodGet:
		orders, err := s.store.ListOrdersByUser(userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
			return
		}
		if orders == nil {
			orders = []store.Order{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"orders": orders, "total": len(orders)})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET/POST only"})
	}
}

// handlePayCallback POST /api/v1/orders/{id}/pay-callback
// V1 mock：无真实支付网关验证，调用即标记 paid + 自动创建 UNUSED license。
// 接真实支付时此 endpoint 替换为支付网关 webhook。
func (s *Server) handlePayCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	userID := UserIDFromCtx(r.Context())
	orderID := strings.TrimPrefix(r.URL.Path, "/api/v1/orders/")
	orderID = strings.TrimSuffix(orderID, "/pay-callback")
	if orderID == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing order_id"})
		return
	}
	order, err := s.store.GetOrder(orderID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "order not found"})
		return
	}
	if order.UserID != userID {
		writeJSON(w, http.StatusForbidden, errBody{"forbidden", "not your order"})
		return
	}
	if err := s.store.MarkOrderPaid(orderID); err != nil {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "order not pending or already paid"})
		return
	}
	plan, err := s.store.GetPlan(order.PlanID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "plan vanished"})
		return
	}
	lic, err := s.store.CreateLicense(userID, plan.PlanID, orderID, plan.Features)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	s.log.Info("order paid + license created", "order_id", orderID, "license_id", lic.LicenseID)
	writeJSON(w, http.StatusOK, map[string]any{
		"order":   order.OrderID,
		"status":  "paid",
		"license": lic,
	})
}

// ---------- Licenses ----------

// handleLicenses GET 列表。
func (s *Server) handleLicenses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	userID := UserIDFromCtx(r.Context())
	lics, err := s.store.ListLicensesByUser(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	if lics == nil {
		lics = []store.LicenseRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"licenses": lics, "total": len(lics)})
}

type activateReq struct {
	DeviceID string `json:"device_id"`
}

// handleLicenseAction 路由 GET/{activate,download}。
// path: /api/v1/licenses/{id} 或 /api/v1/licenses/{id}/{action}
func (s *Server) handleLicenseAction(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromCtx(r.Context())
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/licenses/")
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

	lic, err := s.store.GetLicense(licenseID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "license not found"})
		return
	}
	if lic.UserID != userID {
		writeJSON(w, http.StatusForbidden, errBody{"forbidden", "not your license"})
		return
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		// GET 详情
		writeJSON(w, http.StatusOK, lic)
	case action == "activate" && r.Method == http.MethodPost:
		s.handleActivateLicense(w, r, lic)
	case action == "download" && r.Method == http.MethodGet:
		s.handleDownloadLicense(w, lic)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "see API docs"})
	}
}

func (s *Server) handleActivateLicense(w http.ResponseWriter, r *http.Request, lic store.LicenseRow) {
	userID := UserIDFromCtx(r.Context())
	if lic.Status != "UNUSED" {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "license not UNUSED (status=" + lic.Status + ")"})
		return
	}
	var req activateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
		return
	}
	if !deviceIDPattern.MatchString(req.DeviceID) {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "invalid device_id"})
		return
	}
	// 校验 device 属于本 user
	devs, err := s.store.ListDevicesByUser(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	deviceOK := false
	for _, d := range devs {
		if d.DeviceID == req.DeviceID {
			deviceOK = true
			break
		}
	}
	if !deviceOK {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "device not registered under your account"})
		return
	}

	plan, err := s.store.GetPlan(lic.PlanID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "plan vanished"})
		return
	}
	now := time.Now().Unix()
	expiresAt := now + int64(plan.DurationDays)*86400

	// 构造 Payload（保留 DB 已有 license_id，不用 NewPayload 工厂避免重新生成 ID）
	payload := license.Payload{
		LicenseID:      lic.LicenseID,
		AccountID:      userID,
		PlanID:         plan.PlanID,
		DeviceID:       req.DeviceID,
		IssuedAt:       now,
		ValidFrom:      now,
		ExpiresAt:      expiresAt,
		Features:       plan.Features,
		LicenseVersion: license.Version,
	}
	signed, err := license.Sign(payload, s.privateKey)
	if err != nil {
		s.log.Error("license sign", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "sign failed"})
		return
	}
	payloadJSON, _ := license.Encode(signed)
	if err := s.store.ActivateLicense(lic.LicenseID, req.DeviceID, now, expiresAt, string(payloadJSON), signed.Signature); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	s.log.Info("license activated", "license_id", lic.LicenseID, "device_id", req.DeviceID, "expires_at", expiresAt)
	writeJSON(w, http.StatusOK, signed)
}

func (s *Server) handleDownloadLicense(w http.ResponseWriter, lic store.LicenseRow) {
	if lic.Status != "ACTIVE" || lic.PayloadJSON == "" {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "license not active"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+lic.LicenseID+`.license"`)
	_, _ = w.Write([]byte(lic.PayloadJSON))
}

// handleMe GET /api/v1/users/me —— 测试 JWT 是否有效。
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	userID := UserIDFromCtx(r.Context())
	user, err := s.store.GetUserByID(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "user gone"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}
