// Package api 实现 ControlHub HTTP API。
//
// Phase 1：GET /health（无鉴权）+ POST /devices/{id}/commands（Bearer）+
//
//	GET /devices + GET /devices/{id} + GET /commands/{request_id}。
//
// Phase 4（CH-P2）：API key 持久化（apikey.Store 替代 ephemeral string）+
//
//	POST /api-keys/rotate（A12）+ GET /api-keys。
//	Entitlement 门控 Phase 6 接入（CH-P6）。
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"smart-hid-controlhub/internal/apikey"
	"smart-hid-controlhub/internal/command"
	"smart-hid-controlhub/internal/device"
	"smart-hid-controlhub/internal/license"
	"smart-hid-controlhub/internal/pairing"
	"smart-hid-controlhub/internal/settings"
	"smart-hid-controlhub/internal/trial"
	"smart-hid-controlhub/internal/web"
)

// Server 持有所有依赖。
type Server struct {
	engine      *command.Engine
	devices     *device.Manager
	keys        *apikey.Store
	settings    *settings.Store
	pairingMgr  *pairing.Manager
	trialMgr    *trial.Manager
	licenseMgr  *licmgr.Manager
	log         *slog.Logger
	httpSrv     *http.Server
}

// New 构造 Server（不启动）。
// pairingMgr / trialMgr / licenseMgr 可为 nil（开发/测试场景）。
func New(engine *command.Engine, dm *device.Manager, keys *apikey.Store, setStore *settings.Store, pairingMgr *pairing.Manager, trialMgr *trial.Manager, licenseMgr *licmgr.Manager, log *slog.Logger) *Server {
	return &Server{
		engine:     engine,
		devices:    dm,
		keys:       keys,
		settings:   setStore,
		pairingMgr: pairingMgr,
		trialMgr:   trialMgr,
		licenseMgr: licenseMgr,
		log:        log,
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
	protected.HandleFunc("/api/v1/api-keys", s.handleAPIKeysList)            // GET list
	protected.HandleFunc("/api/v1/api-keys/rotate", s.handleAPIKeysRotate)   // POST 轮换（A12）
	protected.HandleFunc("/api/v1/settings/lan-mode", s.handleSettingsLAN)   // GET/POST LAN 模式（A11）
	protected.HandleFunc("/api/v1/usage", s.handleUsage)                     // GET 当前 Trial 用量（CH-P6）
	protected.HandleFunc("/api/v1/usage/all", s.handleUsageAll)              // GET 所有设备用量
	if s.licenseMgr != nil {
		protected.HandleFunc("/api/v1/license", s.handleLicenseStatus)        // GET 当前 license 状态
		protected.HandleFunc("/api/v1/license/import", s.handleLicenseImport) // POST 离线导入
		protected.HandleFunc("/api/v1/license/list", s.handleLicenseList)     // GET 所有 license
	}
	if s.pairingMgr != nil {
		protected.HandleFunc("/api/v1/pairing/sessions", s.handlePairingSessions)        // POST 创建
		protected.HandleFunc("/api/v1/pairing/sessions/", s.handlePairingSessionsByToken) // GET {token}
	}

	// 用一个 wrapper 给 protected 套鉴权
	auth := s.authMiddleware(protected)
	mux.Handle("/api/v1/devices", auth)
	mux.Handle("/api/v1/devices/", auth)
	mux.Handle("/api/v1/commands/", auth)
	mux.Handle("/api/v1/api-keys", auth)
	mux.Handle("/api/v1/api-keys/rotate", auth)
	mux.Handle("/api/v1/settings/lan-mode", auth)
	mux.Handle("/api/v1/usage", auth)
	mux.Handle("/api/v1/usage/all", auth)
	if s.licenseMgr != nil {
		mux.Handle("/api/v1/license", auth)
		mux.Handle("/api/v1/license/import", auth)
		mux.Handle("/api/v1/license/list", auth)
	}
	if s.pairingMgr != nil {
		mux.Handle("/api/v1/pairing/sessions", auth)
		mux.Handle("/api/v1/pairing/sessions/", auth)
	}

	// Web 管理界面（内嵌静态资源，本身不鉴权；控制调用由前端带 Bearer 请求 /api/v1/*）。
	// 注册在 "/" 兜底：/api/v1/* 更具体会优先生效，其余路径交给 FileServer。
	mux.Handle("/", web.Handler())

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

// authMiddleware 校验 Authorization: Bearer <key>，通过 apikey.Store 查表。
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			writeJSON(w, http.StatusUnauthorized, errBody{"unauthorized", "missing bearer token"})
			return
		}
		rawKey := strings.TrimPrefix(auth, prefix)
		if !s.keys.Verify(rawKey) {
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
