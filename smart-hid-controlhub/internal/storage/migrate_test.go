package storage

import (
	"database/sql"
	"log/slog"
	"testing"

	_ "modernc.org/sqlite"
)

// testDB 打开内存 SQLite 并跑全部 migration，返回就绪的 *sql.DB。
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := RunMigrations(db, slog.Default()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestMigrate_FreshDB(t *testing.T) {
	db := testDB(t)
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("query max version: %v", err)
	}
	if version != 3 {
		t.Fatalf("expected latest version 3 (CL-3a 加 0003), got %d", version)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db := testDB(t)
	// 再次运行必须是 no-op
	if err := RunMigrations(db, slog.Default()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 recorded, got %d", n)
	}
}

func TestMigrate_Phase45TablesExist(t *testing.T) {
	db := testDB(t)
	wantTables := []string{
		"schema_migrations", "app_meta", "settings",
		"devices", "commands",
		"device_credentials", "pairing_sessions",
		"trial_usage", "trial_sessions",
		"api_keys", "licenses", "security_events",
	}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	got := map[string]bool{}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		got[n] = true
	}
	_ = rows.Close()
	for _, want := range wantTables {
		if !got[want] {
			t.Errorf("missing table %q", want)
		}
	}
}

func TestMigrate_DevicesColumnsAdded(t *testing.T) {
	db := testDB(t)
	rows, err := db.Query(`PRAGMA table_info(devices)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	got := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		_ = rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk)
		got[name] = true
	}
	_ = rows.Close()
	wantCols := []string{"device_id", "boot_id", "online", "usb_hid_ready", "firmware",
		"last_seen_at", "created_at", // 0001
		"device_name", "paired_at", "is_paired", "machine_anchor", // 0002
	}
	for _, c := range wantCols {
		if !got[c] {
			t.Errorf("missing column devices.%s", c)
		}
	}
}
