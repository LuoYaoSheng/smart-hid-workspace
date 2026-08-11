package apikey

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"smart-hid-controlhub/internal/storage"
)

func newStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := storage.New(filepath.Join(t.TempDir(), "keys.db"), log)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store.DB, log), store.DB
}

func TestEnsureInitial_FirstRun(t *testing.T) {
	keys, db := newStore(t)
	raw, err := keys.EnsureInitial("initial")
	if err != nil {
		t.Fatalf("EnsureInitial: %v", err)
	}
	if raw == "" {
		t.Fatal("expected non-empty key on first run")
	}
	if len(raw) <= len(KeyPrefix) || raw[:len(KeyPrefix)] != KeyPrefix {
		t.Errorf("key %q missing prefix %q", raw, KeyPrefix)
	}
	// 表里应有 1 条
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM api_keys`).Scan(&n)
	if n != 1 {
		t.Errorf("after EnsureInitial, api_keys count = %d, want 1", n)
	}
}

func TestEnsureInitial_SecondRunNoOp(t *testing.T) {
	keys, _ := newStore(t)
	if _, err := keys.EnsureInitial("initial"); err != nil {
		t.Fatal(err)
	}
	raw, err := keys.EnsureInitial("initial")
	if err != nil {
		t.Fatal(err)
	}
	if raw != "" {
		t.Errorf("second EnsureInitial returned %q, want empty", raw)
	}
}

func TestVerify_HappyPath(t *testing.T) {
	keys, _ := newStore(t)
	raw, _ := keys.EnsureInitial("initial")
	if !keys.Verify(raw) {
		t.Errorf("Verify returned false for valid key")
	}
}

func TestVerify_Rejected(t *testing.T) {
	keys, _ := newStore(t)
	_, _ = keys.EnsureInitial("initial")
	cases := []string{"", "wrong", "chk_short", "chk_" + "0123456789abcdef"}
	for _, c := range cases {
		if keys.Verify(c) {
			t.Errorf("Verify(%q) = true, want false", c)
		}
	}
}

func TestRotate_OldKeyRevoked(t *testing.T) {
	keys, _ := newStore(t)
	oldRaw, _ := keys.EnsureInitial("initial")
	newRaw, err := keys.Rotate("rotated")
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if newRaw == "" || newRaw == oldRaw {
		t.Fatalf("Rotate returned bad key: %q", newRaw)
	}
	// 旧 key 应失效
	if keys.Verify(oldRaw) {
		t.Errorf("old key still valid after rotate")
	}
	// 新 key 应有效
	if !keys.Verify(newRaw) {
		t.Errorf("new key invalid after rotate")
	}
}

func TestList_HidesRawKey(t *testing.T) {
	keys, _ := newStore(t)
	raw, _ := keys.EnsureInitial("initial")
	_, _ = keys.Rotate("rotated")
	list, err := keys.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
	for _, ki := range list {
		if ki.KeyID == raw {
			t.Errorf("List exposed full raw key as key_id: %q", ki.KeyID)
		}
	}
	// 应该一个 active 一个 revoked
	active, revoked := 0, 0
	for _, ki := range list {
		if ki.Active {
			active++
		} else {
			revoked++
		}
	}
	if active != 1 || revoked != 1 {
		t.Errorf("active=%d revoked=%d, want 1/1", active, revoked)
	}
}
