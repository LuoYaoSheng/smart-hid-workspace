// Command mock-device 是 ESP32-S3 固件 F1+F2 阶段的 Go 语义参考实现。
//
// 用途：
//  1. 在没有真实 ESP32-S3 硬件时，完整复现 F2 可靠性语义（dedup / boot_id /
//     TTL / lease / release_all / queue_full），让 ControlHub 端到端验证可信。
//  2. 作为 smart-hid-firmware/ 下 C 代码的行为参考（两侧语义对齐）。
//
// 与 C 固件的对应：
//   dedup          ↔ components/command_engine/dedup_cache.c
//   boot_id 校验   ↔ command_engine.c + device_identity.c
//   TTL 过期       ↔ command_engine worker
//   lease          ↔ hid_engine.c tick_leases（mock 用后台 goroutine 模拟）
//   release_all    ↔ hid_engine_release_all
//   queue_full     ↔ command_engine queue(32)
//
// 注意：本程序不真正发 USB HID Report（USB 行为由 C 代码负责），仅模拟执行耗时
// 并维护"逻辑按下"状态用于 lease 语义。--verbose 可打印详细决策。
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
	"strings"
	"sync"
	"syscall"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	protocolVersion       = "1.0"
	queueSize             = 32
	dedupCacheSize        = 256
	ttlMinMs       int    = 100
	ttlMaxMs       int    = 10000
	requestIDMaxLn int    = 96
	staleStr       string = "STALE_DEVICE_SESSION"
	defaultExecMs         = 6
)

/* ============================================================
 * 数据结构（与 smart-ble/core/protocols/hid-command-schema.ts 对齐）
 * ============================================================ */

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

/* ============================================================
 * DedupCache（环形 FIFO，request_id 最近 256 条）
 *
 * 对应 C: components/command_engine/dedup_cache.c
 * ============================================================ */
type DedupCache struct {
	mu    sync.Mutex
	ids   []string
	head  int
	count int
}

func NewDedupCache() *DedupCache {
	return &DedupCache{ids: make([]string, dedupCacheSize)}
}

// CheckAndAdd 命中（重复）返回 true，首次见返回 false 并插入。
func (d *DedupCache) CheckAndAdd(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := d.count
	if n > dedupCacheSize {
		n = dedupCacheSize
	}
	for i := 0; i < n; i++ {
		if d.ids[i] == id {
			return true
		}
	}
	d.ids[d.head] = id
	d.head = (d.head + 1) % dedupCacheSize
	if d.count < dedupCacheSize {
		d.count++
	}
	return false
}

func (d *DedupCache) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.ids {
		d.ids[i] = ""
	}
	d.head, d.count = 0, 0
}

/* ============================================================
 * LeaseManager（key_down / button_down 的 lease 超时自动 release）
 *
 * 对应 C: hid_engine.c tick_leases + release_all
 *
 * mock 维护"逻辑按下"集合：
 *   keysDown   map[usage]deadline
 *   buttonsDown map[mask]deadline
 * 后台 goroutine 每秒扫一次，过期自动 release（仅日志，无真实 USB）。
 * ============================================================ */
type LeaseManager struct {
	mu         sync.Mutex
	keysDown   map[string]int64 // keyName → deadline unix ms
	buttonsDn  map[string]int64 // buttonName → deadline unix ms
	log        *slog.Logger
}

func NewLeaseManager(log *slog.Logger) *LeaseManager {
	return &LeaseManager{
		keysDown:  map[string]int64{},
		buttonsDn: map[string]int64{},
		log:       log,
	}
}

// KeyDown 模拟按下 key。leaseMs==0 时按文档应 reject，但 mock 在 command 层就拦截了；
// 这里允许 leaseMs>0 时记 lease。
func (l *LeaseManager) KeyDown(key string, leaseMs int) {
	if leaseMs <= 0 {
		leaseMs = 5000 // 文档要求 lease_ms，mock 防呆默认 5s
	}
	l.mu.Lock()
	l.keysDown[key] = time.Now().UnixMilli() + int64(leaseMs)
	l.mu.Unlock()
}

func (l *LeaseManager) KeyUp(key string) {
	l.mu.Lock()
	delete(l.keysDown, key)
	l.mu.Unlock()
}

func (l *LeaseManager) ButtonDown(btn string, leaseMs int) {
	if leaseMs <= 0 {
		leaseMs = 5000
	}
	l.mu.Lock()
	l.buttonsDn[btn] = time.Now().UnixMilli() + int64(leaseMs)
	l.mu.Unlock()
}

func (l *LeaseManager) ButtonUp(btn string) {
	l.mu.Lock()
	delete(l.buttonsDn, btn)
	l.mu.Unlock()
}

