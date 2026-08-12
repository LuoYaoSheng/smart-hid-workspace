// Package api: 激活码消费 + License 刷新（CL-6a）。
//
// 这两个 endpoint 故意 PUBLIC（不走 JWTAuth）：
//   - POST /api/v1/activation/consume  凭据 = 12 字符 Crockford base32 激活码（≈10^18 空间）
//   - POST /api/v1/license/refresh     凭据 = 不可猜测的 license_id（lic_<22hex>）
//
// 标准离线激活模型（类比产品密钥）。设备绑定由 License 签名强制：
// 返回的 License 内嵌 device_id，ControlHub 端 VerifyFull 校验，换设备无法冒用。
// Phase 7 生产安全再上设备证书 / CRL 实时吊销。
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"smart-hid-cloud/internal/store"
	"smart-hid-cloud/pkg/license"
)

// crockfordNormalize 把用户输入的激活码归一化为存储形式：
// 大写 + 去 -/空格 + 易混字符映射（I/L→1, O→0, U→V）。Crockford base32 无 I/L/O/U。
func crockfordNormalize(s string) string {
	s = strings.ToUpper(s)
	s = strings.NewReplacer("-", "", " ", "").Replace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range s {
		switch c {
		case 'I', 'L':
			b.WriteByte('1')
		case 'O':
			b.WriteByte('0')
		case 'U':
			b.WriteByte('V')
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// signAndActivate 签发并把一个 UNUSED license 激活到指定 device（CL-6a 抽出，JWT activate + consume 共用）。
// 调用方负责授权检查：JWT 流程查设备归属；consume 流程靠激活码绑定。
// license 必须 UNUSED；ActivateLicense 的 WHERE status='UNUSED' 兜底并发安全。
func (s *Server) signAndActivate(lic store.LicenseRow, deviceID string) (license.License, error) {
	plan, err := s.store.GetPlan(lic.PlanID)
	if err != nil {
		return license.License{}, fmt.Errorf("plan vanished: %w", err)
	}
	now := time.Now().Unix()
	expiresAt := now + int64(plan.DurationDays)*86400
	payload := license.Payload{
		LicenseID:      lic.LicenseID,
		AccountID:      lic.UserID,
		PlanID:         plan.PlanID,
		DeviceID:       deviceID,
		IssuedAt:       now,
		ValidFrom:      now,
		ExpiresAt:      expiresAt,
		Features:       plan.Features,
		LicenseVersion: license.Version,
	}
	signed, err := license.Sign(payload, s.privateKey)
	if err != nil {
		return license.License{}, fmt.Errorf("sign: %w", err)
	}
	payloadJSON, _ := license.Encode(signed)
	if err := s.store.ActivateLicense(lic.LicenseID, deviceID, now, expiresAt, string(payloadJSON), signed.Signature); err != nil {
		return license.License{}, fmt.Errorf("activate: %w", err)
	}
	return signed, nil
}

// reissueLicense 续期重签一个已 ACTIVE 的 license，延长 addDays 天（CL-6a admin extend 用）。
// 保留原 valid_from / device 绑定，只推进 expires_at + 换新 issued_at + 新签名。
func (s *Server) reissueLicense(cur store.LicenseRow, addDays int) (license.License, error) {
	if cur.ValidFrom == nil || cur.ExpiresAt == nil || cur.DeviceID == "" {
		return license.License{}, fmt.Errorf("license not fully bound (missing valid_from/expires_at/device_id)")
	}
	now := time.Now().Unix()
	newExpires := *cur.ExpiresAt + int64(addDays)*86400
	payload := license.Payload{
		LicenseID:      cur.LicenseID,
		AccountID:      cur.UserID,
		PlanID:         cur.PlanID,
		DeviceID:       cur.DeviceID,
		IssuedAt:       now,
		ValidFrom:      *cur.ValidFrom,
		ExpiresAt:      newExpires,
		Features:       cur.Features,
		LicenseVersion: license.Version,
	}
	signed, err := license.Sign(payload, s.privateKey)
	if err != nil {
		return license.License{}, fmt.Errorf("sign: %w", err)
	}
	payloadJSON, _ := license.Encode(signed)
	if err := s.store.ReissueLicense(cur.LicenseID, newExpires, now, string(payloadJSON), signed.Signature); err != nil {
		return license.License{}, fmt.Errorf("reissue: %w", err)
	}
	return signed, nil
}

// ----- consume -----

type consumeReq struct {
	Code     string `json:"code"`
	DeviceID string `json:"device_id"`
}

// handleActivationConsume POST /api/v1/activation/consume  （PUBLIC，激活码即凭据）
//
// 请求体 {code, device_id}。成功返回签名 License JSON（ControlHub 直接 Import）。
//
// 流程：归一化码 → 查码 → 校验未用/未过期/设备绑定 → 查 license 必须 UNUSED →
// signAndActivate → 标记码 used → 返回签名 License。
//
// 设备绑定双模式：码生成时 device_id 非空 → 消费时必须匹配；为空 → 绑定消费设备。
func (s *Server) handleActivationConsume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	var req consumeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
		return
	}
	req.Code = crockfordNormalize(req.Code)
	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing code"})
		return
	}
	if !deviceIDPattern.MatchString(req.DeviceID) {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "invalid device_id (expect HID-XXXXXXXX)"})
		return
	}

	code, err := s.store.GetActivationCode(req.Code)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "activation code not found"})
		return
	}
	if code.UsedAt != nil {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "activation code already used"})
		return
	}
	if time.Now().Unix() > code.ExpiresAt {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "activation code expired"})
		return
	}
	// 设备绑定校验
	if code.DeviceID != "" && code.DeviceID != req.DeviceID {
		writeJSON(w, http.StatusConflict, errBody{"device_mismatch",
			fmt.Sprintf("code is bound to %s, requested %s", code.DeviceID, req.DeviceID)})
		return
	}

	lic, err := s.store.GetLicense(code.LicenseID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "license for code not found"})
		return
	}
	if lic.Status != "UNUSED" {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "license not UNUSED (status=" + lic.Status + ")"})
		return
	}

	signed, err := s.signAndActivate(lic, req.DeviceID)
	if err != nil {
		s.log.Error("consume: signAndActivate", "code", req.Code, "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	// 标记码已消费。即使此处失败（极少），license 已激活、码也是一次性的，仅记日志。
	if err := s.store.MarkActivationCodeUsed(req.Code); err != nil {
		s.log.Warn("consume: mark code used failed (license already activated)", "code", req.Code, "err", err)
	}
	s.log.Info("activation code consumed", "code", req.Code,
		"license_id", lic.LicenseID, "device_id", req.DeviceID)

	// 返回签名 License JSON（与 /licenses/{id}/download 同形，ControlHub 直接 Import）
	raw, _ := license.Encode(signed)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// ----- refresh -----

type refreshReq struct {
	LicenseID string `json:"license_id"`
}

// handleLicenseRefresh POST /api/v1/license/refresh  （PUBLIC，license_id 即凭据）
//
// 请求体 {license_id}。成功返回该 license 最新签名 JSON（ControlHub 覆盖导入）。
// 续期后（admin extend）ControlHub 用原 license_id 刷新即可拿到新 expires_at。
//
// 只读：仅返回 Cloud 已签发的 payload_json。设备绑定由签名强制。
func (s *Server) handleLicenseRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	var req refreshReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
		return
	}
	if req.LicenseID == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing license_id"})
		return
	}

	lic, err := s.store.GetLicense(req.LicenseID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "license not found"})
		return
	}
	if lic.Status != "ACTIVE" || lic.PayloadJSON == "" {
		writeJSON(w, http.StatusConflict, errBody{"conflict", "license not active (status=" + lic.Status + ")"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(lic.PayloadJSON))
}
