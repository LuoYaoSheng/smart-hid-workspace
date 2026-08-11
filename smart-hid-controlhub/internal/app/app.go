// Package app 装配 ControlHub 各模块：config → logging → storage → broker → device manager → engine → api。
//
// CH-P3 重构：把原 Run 拆成 Build / Start / Wait / Stop / Close，
// 以支持 headless（信号循环）和 tray（GUI 主线程）两种生命周期模型。
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"smart-hid-controlhub/internal/api"
	"smart-hid-controlhub/internal/apikey"
	"smart-hid-controlhub/internal/command"
	"smart-hid-controlhub/internal/config"
	"smart-hid-controlhub/internal/device"
	"smart-hid-controlhub/internal/logging"
	"smart-hid-controlhub/internal/mqtt"
	"smart-hid-controlhub/internal/protocol"
	"smart-hid-controlhub/internal/storage"
	"smart-hid-controlhub/internal/tray"
)

// App 持有所有运行时依赖与生命周期控制。
type App struct {
	cfg       *config.Config
	log       *slog.Logger
	store     *storage.Store
	keys      *apikey.Store
	dm        *device.Manager
	broker    *mqtt.Broker
	hubClient pahomqtt.Client
	engine    *command.Engine
	apiSrv    *api.Server

	ctx       context.Context
	cancel    context.CancelFunc
	stopOnce  sync.Once
	startOnce sync.Once
	started   bool
}

// Build 加载配置并装配所有依赖（不启动服务）。
func Build(cfgPath string) (*App, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	log := logging.NewLogger(cfg.LogLevel).With("component", "controlhub")

	// SQLite
	dbPath := filepath.Join(cfg.DataDir, "controlhub.db")
	store, err := storage.New(dbPath, log.With("component", "storage"))
	if err != nil {
		return nil, err
	}

	// API Key（CH-P2）：apikey.Store 持久化；首次启动生成 + 写文件 + 日志一次。
	keys := apikey.New(store.DB, log.With("component", "apikey"))
	initialKeyPath := filepath.Join(cfg.DataDir, "initial-api-key.txt")
	if raw, err := keys.EnsureInitial("initial"); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("init api key: %w", err)
	} else if raw != "" {
		_ = os.WriteFile(initialKeyPath, []byte(raw+"\n"), 0o600)
		log.Info("api key generated (first run)",
			"key", raw,
			"saved_to", initialKeyPath,
			"note", "delete this file after saving the key; rotate via POST /api/v1/api-keys/rotate")
	} else {
		if _, err := os.Stat(initialKeyPath); err == nil {
			log.Info("api key ready", "initial_key_file", initialKeyPath, "note", "still present; delete after saving")
		} else {
			log.Info("api key ready (loaded from store)")
		}
	}

	// Device Manager
	dm, err := device.New(store, log.With("component", "device"))
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	// MQTT broker（嵌入式）
	broker := mqtt.NewBroker(cfg.MQTT.Host, cfg.MQTT.Port, cfg.MQTT.Username, cfg.MQTT.Password, log.With("component", "mqtt"))

	// ControlHub 自身作为 MQTT client
	hubClient := mqtt.NewClient(cfg.MQTT.Host, cfg.MQTT.Port, "controlhub-internal", cfg.MQTT.Username, cfg.MQTT.Password)

	// Engine + API server（构造时不启动）
	engine := command.New(hubClient, dm, store, log.With("component", "engine"))
	apiSrv := api.New(engine, dm, keys, log.With("component", "api"))

	return &App{
		cfg:       cfg,
		log:       log,
		store:     store,
		keys:      keys,
		dm:        dm,
		broker:    broker,
		hubClient: hubClient,
		engine:    engine,
		apiSrv:    apiSrv,
	}, nil
}

