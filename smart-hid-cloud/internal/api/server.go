// Package api 实现 Smart HID Cloud HTTP API。
//
// CL-2a：基础 server（health endpoint + 鉴权中间件）。
// CL-2b：各业务 endpoint（user/plan/order/license/device/activation）。
package api

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"smart-hid-cloud/internal/auth"
	"smart-hid-cloud/internal/store"
)

// Server 持有所有依赖。
type Server struct {
	store      *store.Store
	jwtSecret  []byte
	privateKey ed25519.PrivateKey
	log        *slog.Logger
	httpSrv    *http.Server
}

// New 构造 Server（不启动）。privateKey 为 nil 时 license 签发会失败（开发期可空）。
func New(st *store.Store, jwtSecret []byte, privateKey ed25519.PrivateKey, log *slog.Logger) *Server {
	return &Server{
		store:      st,
		jwtSecret:  jwtSecret,
		privateKey: privateKey,
		log:        log,
	}
}

// Routes 返回 http.Handler。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// 公开路由
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/auth/register", s.handleRegister)
	mux.HandleFunc("/api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("/api/v1/plans", s.handleListPlans)

	// 受保护路由（需 JWT）
	protected := http.NewServeMux()
	protected.HandleFunc("/api/v1/users/me", s.handleMe)
	protected.HandleFunc("/api/v1/devices", s.handleDevices)
	protected.HandleFunc("/api/v1/orders", s.handleOrders)
	protected.HandleFunc("/api/v1/orders/", s.handlePayCallback) // /orders/{id}/pay-callback
	protected.HandleFunc("/api/v1/licenses", s.handleLicenses)
	protected.HandleFunc("/api/v1/licenses/", s.handleLicenseAction) // /licenses/{id} 和 /licenses/{id}/{action}

	authMW := s.JWTAuthMiddleware(protected)
	mux.Handle("/api/v1/users/me", authMW)
	mux.Handle("/api/v1/devices", authMW)
	mux.Handle("/api/v1/orders", authMW)
	mux.Handle("/api/v1/orders/", authMW)
	mux.Handle("/api/v1/licenses", authMW)
	mux.Handle("/api/v1/licenses/", authMW)

	return s.logMiddleware(mux)
}

// Start 阻塞启动 HTTP server。
func (s *Server) Start(host string, port int) error {
	s.httpSrv = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      s.Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	s.log.Info("cloud http listening", "addr", s.httpSrv.Addr)
	return s.httpSrv.ListenAndServe()
}

// Shutdown 优雅关闭。
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

// JWTAuthMiddleware 从 Authorization 头提取 Bearer token 并校验。
// 成功则把 user_id 写入 request context。
func (s *Server) JWTAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			writeJSON(w, http.StatusUnauthorized, errBody{"unauthorized", "missing bearer token"})
			return
		}
		tok := authHeader[7:]
		claims, err := auth.Verify(tok, s.jwtSecret)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errBody{"unauthorized", err.Error()})
			return
		}
		// 把 user_id 写入 ctx
		ctx := WithUserID(r.Context(), claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleHealth GET /api/v1/health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "smart-hid-cloud",
	})
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		s.log.Info("http",
			"method", r.Method, "path", r.URL.Path,
			"status", rw.status, "ms", time.Since(start).Milliseconds(),
		)
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

// --- context user_id ---

type ctxKey int

const userCtxKey ctxKey = 1

// WithUserID 把 user_id 写入 ctx。
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userCtxKey, userID)
}

// UserIDFromCtx 从 ctx 取 user_id（无则返 ""）。
func UserIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(userCtxKey).(string); ok {
		return v
	}
	return ""
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
