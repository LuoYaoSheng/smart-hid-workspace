package api

import (
	"encoding/json"
	"io"
	"net/http"

	lic "smart-hid-cloud/pkg/license"
	"smart-hid-controlhub/internal/license"
)

// handleLicenseStatus GET /api/v1/license?device_id=X
// 返回该设备的当前 license 状态（验签 + 时效）。
func (s *Server) handleLicenseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	if s.licenseMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody{"unavailable", "license module not initialized"})
		return
	}
	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		// 自动选唯一设备
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
	info, err := s.licenseMgr.LoadForDevice(deviceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleLicenseImport POST /api/v1/license/import
// Body: 完整 License JSON（.license 文件内容）+ device_id（query 或 body.device_id）
// 用于离线场景：用户从 Cloud Web 下载 .license 后拷贝到 ControlHub 导入。
//
// 验收 E5 离线可导入。
func (s *Server) handleLicenseImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	if s.licenseMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody{"unavailable", "license module not initialized"})
		return
	}
	// 读 raw body（License JSON）
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", err.Error()})
		return
	}
	if len(raw) == 0 {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "empty body"})
		return
	}
	// 先 decode 拿 device_id（payload 内的）
	var l lic.License
	if err := json.Unmarshal(raw, &l); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "invalid license JSON: " + err.Error()})
		return
	}
	deviceID := l.Payload.DeviceID
	if deviceID == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "license missing device_id"})
		return
	}
	// 调 mgr.Import（含 VerifyFull + upsert）
	imported, err := s.licenseMgr.Import(raw, deviceID)
	if err != nil {
		s.log.Warn("license import failed", "device_id", deviceID, "err", err)
		writeJSON(w, http.StatusBadRequest, errBody{"import_failed", err.Error()})
		return
	}
	s.log.Info("license imported via /license/import",
		"license_id", imported.Payload.LicenseID, "device_id", deviceID)
	info, _ := s.licenseMgr.LoadForDevice(deviceID)
	writeJSON(w, http.StatusOK, map[string]any{
		"imported": true,
		"license":  imported.Payload.LicenseID,
		"device":   deviceID,
		"status":   info.Status,
	})
}

// handleLicenseList GET /api/v1/license/list
// 返回所有已导入的 license（管理用，不重新验签）。
func (s *Server) handleLicenseList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	if s.licenseMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody{"unavailable", "license module not initialized"})
		return
	}
	list, err := s.licenseMgr.ListAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", err.Error()})
		return
	}
	if list == nil {
		list = []licmgr.LicenseInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"licenses": list, "total": len(list)})
}
