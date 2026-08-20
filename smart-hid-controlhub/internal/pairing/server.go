package pairing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
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
	mgr  *Manager
	addr string
	log  *slog.Logger
	srv  *http.Server
}

// NewDeviceServer 创建设备侧 listener。addr 形如 "0.0.0.0:17892"。
func NewDeviceServer(mgr *Manager, addr string, log *slog.Logger) *DeviceServer {
	return &DeviceServer{mgr: mgr, addr: addr, log: log}
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

	result, err := s.mgr.CompleteSession(req.Token, req.DeviceID, req.BootID, req.Firmware, req.Hardware)
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
	s.log.Info("pairing device served", "device_id", req.DeviceID)
	writeJSON(w, http.StatusOK, result)
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
