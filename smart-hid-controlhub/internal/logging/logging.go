// Package logging 提供 slog 结构化日志封装。
// Phase 1：输出到 stdout（开发期）；Phase 5 接入 lumberjack 滚动文件。
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger 根据 level 字符串（debug/info/warn/error）构造 *slog.Logger。
// 未识别的 level 回退到 info。
func NewLogger(level string) *slog.Logger {
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
	opts := &slog.HandlerOptions{Level: lv}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}
