package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/skip2/go-qrcode"
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

// handlePairingSessionsByToken 分发 /api/v1/pairing/sessions/{token}[/qr.png]：
//   - GET  {token}           查询 session 状态（Web UI 轮询配对结果）
//   - GET  {token}/qr.png    配对二维码 PNG（控制台渲染用）
//   - DELETE {token}         主人取消会话（QR 立即作废）
func (s *Server) handlePairingSessionsByToken(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/pairing/sessions/")
	if rest == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing token"})
		return
	}
	if strings.HasSuffix(rest, "/qr.png") {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
			return
		}
		s.handlePairingQR(w, r, strings.TrimSuffix(rest, "/qr.png"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handlePairingSessionGet(w, r, rest)
	case http.MethodDelete:
		s.handlePairingSessionCancel(w, r, rest)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET or DELETE only"})
	}
}

// handlePairingSessionGet 查询会话状态。
func (s *Server) handlePairingSessionGet(w http.ResponseWriter, _ *http.Request, token string) {
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

// handlePairingSessionCancel 主人取消待配对会话。
func (s *Server) handlePairingSessionCancel(w http.ResponseWriter, _ *http.Request, token string) {
	cancelled, err := s.pairingMgr.CancelSession(token)
	if err != nil {
		s.log.Error("cancel pairing session", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "cancel failed"})
		return
	}
	if !cancelled {
		// 未知 token 或已消费/已过期——幂等返回，不区分细节
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "session not pending"})
		return
	}
	s.log.Info("pairing session cancelled by owner", "token_prefix", token[:8]+"...")
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
}

// handlePairingQR 渲染配对二维码 PNG。仅 pending 会话可取（已消费/已过期
// 的 QR 无意义）。host 按请求解析规则与创建时一致（M1-G3）。
func (s *Server) handlePairingQR(w http.ResponseWriter, r *http.Request, token string) {
	sess, err := s.pairingMgr.GetSession(token)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "query failed"})
		return
	}
	if sess == nil || sess.Status != "pending" {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "session not pending"})
		return
	}
	qrHost, err := s.resolveAdvertise(r)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody{"mqtt_advertise_unresolved",
			"cannot resolve a device-reachable host: " + err.Error()})
		return
	}
	payload := s.pairingMgr.QRPayload(token, qrHost, s.pairingPort)
	png, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "qr encode failed"})
		return
	}
	pngBytes, err := png.PNG(256)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody{"internal", "qr png failed"})
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(pngBytes)
}

// 引用 encoding/json 避免 unused（payload 解析时可能扩展）
var _ = json.Marshal
