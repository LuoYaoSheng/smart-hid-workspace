package settings

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"smart-hid-controlhub/internal/storage"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := storage.New(filepath.Join(t.TempDir(), "settings.db"), log)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store.DB)
}

func TestGet_Defaults(t *testing.T) {
	s := newStore(t)
	if v := s.Get(KeyLANModeEnabled); v != "false" {
		t.Errorf("lan default = %q, want false", v)
	}
	if v := s.Get("demo_int_key"); v != "" {
		t.Errorf("unknown int key default = %q, want empty", v)
	}
	if v := s.Get("nonexistent_key"); v != "" {
		t.Errorf("unknown key = %q, want empty", v)
	}
}

func TestSetGet_RoundTrip(t *testing.T) {
	s := newStore(t)
	if err := s.Set("custom_key", "hello"); err != nil {
		t.Fatal(err)
	}
	if v := s.Get("custom_key"); v != "hello" {
		t.Errorf("custom_key = %q", v)
	}
	// UPSERT
	if err := s.Set("custom_key", "world"); err != nil {
		t.Fatal(err)
	}
	if v := s.Get("custom_key"); v != "world" {
		t.Errorf("after upsert = %q", v)
	}
}

func TestBool(t *testing.T) {
	s := newStore(t)
	if !s.GetBool(KeyLANModeEnabled, true) { // 默认 "false" 应胜过 defaultVal
		// Defaults[lan_mode_enabled]="false" 优先
	}
	// 实际：Get 返 "false" → GetBool 解析 → false
	if s.GetBool(KeyLANModeEnabled, true) != false {
		t.Errorf("GetBool(lan) = true, want false (default wins)")
	}
	if err := s.SetBool(KeyLANModeEnabled, true); err != nil {
		t.Fatal(err)
	}
	if !s.GetBool(KeyLANModeEnabled, false) {
		t.Errorf("after SetBool true, GetBool = false")
	}
}

func TestInt(t *testing.T) {
	s := newStore(t)
	if v := s.GetInt("demo_int_key", 999); v != 999 {
		t.Errorf("unknown key fallback = %d, want 999", v)
	}
	if err := s.SetInt("demo_int_key", 3600); err != nil {
		t.Fatal(err)
	}
	if v := s.GetInt("demo_int_key", 0); v != 3600 {
		t.Errorf("after SetInt = %d, want 3600", v)
	}
	// 非数字 fallback
	_ = s.Set("garbage", "not-a-number")
	if v := s.GetInt("garbage", 42); v != 42 {
		t.Errorf("garbage fallback = %d, want 42", v)
	}
}
