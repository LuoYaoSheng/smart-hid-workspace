package api

import (
	"net/http"
	"strings"

	"smart-hid-controlhub/internal/trial"
)

// handleUsage GET /api/v1/usage —— 当前 Trial 用量。
// 默认查询参数 device_id（若空且只有一台设备，自动选那台；否则 400）。
//
// 设计源：docs/04 §9 GET /usage + docs/10 验收 D5（Trial expired 后配置仍可用，
// 此 endpoint 属于"查询"路径，不走 Entitlement 闸门，始终返回用量信息）。
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	if s.trialMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody{"unavailable", "trial module not initialized"})
		return
	}
	// device_id 从 query 取；空则尝试用唯一在线设备
	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		devs := s.devices.List()
		if len(devs) == 1 {
			deviceID = devs[0].DeviceID
		} else if len(devs) == 0 {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "device_id required (no devices paired)"})
			return
		} else {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "device_id required (multiple devices)"})
			return
		}
	}
	usage := s.trialMgr.Usage(deviceID)
	writeJSON(w, http.StatusOK, usage)
}

// handleUsageAll GET /api/v1/usage/all —— 列出所有设备用量（管理用）。
func (s *Server) handleUsageAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	if s.trialMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody{"unavailable", "trial module not initialized"})
		return
	}
	devs := s.devices.List()
	out := make([]trial.Usage, 0, len(devs))
	for _, d := range devs {
		out = append(out, s.trialMgr.Usage(d.DeviceID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"usages": out, "total": len(out)})
}

// 引用 strings 避免 unused（路径处理时可能扩展）
var _ = strings.HasPrefix
