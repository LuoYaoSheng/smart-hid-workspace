// Command mock-device 模拟 ESP32-S3 设备，用于 Phase 1 端到端验证。
//
// 行为：
//  1. 连本机 MQTT broker（127.0.0.1:17891）
//  2. publish status online（带 device_id / boot_id / usb_hid_ready=true）
//  3. 订阅 smart-hid/v1/devices/{device_id}/command
//  4. 收到命令 → 日志模拟"USB HID 执行" → publish ack executed
//  5. --stale-boot-id 模式：返回 rejected（验证 STALE_DEVICE_SESSION）
//
// 注意：仅供开发期验证，真实固件见 smart-hid-firmware/。
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
)

const protocolVersion = "1.0"

type command struct {
	Protocol     string          `json:"protocol"`
	RequestID    string          `json:"request_id"`
	DeviceID     string          `json:"device_id"`
	TargetBootID string          `json:"target_boot_id"`
	Type         string          `json:"type"`
	Action       string          `json:"action"`
	TTLMs        int             `json:"ttl_ms"`
	Payload      json.RawMessage `json:"payload"`
}

type ack struct {
	Protocol    string `json:"protocol"`
	RequestID   string `json:"request_id"`
	DeviceID    string `json:"device_id"`
	BootID      string `json:"boot_id"`
	Status      string `json:"status"`
	Code        int    `json:"code"`
	ExecutionMs int    `json:"execution_ms,omitempty"`
}

type status struct {
	Protocol    string `json:"protocol"`
	DeviceID    string `json:"device_id"`
	Online      bool   `json:"online"`
	BootID      string `json:"boot_id"`
	USBHIDReady bool   `json:"usb_hid_ready"`
	Firmware    string `json:"firmware"`
	Timestamp   int64  `json:"timestamp"`
}

func main() {
	host := flag.String("mqtt-host", "127.0.0.1", "mqtt broker host")
	port := flag.Int("mqtt-port", 17891, "mqtt broker port")
	mqttUser := flag.String("mqtt-user", "controlhub", "mqtt username")
	mqttPass := flag.String("mqtt-pass", "change-me-in-production", "mqtt password")
	deviceID := flag.String("device-id", "HID-00000001", "device id (must match ^HID-[A-Z0-9]{8}$)")
	stale := flag.Bool("stale-boot-id", false, "reject all commands (simulate boot_id mismatch)")
	execDelay := flag.Int("exec-delay-ms", 6, "simulated USB HID execution delay")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).
		With("component", "mock-device", "device_id", *deviceID)

	bootID := genBootID()
	log.Info("mock device starting", "boot_id", bootID, "stale_mode", *stale, "firmware", "1.0.0-mock")

	// MQTT client
	opts := pahomqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", *host, *port))
	opts.SetClientID("mock-" + *deviceID)
	opts.SetUsername(*mqttUser)
	opts.SetPassword(*mqttPass)
	opts.SetAutoReconnect(true)
	client := pahomqtt.NewClient(opts)
	if t := client.Connect(); t.Wait() && t.Error() != nil {
		log.Error("mqtt connect failed", "err", t.Error())
		os.Exit(1)
	}
	defer client.Disconnect(500)
	log.Info("connected to broker")

	// publish status online
	publishStatus(client, *deviceID, bootID, true)
	log.Info("status online published")

	// 订阅 command topic
	cmdTopic := fmt.Sprintf("smart-hid/v1/devices/%s/command", *deviceID)
	cmdHandler := func(_ pahomqtt.Client, msg pahomqtt.Message) {
		var c command
		if err := json.Unmarshal(msg.Payload(), &c); err != nil {
			log.Warn("command unmarshal failed", "err", err)
			return
		}
		log.Info("command received", "request_id", c.RequestID, "type", c.Type, "action", c.Action,
			"target_boot_id", c.TargetBootID, "ttl_ms", c.TTLMs, "payload", string(c.Payload))

		// 模拟 USB HID 执行延迟
		time.Sleep(time.Duration(*execDelay) * time.Millisecond)

		// 构造 ack
		var a ack
		a.Protocol = protocolVersion
		a.RequestID = c.RequestID
		a.DeviceID = c.DeviceID
		a.BootID = bootID

		if *stale || c.TargetBootID != bootID {
			a.Status = "rejected"
			a.Code = 4001
			log.Warn("command rejected (stale device session)", "request_id", c.RequestID,
				"expected", bootID, "got", c.TargetBootID)
		} else {
			a.Status = "executed"
			a.Code = 0
			a.ExecutionMs = *execDelay
			log.Info("command executed (simulated USB HID)", "request_id", c.RequestID, "exec_ms", *execDelay)
		}

		ackTopic := fmt.Sprintf("smart-hid/v1/devices/%s/ack", c.DeviceID)
		payload, _ := json.Marshal(a)
		client.Publish(ackTopic, 1, false, payload)
	}
	if t := client.Subscribe(cmdTopic, 1, cmdHandler); t.Wait() && t.Error() != nil {
		log.Error("subscribe command failed", "err", t.Error())
		os.Exit(1)
	}
	log.Info("subscribed", "topic", cmdTopic)

	// 定期心跳 status（每 10s）
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			publishStatus(client, *deviceID, bootID, true)
		}
	}()

	// 优雅退出：publish offline 后断开
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
	log.Info("shutting down")
	publishStatus(client, *deviceID, bootID, false)
	log.Info("status offline published, bye")
}

func publishStatus(c pahomqtt.Client, deviceID, bootID string, online bool) {
	s := status{
		Protocol:    protocolVersion,
		DeviceID:    deviceID,
		Online:      online,
		BootID:      bootID,
		USBHIDReady: true,
		Firmware:    "1.0.0-mock",
		Timestamp:   time.Now().Unix(),
	}
	topic := fmt.Sprintf("smart-hid/v1/devices/%s/status", deviceID)
	payload, _ := json.Marshal(s)
	c.Publish(topic, 1, true, payload) // status retained
}

func genBootID() string {
	max := big.NewInt(0).SetUint64(0xFFFFFFFFFFFF)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "B-FALLBACK0"
	}
	return fmt.Sprintf("B-%06X", n.Uint64())
}
