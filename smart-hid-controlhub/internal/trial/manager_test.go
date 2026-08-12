package trial

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"smart-hid-controlhub/internal/settings"
	"smart-hid-controlhub/internal/storage"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTrialMgr(t *testing.T) (*Manager, *sql.DB) {
	t.Helper()
	store, err := storage.New(filepath.Join(t.TempDir(), "trial.db"), silentLogger())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	setStore := settings.New(store.DB)
	// 测试用：1 秒 idle timeout，10 秒 quota
	_ = setStore.SetInt(settings.KeyTrialIdleTimeoutSeconds, 1)
	_ = setStore.SetInt(settings.KeyTrialQuotaSeconds, 10)

	m := New(NewSQLStore(store.DB), setStore, silentLogger())
	return m, store.DB
}

func seedDevice(t *testing.T, db *sql.DB, deviceID string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO devices(device_id) VALUES(?)`, deviceID)
	if err != nil {
		t.Fatalf("seed device: %v", err)
	}
}

// D1：启动后无 active session，IsControlAllowed 允许。
func TestTrial_D1_StartEmpty(t *testing.T) {
	m, db := newTrialMgr(t)
	seedDevice(t, db, "HID-AAAA1111")

	if !m.IsControlAllowed("HID-AAAA1111") {
		t.Errorf("D1: control should be allowed at startup")
	}
	u := m.Usage("HID-AAAA1111")
	if u.UsedSeconds != 0 {
		t.Errorf("D1: used = %v, want 0", u.UsedSeconds)
	}
	if u.SessionActive {
		t.Errorf("D1: session should not be active at startup")
	}
}

// D2：第一条 executed 才启动 session。
func TestTrial_D2_FirstExecutedStartsSession(t *testing.T) {
	m, db := newTrialMgr(t)
	seedDevice(t, db, "HID-AAAA1111")

	// 触发 executed（execMs=500）
	m.OnCommandExecuted("HID-AAAA1111", 500)

	u := m.Usage("HID-AAAA1111")
	if !u.SessionActive {
		t.Errorf("D2: session should be active after first executed")
	}
	if u.UsedSeconds < 0.4 || u.UsedSeconds > 0.6 {
		t.Errorf("D2: used = %v, want ~0.5", u.UsedSeconds)
	}
}

// D3：无操作 idle 超时后 session 结束（自动 flush）。
// 该测试需 ~12 秒（等 idle loop tick），日常 go test -short 跳过。
func TestTrial_D3_IdleTimeoutEndsSession(t *testing.T) {
	if testing.Short() {
		t.Skip("idle timeout test takes ~12s; skip in -short mode")
	}
	m, db := newTrialMgr(t)
	seedDevice(t, db, "HID-AAAA1111")

	m.OnCommandExecuted("HID-AAAA1111", 500)
	if !m.Usage("HID-AAAA1111").SessionActive {
		t.Fatal("expected active session")
	}

	m.Start()
	defer m.Close()
	// idle 超时 1 秒，检查周期 10 秒。等 12 秒让 idle loop tick 一次。
	time.Sleep(12 * time.Second)

	if m.Usage("HID-AAAA1111").SessionActive {
		t.Errorf("D3: session should be ended after idle timeout")
	}
	if m.Usage("HID-AAAA1111").UsedSeconds < 0.4 {
		t.Errorf("D3: usage should be flushed (~0.5), got %v",
			m.Usage("HID-AAAA1111").UsedSeconds)
	}
}

// D4：Usage 调用不消耗 trial（不触发 OnCommandExecuted）。
func TestTrial_D4_StatusDoesNotConsume(t *testing.T) {
	m, db := newTrialMgr(t)
	seedDevice(t, db, "HID-AAAA1111")

	// 反复查 Usage
	for i := 0; i < 10; i++ {
		_ = m.Usage("HID-AAAA1111")
	}
	if m.Usage("HID-AAAA1111").UsedSeconds != 0 {
		t.Errorf("D4: Usage call consumed trial, used = %v",
			m.Usage("HID-AAAA1111").UsedSeconds)
	}
}

// D5：Trial 过期后 IsControlAllowed 返 false。
func TestTrial_D5_ExpiredBlocksControl(t *testing.T) {
	m, db := newTrialMgr(t)
	seedDevice(t, db, "HID-AAAA1111")

	// quota = 10 秒。模拟消耗 11 秒
	for i := 0; i < 22; i++ {
		m.OnCommandExecuted("HID-AAAA1111", 500) // 0.5s each
	}
	u := m.Usage("HID-AAAA1111")
	if !u.Expired {
		t.Errorf("D5: expected expired, used=%v quota=%d", u.UsedSeconds, u.QuotaSeconds)
	}
	if m.IsControlAllowed("HID-AAAA1111") {
		t.Errorf("D5: control should be blocked after expiry")
	}
}

// 多次 OnCommandExecuted 累加。
func TestTrial_Accumulates(t *testing.T) {
	m, db := newTrialMgr(t)
	seedDevice(t, db, "HID-AAAA1111")

	m.OnCommandExecuted("HID-AAAA1111", 100)
	m.OnCommandExecuted("HID-AAAA1111", 200)
	m.OnCommandExecuted("HID-AAAA1111", 300)

	u := m.Usage("HID-AAAA1111")
	if u.UsedSeconds < 0.55 || u.UsedSeconds > 0.65 {
		t.Errorf("used = %v, want ~0.6", u.UsedSeconds)
	}
}

// Flush 持久化到 trial_usage。
func TestTrial_Flush_Persists(t *testing.T) {
	m, db := newTrialMgr(t)
	seedDevice(t, db, "HID-AAAA1111")

	m.OnCommandExecuted("HID-AAAA1111", 1000) // 1.0 sec
	m.Flush()

	var used float64
	_ = db.QueryRow(
		`SELECT used_seconds FROM trial_usage WHERE device_id = ? AND machine_anchor = ?`,
		"HID-AAAA1111", stubAnchor,
	).Scan(&used)
	if used < 0.9 || used > 1.1 {
		t.Errorf("persisted used = %v, want ~1.0", used)
	}
}

// 重启 Manager 后从 trial_usage 恢复（模拟程序重启）。
func TestTrial_PersistsAcrossRestart(t *testing.T) {
	m1, db := newTrialMgr(t)
	seedDevice(t, db, "HID-AAAA1111")
	m1.OnCommandExecuted("HID-AAAA1111", 5000) // 5 sec
	m1.Flush()

	// 重新构造 Manager（同 DB）
	setStore := settings.New(db)
	_ = setStore.SetInt(settings.KeyTrialIdleTimeoutSeconds, 1)
	_ = setStore.SetInt(settings.KeyTrialQuotaSeconds, 10)
	m2 := New(NewSQLStore(db), setStore, silentLogger())

	u := m2.Usage("HID-AAAA1111")
	if u.UsedSeconds < 4.9 || u.UsedSeconds > 5.1 {
		t.Errorf("after restart, used = %v, want ~5.0", u.UsedSeconds)
	}
	// quota 10 - used 5 = 5 剩余，未过期
	if u.Expired {
		t.Errorf("should not be expired, used 5 / quota 10")
	}
}
