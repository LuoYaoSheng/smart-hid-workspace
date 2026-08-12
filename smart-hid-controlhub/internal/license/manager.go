// Package licmgr 实现 ControlHub 端的 License 加载、Ed25519 验签、有效性查询。
//
// 共享类型来自 smart-hid-cloud/pkg/license（Payload / License / VerifyFull）。
// 本包提供 ControlHub 本地管理：从 storage 读 license 行、验签、判断有效性、
// 支持激活/导入/刷新。
//
// 包名 `licmgr` 而非 `license` 是为了避免与 cloud pkg（`license`）import 别名冲突。
//
// 设计源：smart-hid-cloud/docs/license-format.md +
// docs/05_CONTROLHUB_DETAIL_DESIGN_V1.0.md §8 + docs/10 §E。
package licmgr

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	cloudlic "smart-hid-cloud/pkg/license"
)

// 有效 License 状态（ControlHub 只承认 ACTIVE；EXPIRED/DISABLED/REVOKED 拒绝）。
const StatusActive = "ACTIVE"

// 错误。
var (
	ErrNoLicense    = errors.New("license: no license for device")
	ErrNotActive    = errors.New("license: status not ACTIVE")
	ErrImportFailed = errors.New("license: import failed")
)

// LicenseInfo 是查询返回的精简视图（不含 signature）。
type LicenseInfo struct {
	LicenseID      string   `json:"license_id"`
	AccountID      string   `json:"account_id"`
	PlanID         string   `json:"plan_id"`
	DeviceID       string   `json:"device_id"`
	IssuedAt       int64    `json:"issued_at"`
	ValidFrom      int64    `json:"valid_from"`
	ExpiresAt      int64    `json:"expires_at"`
	Features       []string `json:"features"`
	LicenseVersion int      `json:"license_version"`
	Status         string   `json:"status"`
	Effective      bool     `json:"effective"`
	TimeRemaining  int64    `json:"time_remaining_seconds"`
}

// Manager 持有公钥 + storage 句柄。
type Manager struct {
	db        *sql.DB
	publicKey []byte // ed25519.PublicKey (32 bytes)
	log       *slog.Logger
}

// New 创建 Manager。公钥从 EmbeddedPublicKeyHex 解析。
func New(db *sql.DB, log *slog.Logger) (*Manager, error) {
	pub, err := cloudlic.ParsePublicKeyHex(EmbeddedPublicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("parse embedded public key: %w", err)
	}
	return &Manager{
		db:        db,
		publicKey: pub,
		log:       log,
	}, nil
}

// NewWithPublicKey 用指定公钥创建 Manager（测试用，避免依赖 embed 常量）。
func NewWithPublicKey(db *sql.DB, publicKey []byte, log *slog.Logger) *Manager {
	return &Manager{db: db, publicKey: publicKey, log: log}
}

// Import 把完整 License JSON（来自 Cloud 下载或拷贝）验签后写入 licenses 表。
// deviceID 是当前 ControlHub 关联的设备 ID（用于 device_id 绑定检查）。
// 同 license_id 已存在则覆盖（支持刷新）。
func (m *Manager) Import(rawJSON []byte, deviceID string) (cloudlic.License, error) {
	l, err := cloudlic.Decode(rawJSON)
	if err != nil {
		return cloudlic.License{}, fmt.Errorf("%w: decode: %v", ErrImportFailed, err)
	}
	// 验签（含 device 绑定 + 时效）
	if err := cloudlic.VerifyFull(l, m.publicKey, deviceID, time.Now().Unix()); err != nil {
		return cloudlic.License{}, fmt.Errorf("%w: verify: %v", ErrImportFailed, err)
	}
	p := l.Payload
	featuresJSON, _ := json.Marshal(p.Features)
	now := time.Now().Unix()
	_, err = m.db.Exec(
		`INSERT INTO licenses(license_id, device_id, plan_id, valid_from, expires_at,
		                      features_json, signature, imported_at, payload_json,
		                      account_id, issued_at, license_version)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		 ON CONFLICT(license_id) DO UPDATE SET
		   device_id=excluded.device_id, plan_id=excluded.plan_id,
		   valid_from=excluded.valid_from, expires_at=excluded.expires_at,
		   features_json=excluded.features_json, signature=excluded.signature,
		   imported_at=excluded.imported_at, payload_json=excluded.payload_json,
		   account_id=excluded.account_id, issued_at=excluded.issued_at,
		   license_version=excluded.license_version`,
		p.LicenseID, p.DeviceID, p.PlanID, p.ValidFrom, p.ExpiresAt,
		string(featuresJSON), l.Signature, now, string(rawJSON),
		p.AccountID, p.IssuedAt,
	)
	if err != nil {
		return cloudlic.License{}, fmt.Errorf("%w: upsert: %v", ErrImportFailed, err)
	}
	m.log.Info("license imported", "license_id", p.LicenseID,
		"device_id", p.DeviceID, "expires_at", p.ExpiresAt)
	return l, nil
}