// ReleaseAll 清空所有 pressed 状态（fail-safe）。
func (l *LeaseManager) ReleaseAll() {
	l.mu.Lock()
	n := len(l.keysDown) + len(l.buttonsDn)
	l.keysDown = map[string]int64{}
	l.buttonsDn = map[string]int64{}
	l.mu.Unlock()
	if n > 0 {
		l.log.Info("release_all executed", "released_count", n)
	}
}

func (l *LeaseManager) startTicker(ctx context.Context) {
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				nowMs := now.UnixMilli()
				l.mu.Lock()
				for k, dl := range l.keysDown {
					if dl > 0 && nowMs >= dl {
						delete(l.keysDown, k)
						l.log.Info("key lease expired → auto release", "key", k)
					}
				}
				for b, dl := range l.buttonsDn {
					if dl > 0 && nowMs >= dl {
						delete(l.buttonsDn, b)
						l.log.Info("button lease expired → auto release", "button", b)
					}
				}
				l.mu.Unlock()
			}
		}
	}()
}

/* ============================================================
 * Device（ESP32 mock）
 * ============================================================ */
type Device struct {
	mu        sync.Mutex
	bootID    string
	deviceID  string
	staleMode bool
	execDelay time.Duration

	dedup *DedupCache
	lease *LeaseManager

	// queue 模拟（FIFO，size=queueSize）
	queueMu  sync.Mutex
	queue    []command
	queueCap int
	log      *slog.Logger

	// request_id → 创建时间（用于 worker 取出后 TTL 过期判定）
	enqAtMu sync.Mutex
	enqAt   map[string]time.Time
}

func NewDevice(deviceID string, staleMode bool, execDelay time.Duration, log *slog.Logger) *Device {
	d := &Device{
		bootID:    genBootID(),
		deviceID:  deviceID,
		staleMode: staleMode,
		execDelay: execDelay,
		dedup:     NewDedupCache(),
		lease:     NewLeaseManager(log.With("sub", "lease")),
		queue:     make([]command, 0, queueSize),
		queueCap:  queueSize,
		log:       log,
		enqAt:     map[string]time.Time{},
	}
	return d
}

/* ----- 处理路径：MQTT RX → parse → dedup → boot_id → TTL → enqueue → worker → ack -----
 * 对应 C: command_engine.c
 * ----- */

// HandleCommand 对应 command_engine_handle_raw 的同步部分。
// 返回 (immediateAck, hasImmediate)：true 时调用方应立即 publish ack；
// false 时已入队，worker 异步 publish。
func (d *Device) HandleCommand(c command) (ack, bool) {
	// 1. dedup
	if d.dedup.CheckAndAdd(c.RequestID) {
		d.log.Info("dedup hit → duplicate", "request_id", c.RequestID)
		return d.buildAck(c, "duplicate", 0, 0), true
	}

	// 2. boot_id 校验
	if d.staleMode || c.TargetBootID != d.bootID {
		d.log.Warn("stale device session rejected",
			"request_id", c.RequestID, "target", c.TargetBootID, "current", d.bootID)
		return d.buildAck(c, "rejected", 4001, 0), true
	}

	// 3. TTL 范围校验
	if c.TTLMs < ttlMinMs || c.TTLMs > ttlMaxMs {
		d.log.Warn("ttl out of range rejected", "request_id", c.RequestID, "ttl_ms", c.TTLMs)
		return d.buildAck(c, "rejected", 4002, 0), true
	}

	// 4. 入队（满 → queue_full）
	d.queueMu.Lock()
	if len(d.queue) >= d.queueCap {
		d.queueMu.Unlock()
		d.log.Warn("queue full → rejected", "request_id", c.RequestID, "cap", d.queueCap)
		return d.buildAck(c, "rejected", 4003, 0), true
	}
	d.queue = append(d.queue, c)
	d.queueMu.Unlock()

	d.enqAtMu.Lock()
	d.enqAt[c.RequestID] = time.Now()
	d.enqAtMu.Unlock()

	d.log.Info("command enqueued", "request_id", c.RequestID,
		"type", c.Type, "action", c.Action, "ttl_ms", c.TTLMs)
	return ack{}, false
}

