// Package storage 封装 Cloud 的 SQLite 持久化 + 版本化 migration。
// 复用 ControlHub 的 migration 模式（CH-P1）。
package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.up.sql
var migrationFS embed.FS

var migrationFileRe = regexp.MustCompile(`^(\d{4})_.*\.up\.sql$`)

type migrationFile struct {
	version int
	name    string
	content string
}

// Store 是 Cloud SQLite 句柄封装。
type Store struct {
	DB  *sql.DB
	log *slog.Logger
}

// New 打开 SQLite 并跑全部 migration。
func New(path string, log *slog.Logger) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	if err := RunMigrations(db, log); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	log.Info("cloud sqlite ready", "path", path)
	return &Store{DB: db, log: log}, nil
}

// Close 关闭句柄。
func (s *Store) Close() error { return s.DB.Close() }

// RunMigrations 执行所有未应用的 migration。与 ControlHub 实现一致。
func RunMigrations(db *sql.DB, log *slog.Logger) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("query applied: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	_ = rows.Close()

	files, err := loadMigrationFiles()
	if err != nil {
		return err
	}
	for _, f := range files {
		if applied[f.version] {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin %s: %w", f.name, err)
		}
		if _, err := tx.Exec(f.content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", f.name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
			f.version, time.Now().Unix(),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record %s: %w", f.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", f.name, err)
		}
		log.Info("migration applied", "version", f.version, "file", f.name)
	}
	return nil
}

func loadMigrationFiles() ([]migrationFile, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var files []migrationFile
	for _, e := range entries {
		m := migrationFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("parse version %s: %w", e.Name(), err)
		}
		b, err := fs.ReadFile(migrationFS, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		files = append(files, migrationFile{version: v, name: e.Name(), content: string(b)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })
	return files, nil
}
