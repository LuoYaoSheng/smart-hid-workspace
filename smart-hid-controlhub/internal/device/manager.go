// Package device 实现 Device Manager。
// 职责：跟踪设备 online/boot_id/usb_hid_ready/firmware（通过订阅 status topic 更新）；
//
//	提供查询能力供 HTTP 层与 Command Engine 使用。
//
// Phase 1：内存 map + SQLite 持久化；不做配对（CH-06 跳过）。
package device

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"smart-hid-controlhub/internal/protocol"
	"smart-hid-controlhub/internal/storage"
)

// Device 是设备运行态（内存视图）。
type Device struct {
	DeviceID    string
	BootID      string
	Online      bool
	USBHIDReady bool
	Firmware    string
	LastSeenAt  time.Time
}

// Manager 管理已知设备。
type Manager struct {
	mu   sync.RWMutex
	devs map[string]*Device // device_id -> Device
	db   *storage.Store
	log  *slog.Logger
}

// New 构造 Manager 并从 SQLite 恢复设备列表（online 字段置 0，等待 status 刷新）。
func New(db *storage.Store, log *slog.Logger) (*Manager, error) {
	m := &Manager{
		devs: make(map[string]*Device),
		db:   db,
		log:  log,
	}
	if err := m.loadFromDB(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) loadFromDB() error {
	rows, err := m.db.DB.Query(`SELECT device_id, boot_id, firmware, last_seen_at FROM devices`)
	if err != nil {
		return fmt.Errorf("load devices: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d Device
		var lastSeen int64
		if err := rows.Scan(&d.DeviceID, &d.BootID, &d.Firmware, &lastSeen); err != nil {
			return err
		}
		d.LastSeenAt = time.Unix(lastSeen, 0)
		// 启动时一律视为离线，等 status 包刷新
		d.Online = false
		m.devs[d.DeviceID] = &d
	}
	return nil
}

// UpsertStatus 收到设备 status 包时调用。更新内存 + SQLite。
func (m *Manager) UpsertStatus(s *protocol.SmartHidStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	d, exists := m.devs[s.DeviceID]
	if !exists {
		d = &Device{DeviceID: s.DeviceID}
		m.devs[s.DeviceID] = d
		m.log.Info("device registered", "device_id", s.DeviceID, "boot_id", s.BootID)
	}
	prevBoot := d.BootID
	d.BootID = s.BootID
	d.Online = s.Online
	d.USBHIDReady = s.USBHIDReady
	if s.Firmware != "" {
		d.Firmware = s.Firmware
	}
	d.LastSeenAt = now

	// persist（upsert）
	_, err := m.db.DB.Exec(
		`INSERT INTO devices(device_id, boot_id, online, usb_hid_ready, firmware, last_seen_at)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(device_id) DO UPDATE SET
		   boot_id=excluded.boot_id, online=excluded.online,
		   usb_hid_ready=excluded.usb_hid_ready, firmware=excluded.firmware,
		   last_seen_at=excluded.last_seen_at`,
		s.DeviceID, s.BootID, boolToInt(s.Online), boolToInt(s.USBHIDReady), d.Firmware, now.Unix(),
	)
	if err != nil {
		m.log.Warn("persist device failed", "device_id", s.DeviceID, "err", err)
	}
	if prevBoot != "" && prevBoot != s.BootID {
		m.log.Info("device reboot detected", "device_id", s.DeviceID, "old", prevBoot, "new", s.BootID)
	}
}

// Get 返回设备指针的快照拷贝（调用方可安全读取）。
func (m *Manager) Get(deviceID string) (*Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.devs[deviceID]
	if !ok {
		return nil, false
	}
	cp := *d
	return &cp, true
}

// List 返回所有设备快照。
func (m *Manager) List() []Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Device, 0, len(m.devs))
	for _, d := range m.devs {
		out = append(out, *d)
	}
	return out
}

// IsReady 返回设备是否在线 + USB HID 就绪（Command Engine 下发前置检查）。
func (m *Manager) IsReady(deviceID string) (online, usbReady bool, ok bool) {
	d, exists := m.Get(deviceID)
	if !exists {
		return false, false, false
	}
	return d.Online, d.USBHIDReady, true
}

// Delete 注销设备：删除内存视图 + devices 行，并撤销 MQTT 凭据
// （device_credentials）——删除后 PerDeviceHook 查不到凭据，该设备再连
// MQTT 会被拒，即"本 hub 不再信任此设备"。已建立的 MQTT 连接保持到其
// 下次重连被拒为止。该设备的命令历史与配对会话记录一并清理
// （SQLite FK 强制且无级联定义；审计由 security_events 表另行承担）。
// 返回是否确实存在并已删除。
func (m *Manager) Delete(deviceID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.devs[deviceID]; !ok {
		return false, nil
	}
	tx, err := m.db.DB.Begin()
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM commands WHERE device_id = ?`,
		`DELETE FROM device_credentials WHERE device_id = ?`,
		`DELETE FROM pairing_sessions WHERE device_id = ?`,
		`DELETE FROM devices WHERE device_id = ?`,
	} {
		if _, err := tx.Exec(q, deviceID); err != nil {
			return false, fmt.Errorf("%s: %w", q, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	delete(m.devs, deviceID)
	m.log.Info("device deleted (credentials revoked)", "device_id", deviceID)
	return true, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
