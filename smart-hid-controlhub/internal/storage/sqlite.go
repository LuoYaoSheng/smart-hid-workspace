// Package storage 封装 SQLite 持久化。
//
// Phase 1：devices + commands 表（migration 0001）。
// Phase 4/5（CH-P1）：版本化 migration 机制 + schema 扩展（migration 0002）。
// 使用 modernc.org/sqlite（纯 Go，无 CGO），便于跨平台构建。
package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"

	_ "modernc.org/sqlite"
)

// Store 是 SQLite 句柄封装。
type Store struct {
	DB  *sql.DB
	log *slog.Logger
}

// New 打开 SQLite 文件（path 形如 /path/to/controlhub.db）。
// 若库不存在则自动创建 + 执行所有未应用的 migration（见 migrate.go）。
func New(path string, log *slog.Logger) (*Store, error) {
	// DSN：busy_timeout 防止 SQLITE_BUSY；foreign_keys=ON 强制 FK。
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// 单连接，避免写锁竞争（ControlHub 量小，足够）
	db.SetMaxOpenConns(1)

	if err := RunMigrations(db, log); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	log.Info("sqlite ready", "path", path)
	return &Store{DB: db, log: log}, nil
}

// Close 关闭句柄。
//
// modernc.org/sqlite 的 mmap 释放由 finalizer 异步驱动；在 macOS 上紧随其后的
// 目录删除（如测试的 t.TempDir 清理）偶发 "directory not empty"。Close 后强制
// GC + 归还内存，让 munmap 在本调用内完成（对生产 shutdown 也无害）。
func (s *Store) Close() error {
	err := s.DB.Close()
	runtime.GC()
	debug.FreeOSMemory()
	return err
}