// workerLoop 对应 C worker_task：串行出队 → 执行 → publish ack
func (d *Device) workerLoop(ctx context.Context, publish func(ack)) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c, ok := d.dequeue()
		if !ok {
			time.Sleep(2 * time.Millisecond)
			continue
		}

		// 取出后 TTL 过期判定：enqAt + ttl_ms < now
		d.enqAtMu.Lock()
		enqT, has := d.enqAt[c.RequestID]
		delete(d.enqAt, c.RequestID)
		d.enqAtMu.Unlock()
		if has && time.Since(enqT) > time.Duration(c.TTLMs)*time.Millisecond {
			d.log.Warn("expired in queue", "request_id", c.RequestID,
				"age_ms", time.Since(enqT).Milliseconds(), "ttl_ms", c.TTLMs)
			publish(d.buildAck(c, "expired", 5001, 0))
			continue
		}

		// 执行（模拟 USB HID 延迟 + lease 维护）
		execMs := d.simulateExec(c)
		publish(d.buildAck(c, "executed", 0, execMs))
	}
}

func (d *Device) dequeue() (command, bool) {
	d.queueMu.Lock()
	defer d.queueMu.Unlock()
	if len(d.queue) == 0 {
		return command{}, false
	}
	c := d.queue[0]
	d.queue = d.queue[1:]
	return c, true
}

// simulateExec 模拟 USB HID 执行 + 维护 lease 状态
func (d *Device) simulateExec(c command) int {
	execMs := int(d.execDelay / time.Millisecond)
	if execMs <= 0 {
		execMs = defaultExecMs
	}
	time.Sleep(d.execDelay)

	// 维护 lease 状态（仅对有 lease 语义的 action）
	switch c.Type {
	case "keyboard":
		switch c.Action {
		case "key_down":
			var p struct {
				Key     string `json:"key"`
				LeaseMs int    `json:"lease_ms"`
			}
			_ = json.Unmarshal(c.Payload, &p)
			key := p.Key
			if key == "" {
				key = "<unknown>"
			}
			d.lease.KeyDown(key, p.LeaseMs)
		case "key_up":
			var p struct{ Key string `json:"key"` }
			_ = json.Unmarshal(c.Payload, &p)
			d.lease.KeyUp(p.Key)
		}
	case "mouse":
		switch c.Action {
		case "button_down":
			var p struct {
				Button  string `json:"button"`
				LeaseMs int    `json:"lease_ms"`
			}
			_ = json.Unmarshal(c.Payload, &p)
			btn := p.Button
			if btn == "" {
				btn = "LEFT"
			}
			d.lease.ButtonDown(btn, p.LeaseMs)
		case "button_up":
			var p struct{ Button string `json:"button"` }
			_ = json.Unmarshal(c.Payload, &p)
			btn := p.Button
			if btn == "" {
				btn = "LEFT"
			}
			d.lease.ButtonUp(btn)
		case "release_all":
			// 不该走这里（release_all 是 system），保险
		}
	case "system":
		if c.Action == "release_all" {
			d.lease.ReleaseAll()
		}
	}
	return execMs
}

func (d *Device) buildAck(c command, status string, code, execMs int) ack {
	return ack{
		Protocol:    protocolVersion,
		RequestID:   c.RequestID,
		DeviceID:    c.DeviceID,
		BootID:      d.bootID,
		Status:      status,
		Code:        code,
		ExecutionMs: execMs,
	}
}

func (d *Device) BootID() string { return d.bootID }

/* ============================================================
 * 工具
 * ============================================================ */
func genBootID() string {
	max := big.NewInt(0).SetUint64(0xFFFFFF)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "B-FALLBACK0"
	}
	return fmt.Sprintf("B-%06X", n.Uint64())
}

/* ============================================================
 * main
 * ============================================================ */
