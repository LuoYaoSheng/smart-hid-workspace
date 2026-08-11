// Package storage 封装 SQLite 持久化。
// Phase 1：devices + commands 表 + migration 机制（go:embed 内嵌 SQL）。
// 使用 modernc.org/sqlite（纯 Go，无 CGO），便于跨平台构建。
package storage

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_init.sql
var initSQL string

// Store 是 SQLite 句柄封装。
type Store struct {
	DB  *sql.DB
	log *slog.Logger
}

// New 打开 SQLite 文件（path 形如 /path/to/controlhub.db）。
// 若库不存在则自动创建 + 执行 migration。
func New(path string, log *slog.Logger) (*Store, error) {
	// DSN：busy_timeout 防止 SQLITE_BUSY；foreign_keys=ON 强制 FK。
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// 单连接，避免写锁竞争（Phase 1 量小，足够）
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(initSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migration 001: %w", err)
	}

	log.Info("sqlite ready", "path", path)
	return &Store{DB: db, log: log}, nil
}

// Close 关闭句柄。
func (s *Store) Close() error {
	return s.DB.Close()
}
