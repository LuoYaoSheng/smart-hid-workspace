// Command cloud 启动 Smart HID Cloud。
//
// 用法：
//
//	cloud                     # 用默认配置
//	cloud -config config.yaml
//
// CL-2a：启动 HTTP server + health endpoint，跑通基础链路。
// CL-2b 起加业务 endpoint。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"smart-hid-cloud/internal/api"
	"smart-hid-cloud/internal/config"
	"smart-hid-cloud/internal/logging"
	"smart-hid-cloud/internal/storage"
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

	// SQLite
	store, err := storage.New(cfg.Database.Path, log.With("component", "storage"))
	if err != nil {
		log.Error("open storage", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	// 校验 License 私钥可达（CL-2b 实际使用）
	if _, err := os.Stat(cfg.LicenseKeyPath); err != nil {
		log.Warn("license private key not found (license signing will fail until generated)",
			"path", cfg.LicenseKeyPath,
			"hint", "run scripts/gen-keys.sh")
	} else {
		log.Info("license private key ready", "path", cfg.LicenseKeyPath)
	}

	// HTTP server
	srv := api.New(store, []byte(cfg.JWTSecret), log.With("component", "api"))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(cfg.HTTP.Host, cfg.HTTP.Port); err != nil {
			errCh <- err
		}
	}()

	// 写 pid 文件（便于开发期定位）
	pidFile := filepath.Join(cfg.DataDir, "cloud.pid")
	_ = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
	defer os.Remove(pidFile)

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

// 引用 slog 避免 unused（编译期保证 import）
var _ = slog.Default
