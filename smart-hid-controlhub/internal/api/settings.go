package api

import (
	"encoding/json"
	"net/http"
)

// lanModeResp GET/POST /api/v1/settings/lan-mode 响应。
type lanModeResp struct {
	Enabled bool   `json:"enabled"`
	Note    string `json:"note"`
}

// handleSettingsLAN GET 返回当前 LAN 模式开关；POST 切换（需重启生效）。
// 验收清单 A11 "LAN API 需要显式开启"。
//
// 注：HTTP listener 的 bind host 不支持运行时切换（Go http.Server 限制），
// 因此切换只持久化到 settings 表，下次 ControlHub 启动时 Build 会读取应用。
func (s *Server) handleSettingsLAN(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, lanModeResp{
			Enabled: s.settings.GetBool("lan_mode_enabled", false),
			Note:    "toggle via POST; change takes effect after restart",
		})

	case http.MethodPost:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "invalid json: " + err.Error()})
			return
		}
		if err := s.settings.SetBool("lan_mode_enabled", body.Enabled); err != nil {
			s.log.Error("set lan mode", "err", err)
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", "persist failed"})
			return
		}
		s.log.Info("lan mode toggled", "enabled", body.Enabled, "note", "restart required")
		writeJSON(w, http.StatusOK, lanModeResp{
			Enabled: body.Enabled,
			Note:    "setting persisted; restart ControlHub to apply",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET/POST only"})
	}
}
