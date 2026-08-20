package pairing

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"smart-hid-controlhub/internal/storage"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newMgr(t *testing.T) (*Manager, *sql.DB) {
	t.Helper()
	store, err := storage.New(filepath.Join(t.TempDir(), "pair.db"), silentLogger())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store.DB, 17891, 60, silentLogger()), store.DB
}

func TestCreateSession_HappyPath(t *testing.T) {
	m, db := newMgr(t)
	token, exp, err := m.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 32 {
		t.Errorf("token len = %d, want 32", len(token))
	}
	if exp == 0 {
		t.Errorf("exp = 0")
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM pairing_sessions WHERE token = ?`, token).Scan(&status)
	if status != "pending" {
		t.Errorf("status = %q, want pending", status)
	}
}

func TestCompleteSession_HappyPath(t *testing.T) {
	m, _ := newMgr(t)
	token, _, _ := m.CreateSession()
	result, err := m.CompleteSession(token, "HID-ABCD1234", "boot-1", "1.0.0", "v1", "192.168.1.8")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.MQTTUsername != "dev_HID-ABCD1234" {
		t.Errorf("username = %s", result.MQTTUsername)
	}
	if result.MQTTCredential == "" {
		t.Errorf("password empty")
	}
	if result.MQTTPort != 17891 {
		t.Errorf("port = %d", result.MQTTPort)
	}
	// 二次使用同 token 应失败（session 一次性）
	_, err = m.CompleteSession(token, "HID-ABCD1234", "boot-1", "1.0.0", "v1", "192.168.1.8")
	if err == nil {
		t.Errorf("reuse token should fail")
	}
}

func TestCompleteSession_InvalidDeviceID(t *testing.T) {
	m, _ := newMgr(t)
	token, _, _ := m.CreateSession()
	_, err := m.CompleteSession(token, "invalid-id", "boot-1", "", "", "")
	if err == nil {
		t.Errorf("invalid device_id should fail")
	}
}

func TestCompleteSession_MissingBootID(t *testing.T) {
	m, _ := newMgr(t)
	token, _, _ := m.CreateSession()
	_, err := m.CompleteSession(token, "HID-ABCD1234", "", "", "", "")
	if err == nil {
		t.Errorf("missing boot_id should fail")
	}
}

func TestCompleteSession_BadToken(t *testing.T) {
	m, _ := newMgr(t)
	_, err := m.CompleteSession("nonexistent-token", "HID-ABCD1234", "boot-1", "", "", "192.168.1.8")
	if err == nil {
		t.Errorf("bad token should fail")
	}
}

func TestIssueDeviceCredentials_RotateAndRevoke(t *testing.T) {
	m, db := newMgr(t)
	u1, p1, err := m.IssueDeviceCredentials("HID-ABCD1234")
	if err != nil {
		t.Fatal(err)
	}
	u2, p2, err := m.IssueDeviceCredentials("HID-ABCD1234") // 二次签发
	if err != nil {
		t.Fatal(err)
	}
	if u1 != u2 {
		t.Errorf("username changed: %s != %s", u1, u2)
	}
	if p1 == p2 {
		t.Errorf("password not rotated")
	}
	// 旧凭据应被 revoke；只有 1 条 active
	var active int
	_ = db.QueryRow(
		`SELECT COUNT(*) FROM device_credentials WHERE device_id = ? AND revoked_at IS NULL`,
		"HID-ABCD1234",
	).Scan(&active)
	if active != 1 {
		t.Errorf("active creds = %d, want 1", active)
	}
	// devices 行应标记 is_paired=1
	var paired int
	_ = db.QueryRow(`SELECT is_paired FROM devices WHERE device_id = ?`, "HID-ABCD1234").Scan(&paired)
	if paired != 1 {
		t.Errorf("devices.is_paired = %d, want 1", paired)
	}
}

func TestGetSession_StatusReflectsExpiry(t *testing.T) {
	m, db := newMgr(t)
	token, _, _ := m.CreateSession()
	// 手动把 expires_at 改成过去
	_, _ = db.Exec(`UPDATE pairing_sessions SET expires_at = ? WHERE token = ?`, 1, token)
	s, err := m.GetSession(token)
	if err != nil || s == nil {
		t.Fatalf("get: %v %v", s, err)
	}
	if s.Status != "expired" {
		t.Errorf("status = %s, want expired (time-based)", s.Status)
	}
}

func TestCleanupExpired(t *testing.T) {
	m, db := newMgr(t)
	token, _, _ := m.CreateSession()
	_, _ = db.Exec(`UPDATE pairing_sessions SET expires_at = ? WHERE token = ?`, 1, token)
	n, err := m.CleanupExpired()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("cleaned = %d, want 1", n)
	}
}
