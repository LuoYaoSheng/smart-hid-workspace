package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileSink_WritesAndRotates(t *testing.T) {
	dir := t.TempDir()
	cur := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	s, err := NewFileSink(dir)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	s.now = func() time.Time { return cur }

	if _, err := s.Write([]byte("day1-line1\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := s.Write([]byte("day1-line2\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	cur = cur.Add(24 * time.Hour) // 跨日 → 滚动
	if _, err := s.Write([]byte("day2-line1\n")); err != nil {
		t.Fatalf("write after rotate: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	day1, err := os.ReadFile(filepath.Join(dir, "controlhub-20260918.log"))
	if err != nil || string(day1) != "day1-line1\nday1-line2\n" {
		t.Errorf("day1 file = %q err=%v, want two appended lines", day1, err)
	}
	day2, err := os.ReadFile(filepath.Join(dir, "controlhub-20260919.log"))
	if err != nil || string(day2) != "day2-line1\n" {
		t.Errorf("day2 file = %q err=%v, want one line", day2, err)
	}
}

func TestFileSink_PruneKeepsSeven(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileSink(dir)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	defer s.Close()
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	s.now = func() time.Time { return cur }
	// 写 10 天 → 磁盘上应只剩最近 7 天
	for i := 0; i < 10; i++ {
		if _, err := s.Write([]byte("x\n")); err != nil {
			t.Fatalf("write day %d: %v", i, err)
		}
		cur = cur.Add(24 * time.Hour)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var logs []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			logs = append(logs, e.Name())
		}
	}
	if len(logs) != 7 {
		t.Errorf("kept %d log files, want 7: %v", len(logs), logs)
	}
	// 最旧的 20260901-20260903 应已被清掉
	for _, gone := range []string{"controlhub-20260901.log", "controlhub-20260902.log", "controlhub-20260903.log"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s should have been pruned", gone)
		}
	}
}

func TestFileSink_CloseThenWriteReopens(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileSink(dir)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	s.now = func() time.Time { return time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC) }
	if _, err := s.Write([]byte("before-close\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("after-close\n")); err != nil {
		t.Fatalf("write after close should reopen: %v", err)
	}
	defer s.Close()
	b, err := os.ReadFile(s.Path())
	if err != nil || string(b) != "before-close\nafter-close\n" {
		t.Errorf("file = %q err=%v, want both lines", b, err)
	}
}

func TestNewLogger_WithSinkWritesBothWays(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileSink(dir)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	defer s.Close()
	log := NewLogger("info", s)
	log.Info("hello-file-sink")
	b, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "hello-file-sink") {
		t.Errorf("file log missing message: %q", b)
	}
}