// LoadForDevice 从 storage 读 license + 验签，返回 LicenseInfo。
// 若无 license 或验签失败，返相应 error。
func (m *Manager) LoadForDevice(deviceID string) (LicenseInfo, error) {
	var (
		info         LicenseInfo
		featuresJSON string
		signature    sql.NullString
		payloadJSON  sql.NullString
		now          = time.Now().Unix()
	)
	// ControlHub license 行没有 status 列（status 是 Cloud 概念）；
	// ControlHub 端"有效"= 验签通过 + expires_at 未过。我们读最新的一行。
	err := m.db.QueryRow(
		`SELECT license_id, account_id, plan_id, device_id,
		        issued_at, valid_from, expires_at, features_json, signature, payload_json
		 FROM licenses WHERE device_id = ?
		 ORDER BY imported_at DESC LIMIT 1`,
		deviceID,
	).Scan(&info.LicenseID, &info.AccountID, &info.PlanID, &info.DeviceID,
		&info.IssuedAt, &info.ValidFrom, &info.ExpiresAt,
		&featuresJSON, &signature, &payloadJSON)
	if err == sql.ErrNoRows {
		return LicenseInfo{}, ErrNoLicense
	}
	if err != nil {
		return LicenseInfo{}, fmt.Errorf("query license: %w", err)
	}
	_ = json.Unmarshal([]byte(featuresJSON), &info.Features)
	info.LicenseVersion = 1
	info.TimeRemaining = info.ExpiresAt - now

	// 验签（仅当有 payload_json + signature 时）
	if payloadJSON.Valid && signature.Valid {
		l, err := cloudlic.Decode([]byte(payloadJSON.String))
		if err == nil {
			if vErr := cloudlic.VerifyFull(l, m.publicKey, deviceID, now); vErr == nil {
				info.Effective = true
				info.Status = StatusActive
				return info, nil
			} else {
				info.Status = "INVALID:" + vErr.Error()
			}
		}
	}
	info.Status = "INVALID"
	return info, nil
}

// IsEffective 检查设备是否有有效 License（验签 + 时效全过）。
// 用于 Entitlement 闸门：license 优先于 trial。
func (m *Manager) IsEffective(deviceID string) bool {
	info, err := m.LoadForDevice(deviceID)
	if err != nil {
		return false
	}
	return info.Effective
}

// ListAll 返回所有已导入的 license（管理用，不含验签）。
func (m *Manager) ListAll() ([]LicenseInfo, error) {
	rows, err := m.db.Query(
		`SELECT license_id, account_id, plan_id, device_id,
		        issued_at, valid_from, expires_at, features_json
		 FROM licenses ORDER BY imported_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().Unix()
	var out []LicenseInfo
	for rows.Next() {
		var info LicenseInfo
		var featuresJSON string
		if err := rows.Scan(&info.LicenseID, &info.AccountID, &info.PlanID,
			&info.DeviceID, &info.IssuedAt, &info.ValidFrom, &info.ExpiresAt,
			&featuresJSON); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(featuresJSON), &info.Features)
		info.LicenseVersion = 1
		info.TimeRemaining = info.ExpiresAt - now
		// 不在这里验签（性能），调用方按需调 LoadForDevice
		if info.ExpiresAt > now && info.ValidFrom <= now {
			info.Status = StatusActive
		} else if info.ExpiresAt <= now {
			info.Status = "EXPIRED"
		} else {
			info.Status = "FUTURE"
		}
		out = append(out, info)
	}
	return out, nil
}

// PublicKeyHex 返回 embed 的公钥 hex（调试/管理用）。
func (m *Manager) PublicKeyHex() string {
	return EmbeddedPublicKeyHex
}
