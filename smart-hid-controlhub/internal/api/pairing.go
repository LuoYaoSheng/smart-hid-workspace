package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

// createSessionResp POST /api/v1/pairing/sessions 响应。
type createSessionResp struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	QRPayload string `json:"qr_payload"` // shid://pair?token=...&host=...&port=...
}

// handlePairingSessions POST /api/v1/pairing/sessions —— 创建配对会话。
func (s *Server) handlePairingSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	// QR host 按浏览器请求实际到达的本机地址解析（M1-G3：多网卡下与
	// 设备 pairing 路径同一套解析规则，不再首网卡猜测）。
	qrHost, err := s.resolveAdvertise(r)
	if err != nil {
		s.log.Error("resolve advertise host for QR", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, errBody{"mqtt_advertise_unresolved",
			"cannot resolve a device-reachable host: " + err.Error()})
		return
	}
	token, expiresAt, err := s.pairingMgr.CreateSession()
	if err != nil {
		s.log.Error("create pairing session", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "create session failed"})
		return
	}
	qr := s.pairingMgr.QRPayload(token, qrHost, s.pairingPort)
	s.log.Info("pairing session created via api", "token_prefix", token[:8]+"...", "qr_host", qrHost)
	writeJSON(w, http.StatusOK, createSessionResp{
		Token:     token,
		ExpiresAt: expiresAt,
		QRPayload: qr,
	})
}

// resolveAdvertise 与 pairing.DeviceServer.resolveAdvertise 同规则：
// LocalAddrContextKey 优先，RemoteAddr peer 兜底。
func (s *Server) resolveAdvertise(r *http.Request) (string, error) {
	var localAddr net.Addr
	if la, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		localAddr = la
	}
	var peer net.IP
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = net.ParseIP(host)
	}
	return s.advertiseRes.Resolve(localAddr, peer)
}

// handlePairingSessionsByToken GET /api/v1/pairing/sessions/{token} —— 查询 session 状态。
// 用于 Web UI 轮询配对结果。
func (s *Server) handlePairingSessionsByToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/api/v1/pairing/sessions/")
	if token == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing token"})
		return
	}
	sess, err := s.pairingMgr.GetSession(token)
	if err != nil {
		s.log.Error("get pairing session", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "query failed"})
		return
	}
	if sess == nil {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "session not found"})
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// 引用 encoding/json 避免 unused（payload 解析时可能扩展）
var _ = json.Marshal
