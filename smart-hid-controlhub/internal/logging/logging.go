// Package logging 提供 slog 结构化日志封装。
//
// 输出：JSON 双路——stdout（服务化/开发可见）+ 可选 FileSink 按日落盘
// （data/logs/controlhub-YYYYMMDD.log，保留 7 天）。落盘的意义是观测性：
// 进程异常退出（panic / 崩溃 / 被硬杀）后，事发前的最后事件仍在磁盘上；
// stdout 在 GUI 子系统与服务化场景均不可回收。级别由 config.log_level 控制。
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

// NewLogger 根据 level 字符串（debug/info/warn/error）构造 *slog.Logger。
// 未识别的 level 回退到 info。额外的 sinks 会被并入输出（stdout 始终保留）。
func NewLogger(level string, sinks ...io.Writer) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	var w io.Writer = os.Stdout
	if len(sinks) > 0 {
		w = io.MultiWriter(append([]io.Writer{os.Stdout}, sinks...)...)
	}
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lv})
	return slog.New(handler)
}

// Recover 供后台 goroutine / 外部库回调的 defer 使用：panic 落日志
// （含堆栈）而不是杀死整个进程。此前 broker hook / paho 回调 /
// realtime 泵等 goroutine 裸奔，任一 panic 即整进程消失且无线索。
func Recover(log *slog.Logger, where string) {
	if r := recover(); r != nil {
		log.Error("panic recovered", "where", where, "panic", r, "stack", string(debug.Stack()))
	}
}

// FileSink 是按日滚动的日志文件：dir/<prefix>YYYYMMDD.log。
// 打开/滚动失败时 Write 返回错误（该行仅丢文件路，stdout 路不受影响），
// 日志系统的问题绝不反噬主流程。
type FileSink struct {
	mu     sync.Mutex
	dir    string
	prefix string
	keep   int
	now    func() time.Time // 注入时钟，测试滚动用

	day string
	f   *os.File
}

// NewFileSink 创建 sink、打开当日文件并清理超期旧文件。
// dir 不存在会创建（与 DataDir 同级语义，权限 0755/文件 0600）。
func NewFileSink(dir string) (*FileSink, error) {
	s := &FileSink{dir: dir, prefix: "controlhub-", keep: 7, now: time.Now}
	if err := s.open(s.now()); err != nil {
		return nil, err
	}
	s.pruneLocked()
	return s, nil
}

// Path 返回当前日志文件路径（供启动日志告知用户）。
func (s *FileSink) Path() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return filepath.Join(s.dir, s.prefix+s.day+".log")
}

// Write 实现 io.Writer。slog 每条日志一次 Write，O_APPEND 追加；
// 跨日首次写入触发滚动与清理。
func (s *FileSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.f == nil || now.Format("20060102") != s.day {
		if err := s.open(now); err != nil {
			return 0, err
		}
		s.pruneLocked()
	}
	return s.f.Write(p)
}

// Close 关闭当前文件；此后再写入会重新打开（Close 后的零星日志不丢）。
func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

func (s *FileSink) open(now time.Time) error {
	day := now.Format("20060102")
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.dir, s.prefix+day+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if s.f != nil {
		_ = s.f.Close()
	}
	s.day, s.f = day, f
	return nil
}

// pruneLocked 只保留最近 keep 个日志文件（调用方持锁）。
func (s *FileSink) pruneLocked() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	// 文件名 = prefix + 8 位日期 + ".log"，长度固定可作粗过滤
	want := len(s.prefix) + 8 + len(".log")
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, s.prefix) && strings.HasSuffix(n, ".log") && len(n) == want {
			names = append(names, n)
		}
	}
	sort.Strings(names) // 日期字典序 = 时间序
	for len(names) > s.keep {
		_ = os.Remove(filepath.Join(s.dir, names[0]))
		names = names[1:]
	}
}
