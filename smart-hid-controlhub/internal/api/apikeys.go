package api

import (
	"encoding/json"
	"net/http"

	"smart-hid-controlhub/internal/apikey"
)

// handleAPIKeysList GET /api/v1/api-keys —— 列出所有 key（不含明文）。
func (s *Server) handleAPIKeysList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	keys, err := s.keys.List()
	if err != nil {
		s.log.Error("api-keys list", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "list failed"})
		return
	}
	if keys == nil {
		keys = []apikey.KeyInfo{} // 避免 null
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys, "total": len(keys)})
}

// rotateReq POST /api/v1/api-keys/rotate 请求体。
type rotateReq struct {
	Label string `json:"label,omitempty"`
}

// rotateResp 返回一次性的新 key 明文。
type rotateResp struct {
	APIKey string `json:"api_key"`
	Label  string `json:"label"`
	Note   string `json:"note"`
}

// handleAPIKeysRotate POST /api/v1/api-keys/rotate
// 撤销所有当前 active key，生成新 key，返明文一次。
// 验收清单 A12 "API Key 可重新生成"。
func (s *Server) handleAPIKeysRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	var body rotateReq
	// 空 body 合法（label 走默认）
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "invalid json: " + err.Error()})
			return
		}
	}
	if body.Label == "" {
		body.Label = "rotated"
	}
	raw, err := s.keys.Rotate(body.Label)
	if err != nil {
		s.log.Error("api-keys rotate", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "rotate failed"})
		return
	}
	s.log.Info("api key rotated", "label", body.Label)
	writeJSON(w, http.StatusOK, rotateResp{
		APIKey: raw,
		Label:  body.Label,
		Note:   "save this key now; it will not be shown again. Previous keys are revoked.",
	})
}
