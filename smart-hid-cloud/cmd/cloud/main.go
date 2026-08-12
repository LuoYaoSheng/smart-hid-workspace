// Command cloud 启动 Smart HID Cloud。
//
// 用法：
//
//	cloud                     # 用默认配置
//	cloud -config config.yaml
//
// CL-2b：完整业务 endpoint（auth/plans/devices/orders/licenses）+ License 签发。
package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smart-hid-cloud/internal/api"
	"smart-hid-cloud/internal/config"
	"smart-hid-cloud/internal/logging"
	"smart-hid-cloud/internal/storage"
	"smart-hid-cloud/internal/store"
	"smart-hid-cloud/pkg/license"
)

func main() {
	cfgPath := flag.String("config", "", "path to config.yaml (default: built-in defaults)")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cloud: load config: %v\n", err)
		os.Exit(1)
	}
	log := logging.NewLogger(cfg.LogLevel).With("component", "cloud")

	// SQLite（migration 自动跑）
	storageStore, err := storage.New(cfg.Database.Path, log.With("component", "storage"))
	if err != nil {
		log.Error("open storage", "err", err)
		os.Exit(1)
	}
	defer storageStore.Close()

	// 业务 store
	bizStore := store.New(storageStore.DB)

	// 加载 License 私钥（缺失则 license 签发会 500，但其他 endpoint 正常）
	var privKey ed25519.PrivateKey
	if cfg.LicenseKeyPath != "" {
		if k, err := license.LoadPrivateKey(cfg.LicenseKeyPath); err != nil {
			log.Warn("license private key load failed (signing endpoints will 500)",
				"path", cfg.LicenseKeyPath, "err", err,
				"hint", "run scripts/gen-keys.sh")
		} else {
			privKey = k
			log.Info("license private key ready", "path", cfg.LicenseKeyPath)
		}
	}

	// Seed 默认套餐（V1）
	if err := bizStore.SeedPlans(defaultPlans()); err != nil {
		log.Warn("seed plans", "err", err)
	} else {
		log.Info("plans seeded", "count", len(defaultPlans()))
	}

	// Promote admin（CL-5a）：config.admin_email 指定的用户提升为 admin。
	// 用户需先在 app.html 注册；此处仅在已存在时提升，否则 warn。
	if cfg.AdminEmail != "" {
		if err := bizStore.PromoteAdmin(cfg.AdminEmail); err != nil {
			log.Warn("promote admin skipped (user not registered yet?)",
				"admin_email", cfg.AdminEmail, "err", err,
				"hint", "先在 app.html 注册该邮箱，再重启 cloud")
		} else {
			log.Info("admin promoted", "admin_email", cfg.AdminEmail)
		}
	}

	// HTTP server
	srv := api.New(bizStore, []byte(cfg.JWTSecret), privKey, log.With("component", "api"))
	srv.SetCORS(cfg.HTTP.CORSOrigins)
	if cfg.HTTP.WebRoot != "" {
		srv.SetWebRoot(cfg.HTTP.WebRoot)
		log.Info("web root enabled (same-origin portal)", "dir", cfg.HTTP.WebRoot)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(cfg.HTTP.Host, cfg.HTTP.Port); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		log.Error("http server error", "err", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("cloud stopped")
}

// defaultPlans V1 默认套餐（每次启动 upsert，可在数据库改 active 调整）。
func defaultPlans() []store.Plan {
	return []store.Plan{
		{
			PlanID:       "plan_basic_monthly",
			Name:         "基础版 月度",
			Description:  "Smart HID 基础版，月度订阅",
			PriceCents:   1900,
			Currency:     "CNY",
			DurationDays: 30,
			Features:     []string{"hid_control"},
			Active:       true,
		},
		{
			PlanID:       "plan_basic_yearly",
			Name:         "基础版 年度",
			Description:  "Smart HID 基础版，年度订阅（优惠）",
			PriceCents:   19900,
			Currency:     "CNY",
			DurationDays: 365,
			Features:     []string{"hid_control"},
			Active:       true,
		},
	}
}

// 引用 slog 避免 unused
var _ = slog.Default
