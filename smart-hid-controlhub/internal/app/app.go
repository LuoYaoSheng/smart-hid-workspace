// Package app 装配 ControlHub 各模块：config → logging → storage → broker → device manager → engine → api。
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"smart-hid-controlhub/internal/api"
	"smart-hid-controlhub/internal/command"
	"smart-hid-controlhub/internal/config"
	"smart-hid-controlhub/internal/device"
	"smart-hid-controlhub/internal/logging"
	"smart-hid-controlhub/internal/mqtt"
	"smart-hid-controlhub/internal/protocol"
	"smart-hid-controlhub/internal/storage"
)

// Run 启动整个 ControlHub，阻塞直到收到 SIGINT/SIGTERM。
func Run(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	log := logging.NewLogger(cfg.LogLevel).With("component", "controlhub")

	// API Key：留空则随机生成并打印（Phase 1 开发期可接受）
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey, err = generateAPIKey(32)
		if err != nil {
			return fmt.Errorf("generate api key: %w", err)
		}
	}
	log.Info("api key", "key", apiKey, "note", "set http.api_key in config.yaml to override")

	// SQLite
	dbPath := filepath.Join(cfg.DataDir, "controlhub.db")
	store, err := storage.New(dbPath, log.With("component", "storage"))
	if err != nil {
		return err
	}
	defer store.Close()

	// Device Manager
	dm, err := device.New(store, log.With("component", "device"))
	if err != nil {
		return err
	}

	// MQTT broker（嵌入式）
	broker := mqtt.NewBroker(cfg.MQTT.Host, cfg.MQTT.Port, cfg.MQTT.Username, cfg.MQTT.Password, log.With("component", "mqtt"))
	if err := broker.Start(cfg.MQTT.Username, cfg.MQTT.Password); err != nil {
		return err
	}
	defer broker.Close()
	// 给 broker 一点启动时间
	time.Sleep(200 * time.Millisecond)

	// ControlHub 自身作为 MQTT client（订阅所有设备的 ack + status 通配）
	hubClient := mqtt.NewClient(cfg.MQTT.Host, cfg.MQTT.Port, "controlhub-internal", cfg.MQTT.Username, cfg.MQTT.Password)
	if t := hubClient.Connect(); t.Wait() && t.Error() != nil {
		return fmt.Errorf("controlhub mqtt connect: %w", t.Error())
	}
	defer hubClient.Disconnect(500)
	log.Info("controlhub mqtt client connected")

	// Engine
	engine := command.New(hubClient, dm, store, log.With("component", "engine"))

	// 订阅 ack 通配（engine.HandleAck 路由到 pending chan）
	ackSub := fmt.Sprintf("smart-hid/v1/devices/+/ack")
	if t := hubClient.Subscribe(ackSub, 1, engine.HandleAck); t.Wait() && t.Error() != nil {
		return fmt.Errorf("subscribe ack: %w", t.Error())
	}
	// 订阅 status 通配 → 更新 Device Manager
	statusSub := fmt.Sprintf("smart-hid/v1/devices/+/status")
	statusHandler := func(_ pahomqtt.Client, msg pahomqtt.Message) {
		var st protocol.SmartHidStatus
		if err := json.Unmarshal(msg.Payload(), &st); err != nil {
			log.Warn("status unmarshal failed", "err", err, "payload", string(msg.Payload()))
			return
		}
		dm.UpsertStatus(&st)
		log.Debug("status updated", "device_id", st.DeviceID, "online", st.Online, "boot_id", st.BootID)
	}
	if t := hubClient.Subscribe(statusSub, 1, statusHandler); t.Wait() && t.Error() != nil {
		return fmt.Errorf("subscribe status: %w", t.Error())
	}
	log.Info("subscribed", "ack_topic", ackSub, "status_topic", statusSub)

	// HTTP API server
	apiSrv := api.New(engine, dm, apiKey, log.With("component", "api"))

	// 信号处理
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		if err := apiSrv.Start(cfg.HTTP.Host, cfg.HTTP.Port); err != nil {
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
	_ = apiSrv.Shutdown(shutdownCtx)
	log.Info("controlhub stopped")
	return nil
}

func generateAPIKey(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "chk_" + hex.EncodeToString(b), nil
}
