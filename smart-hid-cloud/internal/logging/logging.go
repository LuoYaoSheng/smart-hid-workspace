// Package logging 提供 slog JSON 日志器（与 ControlHub 一致）。
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger 返回输出到 stdout 的 JSON slog Logger。
// level 支持 debug/info/warn/error（不区分大小写）。
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
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}
