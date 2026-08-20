package pairing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"smart-hid-controlhub/internal/netaddr"
)

// DeviceReq 是 ESP32 调用 POST /api/v1/pairing/device 的请求体。
type DeviceReq struct {
	Token    string `json:"token"`
	DeviceID string `json:"device_id"`
	BootID   string `json:"boot_id"`
	Firmware string `json:"firmware,omitempty"`
	Hardware string `json:"hardware,omitempty"`
}

// DeviceServer 是独立的设备侧配对 HTTP listener（端口默认 17892）。
// 与主 API server 隔离：主 server 的 /api/v1/* 需要 API key，
// 而设备侧 /api/v1/pairing/device 仅凭 token 鉴权（设备在配对前不可能有 API key）。
type DeviceServer struct {
	mgr      *Manager
	resolver *netaddr.Resolver // MQTT advertise host 解析（M1-G3）
	addr     string
	log      *slog.Logger
	srv      *http.Server
}

// NewDeviceServer 创建设备侧 listener。addr 形如 "0.0.0.0:17892"。
// resolver 用于把返回给设备的 mqtt_host 解析成该请求实际可达的本机地址。
func NewDeviceServer(mgr *Manager, resolver *netaddr.Resolver, addr string, log *slog.Logger) *DeviceServer {
	return &DeviceServer{mgr: mgr, resolver: resolver, addr: addr, log: log}
}

// Start 启动 HTTP server（阻塞）。
func (s *DeviceServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/pairing/device", s.handleDevice)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "service": "pairing"})
	})

	s.srv = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	s.log.Info("pairing server listening", "addr", s.addr)
	return s.srv.ListenAndServe()
}

// Shutdown 优雅关闭。
func (s *DeviceServer) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// handleDevice POST /api/v1/pairing/device
// 无 API key，凭 body.token 鉴权。成功返一次性 MQTT 凭据。
//
// advertise host 在消费 token **之前**按本请求路径解析（spec M1-G3 §11）：
// 解析失败 → 503，token 保持 pending，用户修复 mqtt.advertise_host 后可直接重试。
func (s *DeviceServer) handleDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}
	var req DeviceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "invalid json: " + err.Error()})
		return
	}
	if req.Token == "" || req.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "token and device_id required"})
		return
	}

	// 1) resolve + validate MQTT endpoint（失败不消费 token）
	advHost, err := s.resolveAdvertise(r)
	if err != nil {
		s.log.Error("mqtt advertise resolution failed; token NOT consumed",
			"device_id", req.DeviceID, "err", err)
		writeJSON(w, http.StatusServiceUnavailable, errBody{
			"mqtt_advertise_unresolved",
			"cannot resolve a device-reachable MQTT host: " + err.Error(),
		})
		return
	}

	// 2) 原子消费 token + 签发凭据
	result, err := s.mgr.CompleteSession(req.Token, req.DeviceID, req.BootID, req.Firmware, req.Hardware, advHost)
	if err != nil {
		s.log.Warn("pairing complete failed",
			"device_id", req.DeviceID, "err", err)
		// 哨兵错误 → 稳定错误码；其余（格式/内部）→ 400/500。
		switch {
		case errors.Is(err, ErrTokenNotFound):
			writeJSON(w, http.StatusNotFound, errBody{"pairing_token_invalid", "token not found"})
		case errors.Is(err, ErrTokenExpired):
			writeJSON(w, http.StatusGone, errBody{"pairing_token_expired", "token expired or revoked"})
		case errors.Is(err, ErrTokenUsed):
			writeJSON(w, http.StatusConflict, errBody{"pairing_token_used", "token already consumed"})
		default:
			writeJSON(w, http.StatusBadRequest, errBody{"pairing_failed", err.Error()})
		}
		return
	}
	s.log.Info("pairing device served", "device_id", req.DeviceID, "mqtt_host", advHost)
	writeJSON(w, http.StatusOK, result)
}

// resolveAdvertise 按请求路径解析 advertise host：
// 优先 http.LocalAddrContextKey（设备连接实际到达的本机地址，精确），
// 其次 RemoteAddr peer（UDP 出口推导 / 唯一 LAN IPv4 兜底）。
func (s *DeviceServer) resolveAdvertise(r *http.Request) (string, error) {
	var localAddr net.Addr
	if la, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		localAddr = la
	}
	var peer net.IP
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = net.ParseIP(host)
	}
	return s.resolver.Resolve(localAddr, peer)
}

// --- 内部辅助 ---

type errBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}



// 引用 fmt 避免 unused（如果未来加日志细节）
var _ = fmt.Sprintf
