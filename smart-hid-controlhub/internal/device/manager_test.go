package device

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"smart-hid-controlhub/internal/protocol"
	"smart-hid-controlhub/internal/storage"
)

// newTestStore 在 t.TempDir() 下创建一个全新的 SQLite Store，测试结束自动清理。
func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(path, silentLogger())
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func status(devID, bootID string, online, usb bool, fw string) *protocol.SmartHidStatus {
	return &protocol.SmartHidStatus{
		Protocol:    protocol.ProtocolVersion,
		DeviceID:    devID,
		Online:      online,
		BootID:      bootID,
		USBHIDReady: usb,
		Firmware:    fw,
		Timestamp:   1700000000,
	}
}

func TestNewManager_Empty(t *testing.T) {
	m, err := New(newTestStore(t), silentLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := m.List(); len(got) != 0 {
		t.Fatalf("empty manager should have no devices, got %d", len(got))
	}
	if _, ok := m.Get("HID-ABCD1234"); ok {
		t.Fatal("Get on unknown device should return ok=false")
	}
}

func TestUpsertStatus_NewDevice(t *testing.T) {
	m, _ := New(newTestStore(t), silentLogger())
	m.UpsertStatus(status("HID-ABCD1234", "boot-1", true, true, "1.0.0"))

	d, ok := m.Get("HID-ABCD1234")
	if !ok {
		t.Fatal("device not registered after UpsertStatus")
	}
	if d.BootID != "boot-1" || !d.Online || !d.USBHIDReady || d.Firmware != "1.0.0" {
		t.Fatalf("device fields wrong: %+v", d)
	}
	if d.LastSeenAt.IsZero() {
		t.Fatal("LastSeenAt should be set")
	}
}

func TestUpsertStatus_UpdateExisting(t *testing.T) {
	m, _ := New(newTestStore(t), silentLogger())
	m.UpsertStatus(status("HID-ABCD1234", "boot-1", true, false, "1.0.0"))
	// USB 从未就绪 → 就绪
	m.UpsertStatus(status("HID-ABCD1234", "boot-1", true, true, "1.0.1"))

	d, _ := m.Get("HID-ABCD1234")
	if !d.USBHIDReady {
		t.Error("USBHIDReady should have flipped to true")
	}
	if d.Firmware != "1.0.1" {
		t.Errorf("firmware should update to 1.0.1, got %q", d.Firmware)
	}
	if d.BootID != "boot-1" {
		t.Error("boot_id should stay boot-1")
	}
}

func TestUpsertStatus_RebootDetected(t *testing.T) {
	m, _ := New(newTestStore(t), silentLogger())
	m.UpsertStatus(status("HID-ABCD1234", "boot-1", true, true, "1.0.0"))
	// boot_id 变化表示设备重启
	m.UpsertStatus(status("HID-ABCD1234", "boot-2", true, true, "1.0.0"))

	d, _ := m.Get("HID-ABCD1234")
	if d.BootID != "boot-2" {
		t.Fatalf("boot_id should become boot-2 after reboot, got %q", d.BootID)
	}
}

func TestUpsertStatus_FirmwareRetainedWhenEmpty(t *testing.T) {
	m, _ := New(newTestStore(t), silentLogger())
	m.UpsertStatus(status("HID-ABCD1234", "boot-1", true, true, "1.0.0"))
	// 后续 status 不带 firmware —— 应保留旧值
	m.UpsertStatus(status("HID-ABCD1234", "boot-1", true, true, ""))

	d, _ := m.Get("HID-ABCD1234")
	if d.Firmware != "1.0.0" {
		t.Fatalf("firmware should be retained as 1.0.0, got %q", d.Firmware)
	}
}

func TestIsReady(t *testing.T) {
	m, _ := New(newTestStore(t), silentLogger())

	t.Run("unknown", func(t *testing.T) {
		online, usb, ok := m.IsReady("HID-00000000")
		if ok {
			t.Error("unknown device should return ok=false")
		}
		if online || usb {
			t.Error("unknown device online/usb should be false")
		}
	})
	t.Run("online_usb_ready", func(t *testing.T) {
		m.UpsertStatus(status("HID-11111111", "b", true, true, "1"))
		online, usb, ok := m.IsReady("HID-11111111")
		if !ok || !online || !usb {
			t.Fatalf("expected all true, got online=%v usb=%v ok=%v", online, usb, ok)
		}
	})
	t.Run("offline", func(t *testing.T) {
		m.UpsertStatus(status("HID-22222222", "b", false, true, "1"))
		online, usb, ok := m.IsReady("HID-22222222")
		if !ok {
			t.Fatal("known device should return ok=true")
		}
		if online {
			t.Error("offline device online should be false")
		}
		if !usb {
			t.Error("usb should still be true from status")
		}
	})
	t.Run("online_usb_not_ready", func(t *testing.T) {
		m.UpsertStatus(status("HID-33333333", "b", true, false, "1"))
		online, usb, ok := m.IsReady("HID-33333333")
		if !ok || !online {
			t.Fatal("expected online=true ok=true")
		}
		if usb {
			t.Error("usb should be false")
		}
	})
}

func TestList_SnapshotIsolation(t *testing.T) {
	m, _ := New(newTestStore(t), silentLogger())
	m.UpsertStatus(status("HID-ABCD1234", "b", true, true, "1"))
	before := m.List()
	before[0].BootID = "tampered"
	before[0].Online = false

	d, _ := m.Get("HID-ABCD1234")
	if d.BootID != "b" || !d.Online {
		t.Fatal("mutating List() snapshot must not affect Manager state")
	}
}

func TestGet_SnapshotIsolation(t *testing.T) {
	m, _ := New(newTestStore(t), silentLogger())
	m.UpsertStatus(status("HID-ABCD1234", "b", true, true, "1"))
	d1, _ := m.Get("HID-ABCD1234")
	d1.BootID = "tampered"

	d2, _ := m.Get("HID-ABCD1234")
	if d2.BootID != "b" {
		t.Fatal("mutating Get() snapshot must not affect Manager state")
	}
}

// PersistenceAcrossRestart 验证：设备写入 SQLite 后，新建 Manager 应能恢复设备记录，
// 且按设计 online 被重置为 false（等 status 刷新）。
func TestManager_PersistenceAcrossRestart(t *testing.T) {
	store := newTestStore(t)

	m1, err := New(store, silentLogger())
	if err != nil {
		t.Fatalf("New m1: %v", err)
	}
	m1.UpsertStatus(status("HID-ABCD1234", "boot-1", true, true, "1.0.0"))
	m1.UpsertStatus(status("HID-55556666", "boot-2", true, true, "1.1.0"))

	// 用同一个 store 新建第二个 Manager（模拟进程重启）
	m2, err := New(store, silentLogger())
	if err != nil {
		t.Fatalf("New m2: %v", err)
	}
	got := m2.List()
	if len(got) != 2 {
		t.Fatalf("expected 2 devices restored, got %d", len(got))
	}
	for _, d := range got {
		// 重启后 online 一律 false（等 status 包刷新）—— 设计契约
		if d.Online {
			t.Errorf("device %s should be offline after restart (by design)", d.DeviceID)
		}
		// 但 firmware / boot_id 应从 DB 恢复
		if d.Firmware == "" {
			t.Errorf("device %s firmware should be restored", d.DeviceID)
		}
		if d.BootID == "" {
			t.Errorf("device %s boot_id should be restored", d.DeviceID)
		}
	}
}