// Start 启动后台服务（broker、MQTT 订阅、HTTP server）。
// 必须在 Build 之后、Wait 之前调用一次。
func (a *App) Start() error {
	var startErr error
	a.startOnce.Do(func() {
		// 启动 broker
		if err := a.broker.Start(a.cfg.MQTT.Username, a.cfg.MQTT.Password); err != nil {
			startErr = err
			return
		}
		time.Sleep(200 * time.Millisecond) // 给 broker 一点启动时间

		// hub client 连接
		if t := a.hubClient.Connect(); t.Wait() && t.Error() != nil {
			startErr = fmt.Errorf("controlhub mqtt connect: %w", t.Error())
			return
		}
		a.log.Info("controlhub mqtt client connected")

		// 订阅 ack / status
		ackSub := "smart-hid/v1/devices/+/ack"
		if t := a.hubClient.Subscribe(ackSub, 1, a.engine.HandleAck); t.Wait() && t.Error() != nil {
			startErr = fmt.Errorf("subscribe ack: %w", t.Error())
			return
		}
		statusSub := "smart-hid/v1/devices/+/status"
		statusHandler := func(_ pahomqtt.Client, msg pahomqtt.Message) {
			var st protocol.SmartHidStatus
			if err := json.Unmarshal(msg.Payload(), &st); err != nil {
				a.log.Warn("status unmarshal failed", "err", err, "payload", string(msg.Payload()))
				return
			}
			a.dm.UpsertStatus(&st)
			a.log.Debug("status updated", "device_id", st.DeviceID, "online", st.Online, "boot_id", st.BootID)
		}
		if t := a.hubClient.Subscribe(statusSub, 1, statusHandler); t.Wait() && t.Error() != nil {
			startErr = fmt.Errorf("subscribe status: %w", t.Error())
			return
		}
		a.log.Info("subscribed", "ack_topic", ackSub, "status_topic", statusSub)

		// HTTP server（goroutine）
		a.ctx, a.cancel = context.WithCancel(context.Background())
		go func() {
			if err := a.apiSrv.Start(a.cfg.HTTP.Host, a.cfg.HTTP.Port); err != nil {
				a.log.Error("http server error", "err", err)
				a.Stop()
			}
		}()

		a.started = true
		a.log.Info("controlhub started",
			"http_addr", fmt.Sprintf("%s:%d", a.cfg.HTTP.Host, a.cfg.HTTP.Port),
			"mqtt_port", a.cfg.MQTT.Port)
	})
	return startErr
}

// Wait 阻塞直到收到 SIGINT/SIGTERM 或 ctx 被 cancel（Stop / tray quit）。
// 返回前完成 graceful shutdown。
func (a *App) Wait() {
	if a.ctx == nil {
		return
	}
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-a.ctx.Done():
		a.log.Info("stop requested")
	case <-sigCtx.Done():
		a.log.Info("shutdown signal received")
		a.cancel()
	}
	a.shutdown()
}

// Stop 触发停止（幂等）。tray 退出菜单 / HTTP server 错误 / 外部调用使用。
func (a *App) Stop() {
	a.stopOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}
	})
}

// Close 释放所有底层资源（store 等）。Build 后必须 defer Close。
func (a *App) Close() {
	a.shutdown()
	if a.store != nil {
		_ = a.store.Close()
	}
}

func (a *App) shutdown() {
	if !a.started {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.apiSrv.Shutdown(ctx)
	if a.hubClient != nil && a.hubClient.IsConnected() {
		a.hubClient.Disconnect(500)
	}
	if a.broker != nil {
		a.broker.Close()
	}
	a.started = false
	a.log.Info("controlhub stopped")
}

// --- tray.Controller 实现 ---

// HTTPPort 返回本地 HTTP 端口，供 tray "打开控制台" 使用。
func (a *App) HTTPPort() int { return a.cfg.HTTP.Port }

// RotateAPIKey 暴露给 tray 菜单调用。
func (a *App) RotateAPIKey() (string, error) {
	return a.keys.Rotate("tray")
}

// --- 顶层入口（保留向后兼容） ---

// Run headless 模式：Build + Start + Wait + Close。
// 阻塞直到 SIGINT/SIGTERM。
func Run(cfgPath string) error {
	a, err := Build(cfgPath)
	if err != nil {
		return err
	}
	defer a.Close()
	if err := a.Start(); err != nil {
		return err
	}
	a.Wait()
	return nil
}

// RunWithTray tray 模式：Build + Start + 主线程跑 systray。
// 阻塞直到用户在托盘菜单选"退出"或收到 SIGINT/SIGTERM。
//
// 必须在主 goroutine 调用（systray 库限制：macOS NSApplication 必须主线程）。
func RunWithTray(cfgPath string) error {
	a, err := Build(cfgPath)
	if err != nil {
		return err
	}
	defer a.Close()
	if err := a.Start(); err != nil {
		return err
	}
	tray.Run(a, a.log.With("component", "tray"))
	a.Wait() // 确保 tray.Quit → onExit → Stop → ctx.Done → shutdown 完成
	return nil
}
