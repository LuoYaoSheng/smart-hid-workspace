// Package settings 管理 ControlHub 的用户可改运行时配置。
//
// 存储介质：settings 表（CH-P1 migration 0002 创建），key/value/updated_at 三列。
// 与 config.yaml 的区别：config 是启动配置（端口、密码、log level），
// settings 是运行时可改的状态（LAN 模式开关等）。
//

package settings

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

// 已知 key 常量。
const (
	// KeyLANModeEnabled 控制 HTTP API 是否监听 0.0.0.0（true）或 127.0.0.1（false）。
	// 验收 A11 "LAN API 需要显式开启"。默认 false。
	KeyLANModeEnabled = "lan_mode_enabled"
)

// Defaults 内置默认值。Get* 找不到 key 时回退。
var Defaults = map[string]string{
	KeyLANModeEnabled: "false",
}

// Store 是 settings 表的 CRUD 封装。
type Store struct {
	db *sql.DB
}

// New 创建 Store。db 必须已跑过 migration。
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Get 返回 key 对应值；不存在则回退到 Defaults[key]；都没有返回 ""。
func (s *Store) Get(key string) string {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		if d, ok := Defaults[key]; ok {
			return d
		}
		return ""
	}
	if err != nil {
		return Defaults[key] // 数据库错误时降级到默认
	}
	return v
}

// Set 设置 key=value，UPSERT 语义。
func (s *Store) Set(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings(key, value, updated_at) VALUES(?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}
	return nil
}

// GetBool 解析 "true"/"false"；其他值或缺失回退到 defaultVal。
func (s *Store) GetBool(key string, defaultVal bool) bool {
	v := s.Get(key)
	switch v {
	case "true":
		return true
	case "false":
		return false
	}
	return defaultVal
}

// SetBool 是 Set(key, strconv.FormatBool(v)) 的便捷封装。
func (s *Store) SetBool(key string, v bool) error {
	return s.Set(key, strconv.FormatBool(v))
}

// GetInt 解析整数；失败回退到 defaultVal。
func (s *Store) GetInt(key string, defaultVal int) int {
	v := s.Get(key)
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// SetInt 是 Set(key, strconv.Itoa(v)) 的便捷封装。
func (s *Store) SetInt(key string, v int) error {
	return s.Set(key, strconv.Itoa(v))
}
