// Package api 实现 ControlHub HTTP API。
// Phase 1：GET /health（无鉴权）+ POST /devices/{id}/commands（Bearer）+ GET /devices + GET /devices/{id} + GET /commands/{request_id}。
// Entitlement 门控 Phase 1 跳过（passthrough），Phase 6 接入。
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"smart-hid-controlhub/internal/command"
	"smart-hid-controlhub/internal/device"
)

// Server 持有所有依赖。
type Server struct {
	engine   *command.Engine
	devices  *device.Manager
	apiKey   string
	log      *slog.Logger
	httpSrv  *http.Server
}

// New 构造 Server（不启动）。
func New(engine *command.Engine, dm *device.Manager, apiKey string, log *slog.Logger) *Server {
	return &Server{
		engine:  engine,
		devices: dm,
		apiKey:  apiKey,
		log:     log,
	}
}

// Routes 返回 http.Handler（带鉴权中间件 + 路由分发）。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", s.handleHealth)

	// 受保护路由（需 Bearer）
	protected := http.NewServeMux()
	protected.HandleFunc("/api/v1/devices", s.handleDevicesList)
	protected.HandleFunc("/api/v1/devices/", s.handleDeviceOrCommand) // /devices/{id} 与 /devices/{id}/commands
	protected.HandleFunc("/api/v1/commands/", s.handleCommandQuery)

	// 用一个 wrapper 给 protected 套鉴权
	mux.Handle("/api/v1/devices", s.authMiddleware(protected))
	mux.Handle("/api/v1/devices/", s.authMiddleware(protected))
	mux.Handle("/api/v1/commands/", s.authMiddleware(protected))

	return s.logMiddleware(mux)
}

// Start 启动 HTTP server（阻塞）。
func (s *Server) Start(host string, port int) error {
	s.httpSrv = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      s.Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	s.log.Info("http api listening", "addr", s.httpSrv.Addr)
	return s.httpSrv.ListenAndServe()
}

// Shutdown 优雅关闭。
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

// authMiddleware 校验 Authorization: Bearer <key>。
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /health 不走这里
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			writeJSON(w, http.StatusUnauthorized, errBody{"unauthorized", "missing bearer token"})
			return
		}
		if strings.TrimPrefix(auth, prefix) != s.apiKey {
			writeJSON(w, http.StatusUnauthorized, errBody{"unauthorized", "invalid api key"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// logMiddleware 记录每个请求的方法/路径/状态/耗时。
func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rw.status, "ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// --- 响应辅助 ---

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type errBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
