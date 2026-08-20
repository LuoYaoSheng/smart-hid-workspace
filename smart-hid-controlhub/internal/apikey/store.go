// Package apikey 实现 ControlHub API key 的持久化、校验、轮换。
//
// 设计源：docs/archive/05_CONTROLHUB_DETAIL_DESIGN_V1.0.md §6（HTTP API 鉴权）+
// §10 验收清单 A12 "API Key 可重新生成"。
//
// 存储：api_keys 表（CH-P1 migration 0002 创建）。
// 安全：明文不落库，仅存 SHA-256 hash。明文只在 EnsureInitial / Rotate 时
// 返回一次，由调用者负责（写 initial-api-key.txt 或一次性返回给客户端）。
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

const (
	// KeyPrefix 所有 ControlHub 签发的 key 都以此开头。
	KeyPrefix = "chk_"
	// keyBytes 随机字节数（32 → hex 后 64 字符 + 前缀）。
	keyBytes = 32
	// keyIDLen key_id 取明文前缀长度（chk_ + 12 hex）。
	keyIDLen = len(KeyPrefix) + 12
)

// Store 管理 api_keys 表。
type Store struct {
	db  *sql.DB
	log *slog.Logger
}

// KeyInfo 列表项（不含明文）。
type KeyInfo struct {
	KeyID      string `json:"key_id"`
	Label      string `json:"label"`
	CreatedAt  int64  `json:"created_at"`
	RevokedAt  *int64 `json:"revoked_at,omitempty"`
	LastUsedAt *int64 `json:"last_used_at,omitempty"`
	Active     bool   `json:"active"`
}

// New 创建 Store。db 必须已跑过 migration（api_keys 表存在）。
func New(db *sql.DB, log *slog.Logger) *Store {
	return &Store{db: db, log: log}
}

// EnsureInitial 若表为空，生成一个初始 key 持久化，返回明文（仅此一次）。
// 若表已有 key，返回 "" 且 nil error（表示无需初始化）。
func (s *Store) EnsureInitial(label string) (string, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM api_keys`).Scan(&n); err != nil {
		return "", fmt.Errorf("count api_keys: %w", err)
	}
	if n > 0 {
		return "", nil
	}
	raw, err := generate()
	if err != nil {
		return "", err
	}
	if err := s.insert(raw, label); err != nil {
		return "", err
	}
	return raw, nil
}

// Verify 校验 rawKey 是否匹配某个未撤销记录。
// 匹配时异步更新 last_used_at（best-effort）。
// 注：明文不入库，无法用 subtle.ConstantTimeCompare 直接比较原 key；
// 这里通过比较 SHA-256 hash 实现（hash 抗碰撞，等价强度）。
func (s *Store) Verify(rawKey string) bool {
	if len(rawKey) <= len(KeyPrefix) || rawKey[:len(KeyPrefix)] != KeyPrefix {
		return false
	}
	hash := hashKey(rawKey)
	var keyID string
	err := s.db.QueryRow(
		`SELECT key_id FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL LIMIT 1`,
		hash,
	).Scan(&keyID)
	if err != nil {
		return false
	}
	// 异步更新 last_used_at，不阻塞请求路径
	go func() {
		_, _ = s.db.Exec(
			`UPDATE api_keys SET last_used_at = ? WHERE key_id = ?`,
			time.Now().Unix(), keyID,
		)
	}()
	return true
}

// Rotate 撤销所有当前未撤销的 key，生成新 key 返明文。
// 调用者必须立即返回给客户端一次，之后不可再取。
func (s *Store) Rotate(label string) (string, error) {
	raw, err := generate()
	if err != nil {
		return "", err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	now := time.Now().Unix()
	if _, err := tx.Exec(
		`UPDATE api_keys SET revoked_at = ? WHERE revoked_at IS NULL`, now,
	); err != nil {
		_ = tx.Rollback()
		return "", fmt.Errorf("revoke old: %w", err)
	}
	if err := insertTx(tx, raw, label); err != nil {
		_ = tx.Rollback()
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return raw, nil
}

// List 返回所有 key（不含明文），按创建时间倒序。
func (s *Store) List() ([]KeyInfo, error) {
	rows, err := s.db.Query(
		`SELECT key_id, label, created_at, revoked_at, last_used_at
		 FROM api_keys ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list api_keys: %w", err)
	}
	defer rows.Close()
	var out []KeyInfo
	for rows.Next() {
		var ki KeyInfo
		var revoked, lastUsed sql.NullInt64
		if err := rows.Scan(&ki.KeyID, &ki.Label, &ki.CreatedAt, &revoked, &lastUsed); err != nil {
			return nil, err
		}
		if revoked.Valid {
			v := revoked.Int64
			ki.RevokedAt = &v
		}
		if lastUsed.Valid {
			v := lastUsed.Int64
			ki.LastUsedAt = &v
		}
		ki.Active = !revoked.Valid
		out = append(out, ki)
	}
	return out, nil
}

// InsertTesting 直接持久化一个已知明文 key。仅测试用。
func (s *Store) InsertTesting(rawKey, label string) error {
	return s.insert(rawKey, label)
}

func (s *Store) insert(rawKey, label string) error {
	return insertDB(s.db, rawKey, label)
}

func insertDB(db *sql.DB, rawKey, label string) error {
	_, err := db.Exec(
		`INSERT INTO api_keys(key_id, key_hash, label) VALUES(?, ?, ?)`,
		keyIDFrom(rawKey), hashKey(rawKey), label,
	)
	return err
}

func insertTx(tx *sql.Tx, rawKey, label string) error {
	_, err := tx.Exec(
		`INSERT INTO api_keys(key_id, key_hash, label) VALUES(?, ?, ?)`,
		keyIDFrom(rawKey), hashKey(rawKey), label,
	)
	return err
}

func generate() (string, error) {
	b := make([]byte, keyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return KeyPrefix + hex.EncodeToString(b), nil
}

func hashKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

func keyIDFrom(rawKey string) string {
	if len(rawKey) > keyIDLen {
		return rawKey[:keyIDLen]
	}
	return rawKey
}
