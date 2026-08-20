// Package pairing 实现 ControlHub 的设备配对流程。
//
// 设计源（历史设计资料，仅溯源用）：
//   - docs/archive/05_CONTROLHUB_DETAIL_DESIGN_V1.0.md §5（Pairing flow）+ §4（端口 17892）
//   - docs/archive/10 验收清单（历史） A7（每设备 Topic ACL）
//   - docs/archive/03 BLE_PROVISIONING_PROTOCOL_V1.1.md（ControlHub Pairing QR 载荷）
//
// 流程：
//  1. 用户在 Web UI 点"添加设备" → POST /api/v1/pairing/sessions（需 API key）
//     → ControlHub 创建 pairing_session（token + 过期时间）→ 返回 token + QR payload
//  2. ESP32 通过 BLE 协议拿到 QR payload（含 token）→
//     POST http://<hub-lan-ip>:17892/api/v1/pairing/device（无 API key，凭 token）
//     → ControlHub 校验 token、签发 per-device MQTT 凭据、标记 session 已用
//     → 返回 {mqtt_host, mqtt_port, mqtt_username, mqtt_credential}
//  3. ESP32 用拿到的凭据连接 MQTT broker（PerDeviceHook 接管鉴权 + per-device ACL）
//
// token：32 字符 hex（16 随机字节）；TTL 默认 5 分钟；session 一次性。
// 设备凭据：dev_<device_id> username + 32 字节随机 password（SHA-256 hash 入库）。
package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"time"
)

// DeviceIDPattern 与 hid-command-schema 一致：^HID-[A-Z0-9]{8}$。
var DeviceIDPattern = regexp.MustCompile(`^HID-[A-Z0-9]{8}$`)

// DefaultTTLSec 默认 session 有效期 5 分钟。
const DefaultTTLSec = 300

// DefaultPairingPort 设备侧 HTTP listener 端口（docs/archive/05 §4）。
const DefaultPairingPort = 17892

// Session 对应 pairing_sessions 表一行。
type Session struct {
	Token     string `json:"token"`
	DeviceID  string `json:"device_id,omitempty"` // 配对成功后填
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	UsedAt    *int64 `json:"used_at,omitempty"`
	Status    string `json:"status"` // pending|success|expired|revoked
}

// PairingResult 是设备从 POST /api/v1/pairing/device 拿到的响应。
type PairingResult struct {
	MQTTHost       string `json:"mqtt_host"`
	MQTTPort       int    `json:"mqtt_port"`
	MQTTUsername   string `json:"mqtt_username"`
	MQTTCredential string `json:"mqtt_credential"` // 一次性，仅此响应可见
	DeviceID       string `json:"device_id"`
}

// Manager 管理 pairing_sessions + device_credentials 表。
type Manager struct {
	db       *sql.DB
	log      *slog.Logger
	ttlSec   int
	mqttHost string // 设备连 broker 用的 host（对外 LAN 地址，由调用方决定）
	mqttPort int
}

// New 创建 Manager。mqttHost/mqttPort 是发给设备的 broker 地址。
// ttlSec <= 0 时用 DefaultTTLSec。
func New(db *sql.DB, mqttHost string, mqttPort, ttlSec int, log *slog.Logger) *Manager {
	if ttlSec <= 0 {
		ttlSec = DefaultTTLSec
	}
	return &Manager{
		db:       db,
		log:      log,
		ttlSec:   ttlSec,
		mqttHost: mqttHost,
		mqttPort: mqttPort,
	}
}

// CreateSession 生成新 token + 入库。返回 token 与过期 Unix 秒。
func (m *Manager) CreateSession() (token string, expiresAt int64, err error) {
	token, err = generateToken()
	if err != nil {
		return "", 0, err
	}
	now := time.Now().Unix()
	expiresAt = now + int64(m.ttlSec)
	_, err = m.db.Exec(
		`INSERT INTO pairing_sessions(token, created_at, expires_at, status) VALUES(?, ?, ?, 'pending')`,
		token, now, expiresAt,
	)
	if err != nil {
		return "", 0, fmt.Errorf("insert pairing_session: %w", err)
	}
	m.log.Info("pairing session created", "token_prefix", token[:8]+"...", "expires_at", expiresAt)
	return token, expiresAt, nil
}