func main() {
	host := flag.String("mqtt-host", "127.0.0.1", "mqtt broker host")
	port := flag.Int("mqtt-port", 17891, "mqtt broker port")
	mqttUser := flag.String("mqtt-user", "controlhub", "mqtt username")
	mqttPass := flag.String("mqtt-pass", "change-me-in-production", "mqtt password")
	deviceID := flag.String("device-id", "HID-00000001",
		"device id (must match ^HID-[A-Z0-9]{8}$)")
	stale := flag.Bool("stale-boot-id", false, "reject all commands (simulate boot_id mismatch)")
	execDelay := flag.Int("exec-delay-ms", defaultExecMs, "simulated USB HID execution delay")
	verbose := flag.Bool("verbose", false, "verbose decision logging")
	flag.Parse()

	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})).
		With("component", "mock-device", "device_id", *deviceID)

	if !strings.HasPrefix(*deviceID, "HID-") || len(*deviceID) != 12 {
		log.Error("invalid device_id", "device_id", *deviceID,
			"hint", "must match ^HID-[A-Z0-9]{8}$")
		os.Exit(2)
	}

	dev := NewDevice(*deviceID, *stale, time.Duration(*execDelay)*time.Millisecond, log)
	log.Info("mock device starting",
		"boot_id", dev.BootID(), "stale_mode", *stale, "exec_delay_ms", *execDelay,
		"queue_size", queueSize, "dedup_size", dedupCacheSize, "firmware", "1.0.0-mock-f2")

	/* ----- MQTT 客户端 ----- */
	opts := pahomqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", *host, *port))
	opts.SetClientID("mock-" + *deviceID)
	opts.SetUsername(*mqttUser)
	opts.SetPassword(*mqttPass)
	opts.SetAutoReconnect(true)

	// LWT：online=false retained status（与 C mqtt_manager 一致）
	lwtStatus := status{
		Protocol: protocolVersion, DeviceID: *deviceID, Online: false,
		BootID: dev.BootID(), USBHIDReady: false, Firmware: "1.0.0-mock-f2",
		Timestamp: time.Now().Unix(),
	}
	lwtPayload, _ := json.Marshal(lwtStatus)
	lwtTopic := fmt.Sprintf("smart-hid/v1/devices/%s/status", *deviceID)
	opts.SetWill(lwtTopic, string(lwtPayload), 1, true)

	// MQTT 断开 → release_all（与 C wifi/mqtt manager 一致）
	opts.OnConnectionLost = func(_ pahomqtt.Client, _ error) {
		log.Warn("mqtt connection lost → release_all")
		dev.lease.ReleaseAll()
	}

	client := pahomqtt.NewClient(opts)
	if t := client.Connect(); t.Wait() && t.Error() != nil {
		log.Error("mqtt connect failed", "err", t.Error())
		os.Exit(1)
	}
	defer client.Disconnect(500)
	log.Info("connected to broker")

	// 上线 status retained
	pubStatus(client, *deviceID, dev.BootID(), true)
	log.Info("status online published (retained)")

	// 启动 lease ticker
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	dev.lease.startTicker(ctx)

	// 订阅 command
	cmdTopic := fmt.Sprintf("smart-hid/v1/devices/%s/command", *deviceID)
	cmdHandler := func(_ pahomqtt.Client, msg pahomqtt.Message) {
		var c command
		if err := json.Unmarshal(msg.Payload(), &c); err != nil {
			log.Warn("command unmarshal failed", "err", err, "raw", string(msg.Payload()))
			return
		}
		log.Info("command RX",
			"request_id", c.RequestID, "type", c.Type, "action", c.Action,
			"target_boot_id", c.TargetBootID, "ttl_ms", c.TTLMs,
			"payload", string(c.Payload))

		publishAck := func(a ack) {
			ackTopic := fmt.Sprintf("smart-hid/v1/devices/%s/ack", c.DeviceID)
			payload, _ := json.Marshal(a)
			client.Publish(ackTopic, 1, false, payload)
			log.Info("ack published",
				"request_id", a.RequestID, "status", a.Status, "code", a.Code,
				"exec_ms", a.ExecutionMs)
		}

		immediate, hasImmediate := dev.HandleCommand(c)
		if hasImmediate {
			publishAck(immediate)
		}
		// 否则 worker 异步处理
	}
	if t := client.Subscribe(cmdTopic, 1, cmdHandler); t.Wait() && t.Error() != nil {
		log.Error("subscribe command failed", "err", t.Error())
		os.Exit(1)
	}
	log.Info("subscribed", "topic", cmdTopic)

	// worker goroutine（与 C worker_task 对应）
	workerPublish := func(a ack) {
		ackTopic := fmt.Sprintf("smart-hid/v1/devices/%s/ack", a.DeviceID)
		payload, _ := json.Marshal(a)
		client.Publish(ackTopic, 1, false, payload)
		log.Info("ack published (worker)",
			"request_id", a.RequestID, "status", a.Status, "code", a.Code,
			"exec_ms", a.ExecutionMs)
	}
	go dev.workerLoop(ctx, workerPublish)

	// 心跳 status（每 N 秒）
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pubStatus(client, *deviceID, dev.BootID(), true)
			}
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	pubStatus(client, *deviceID, dev.BootID(), false)
	log.Info("status offline published, bye")
}

/* 真实 publishStatus 实现（pubStatus 接收 paho client） */
func pubStatus(c pahomqtt.Client, deviceID, bootID string, online bool) {
	s := status{
		Protocol: protocolVersion, DeviceID: deviceID, Online: online,
		BootID: bootID, USBHIDReady: true, Firmware: "1.0.0-mock-f2",
		Timestamp: time.Now().Unix(),
	}
	topic := fmt.Sprintf("smart-hid/v1/devices/%s/status", deviceID)
	payload, _ := json.Marshal(s)
	c.Publish(topic, 1, true, payload) // retained
}
