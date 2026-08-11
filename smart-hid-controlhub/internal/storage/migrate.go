// Package storage：migration runner。
//
// 版本化迁移机制（CH-P1 引入）：
//   - migrations/ 目录下放 NNNN_*.up.sql 文件（NNNN 为零填充版本号）
//   - schema_migrations(version, applied_at) 表跟踪已应用版本
//   - 启动时按版本号顺序、事务化 apply 未执行的 migration
//   - 幂等：已应用的不会重复执行
//
// 设计权衡：自实现，不引入 golang-migrate，依赖最小。
// 一个 migration 文件 = 一个事务，要么全部成功，要么全部回滚。
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
)

//go:embed migrations/*.up.sql
var migrationFS embed.FS

// migrationFileRe 匹配 NNNN_*.up.sql 形式的文件名。
var migrationFileRe = regexp.MustCompile(`^(\d{4})_.*\.up\.sql$`)

type migrationFile struct {
	version int
	name    string
	content string
}

// RunMigrations 执行所有未应用的 migration。
// 幂等：已记录在 schema_migrations 的版本会被跳过。
func RunMigrations(db *sql.DB, log *slog.Logger) error {
	// 1. 确保跟踪表存在
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// 2. 收集已应用版本
	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("query applied migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fmt.Errorf("scan applied: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close applied rows: %w", err)
	}

	// 3. 列出 embed 中的 migration 文件
	files, err := loadMigrationFiles()
	if err != nil {
		return err
	}

	// 4. 顺序 apply 未应用的
	for _, f := range files {
		if applied[f.version] {
			continue
		}
		if err := applyMigration(db, f); err != nil {
			return err
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

func applyMigration(db *sql.DB, f migrationFile) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx for %s: %w", f.name, err)
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
	return nil
}