// GetSession 查询 session。若 token 不存在或已过期（status=pending 但 expires_at<now），
// 返回的 Session.Status 反映实际状态，但**不**自动改库（避免读路径副作用）。
func (m *Manager) GetSession(token string) (*Session, error) {
	var s Session
	var deviceID sql.NullString
	var usedAt sql.NullInt64
	err := m.db.QueryRow(
		`SELECT token, device_id, created_at, expires_at, used_at, status
		 FROM pairing_sessions WHERE token = ?`,
		token,
	).Scan(&s.Token, &deviceID, &s.CreatedAt, &s.ExpiresAt, &usedAt, &s.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}
	if deviceID.Valid {
		s.DeviceID = deviceID.String
	}
	if usedAt.Valid {
		v := usedAt.Int64
		s.UsedAt = &v
	}
	// 计算过期态（pending + 过期）
	if s.Status == "pending" && time.Now().Unix() > s.ExpiresAt {
		s.Status = "expired"
	}
	return &s, nil
}

// CompleteSession 设备侧调用：校验 token + device_id，签发凭据，标记 session。
// 返回 PairingResult（含一次性明文密码）。
func (m *Manager) CompleteSession(token, deviceID, bootID, firmware, hardware string) (*PairingResult, error) {
	if !DeviceIDPattern.MatchString(deviceID) {
		return nil, fmt.Errorf("invalid device_id format")
	}
	if bootID == "" {
		return nil, fmt.Errorf("boot_id required")
	}

	s, err := m.GetSession(token)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("token not found")
	}
	if s.Status != "pending" {
		return nil, fmt.Errorf("token %s (already used or revoked)", s.Status)
	}

	// 签发凭据
	username, password, err := m.IssueDeviceCredentials(deviceID)
	if err != nil {
		return nil, err
	}

	// 标记 session
	now := time.Now().Unix()
	_, err = m.db.Exec(
		`UPDATE pairing_sessions SET device_id = ?, used_at = ?, status = 'success' WHERE token = ?`,
		deviceID, now, token,
	)
	if err != nil {
		return nil, fmt.Errorf("mark session success: %w", err)
	}

	m.log.Info("pairing completed",
		"device_id", deviceID, "boot_id", bootID,
		"firmware", firmware, "token_prefix", token[:8]+"...")

	return &PairingResult{
		MQTTHost:       m.mqttHost,
		MQTTPort:       m.mqttPort,
		MQTTUsername:   username,
		MQTTCredential: password,
		DeviceID:       deviceID,
	}, nil
}

// IssueDeviceCredentials 为 deviceID 生成 dev_<device_id> + 32 字节随机密码。
// 撤销旧的 + 写入新 + upsert devices 行（is_paired=1）。返回明文密码（一次性）。
func (m *Manager) IssueDeviceCredentials(deviceID string) (username, password string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("gen password: %w", err)
	}
	password = hex.EncodeToString(raw)
	username = "dev_" + deviceID
	hash := hashSha256(password)
	now := time.Now().Unix()

	tx, err := m.db.Begin()
	if err != nil {
		return "", "", err
	}
	// upsert devices 行（先建 FK 父行；is_paired=1）
	if _, err := tx.Exec(
		`INSERT INTO devices(device_id, boot_id, paired_at, is_paired, machine_anchor)
		 VALUES(?, '', ?, 1, '')
		 ON CONFLICT(device_id) DO UPDATE SET paired_at=excluded.paired_at, is_paired=1`,
		deviceID, now,
	); err != nil {
		_ = tx.Rollback()
		return "", "", fmt.Errorf("upsert device: %w", err)
	}
	// 撤销旧凭据（如有）— 由于 device_credentials PK=device_id，单行旋转模式：
	// 历史用 security_events 留痕，本表只保留最新一份 active 凭据。
	// 写新凭据（INSERT OR REPLACE，等价于"撤销旧 + 写新"的合并语义）
	if _, err := tx.Exec(
		`INSERT INTO device_credentials(device_id, mqtt_username, mqtt_credential_hash, issued_at)
		 VALUES(?, ?, ?, ?)
		 ON CONFLICT(device_id) DO UPDATE SET
		   mqtt_username=excluded.mqtt_username,
		   mqtt_credential_hash=excluded.mqtt_credential_hash,
		   issued_at=excluded.issued_at,
		   revoked_at=NULL`,
		deviceID, username, hash, now,
	); err != nil {
		_ = tx.Rollback()
		return "", "", fmt.Errorf("upsert creds: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return username, password, nil
}

// CleanupExpired 把所有过期未用的 session 标记为 expired。
// 尚未接入周期定时器（当前由 GetSession 的惰性过期判定兜底）；预留接口。
func (m *Manager) CleanupExpired() (int64, error) {
	res, err := m.db.Exec(
		`UPDATE pairing_sessions SET status = 'expired'
		 WHERE status = 'pending' AND expires_at < ?`,
		time.Now().Unix(),
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func generateToken() (string, error) {
	b := make([]byte, 16) // 16 字节 → 32 hex
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashSha256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
