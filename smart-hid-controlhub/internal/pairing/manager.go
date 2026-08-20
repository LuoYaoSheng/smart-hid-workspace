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
	Status    string `json:"status"` // pending|consuming|success|expired|revoked
}

// CompleteSession 消费哨兵错误（HTTP 层映射稳定错误码）。
var (
	ErrTokenNotFound = fmt.Errorf("pairing token not found")
	ErrTokenExpired  = fmt.Errorf("pairing token expired or revoked")
	ErrTokenUsed     = fmt.Errorf("pairing token already used")
)

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
	db      *sql.DB
	log     *slog.Logger
	ttlSec  int
	mqttPort int // broker 端口（固定）；host 由调用方按请求路径解析后传入 CompleteSession
}

// New 创建 Manager。mqttPort 是发给设备的 broker 端口；broker host 不再是
// Manager 的静态字段——每次 CompleteSession 由 DeviceServer 按设备请求路径
// 解析 advertise host 后传入（M1-G3 网络模型拆分）。
// ttlSec <= 0 时用 DefaultTTLSec。
func New(db *sql.DB, mqttPort, ttlSec int, log *slog.Logger) *Manager {
	if ttlSec <= 0 {
		ttlSec = DefaultTTLSec
	}
	return &Manager{
		db:       db,
		log:      log,
		ttlSec:   ttlSec,
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

// CompleteSession 设备侧调用：单事务原子消费 token（0→1 严格一次）并签发凭据。
//
// advertiseHost 必须由调用方（DeviceServer）在消费 token **之前**按设备请求
// 路径解析好并传入——endpoint 解析失败时 token 尚未被消费，用户可直接重试
// （spec M1-G3 §11 顺序：resolve → validate → atomic consume → return）。
//
// 事务内三步：
//  1. CAS 认领：UPDATE ... WHERE status='pending' AND expires_at>=now，RowsAffected==1
//     才是赢家（并发竞争者全部在此落败，不存在 TOCTOU）
//  2. 签发凭据（devices upsert + device_credentials upsert，同事务）
//  3. 标记 success
//
// 任一步失败整体 ROLLBACK —— 不会出现"凭据已发但 session 仍 pending"或反向半状态。
// 返回 PairingResult（含一次性明文密码）；失败返回哨兵错误（ErrTokenUsed/Expired/NotFound）。
func (m *Manager) CompleteSession(token, deviceID, bootID, firmware, hardware, advertiseHost string) (*PairingResult, error) {
	if !DeviceIDPattern.MatchString(deviceID) {
		return nil, fmt.Errorf("invalid device_id format")
	}
	if bootID == "" {
		return nil, fmt.Errorf("boot_id required")
	}
	if advertiseHost == "" {
		return nil, fmt.Errorf("advertise host required (resolve before consuming token)")
	}

	// 随机凭据在内存生成（不涉库，可在事务外）；持久化与 session 状态同事务。
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("gen password: %w", err)
	}
	password := hex.EncodeToString(raw)
	username := "dev_" + deviceID
	hash := hashSha256(password)
	now := time.Now().Unix()

	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 1) CAS 认领（pending + 未过期 → consuming）
	res, err := tx.Exec(
		`UPDATE pairing_sessions SET status='consuming', device_id=?, used_at=?
		 WHERE token=? AND status='pending' AND expires_at >= ?`,
		deviceID, now, token, now,
	)
	if err != nil {
		return nil, fmt.Errorf("claim pairing session: %w", err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		// 认领失败：分类原因（读路径不影响事务，回滚后再查）。
		_ = tx.Rollback()
		committed = true // 已显式回滚，跳过 defer 的二次回滚
		return nil, m.classifyConsumeFailure(token)
	}

	// 2) 凭据 + 设备 upsert（同事务）
	if err := issueDeviceCredentialsTx(tx, deviceID, username, hash, now); err != nil {
		return nil, err // defer 回滚
	}

	// 3) success 标记（同事务）
	if _, err := tx.Exec(`UPDATE pairing_sessions SET status='success' WHERE token=?`, token); err != nil {
		return nil, fmt.Errorf("mark session success: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	m.log.Info("pairing completed",
		"device_id", deviceID, "boot_id", bootID,
		"firmware", firmware, "token_prefix", token[:8]+"...")

	return &PairingResult{
		MQTTHost:       advertiseHost,
		MQTTPort:       m.mqttPort,
		MQTTUsername:   username,
		MQTTCredential: password,
		DeviceID:       deviceID,
	}, nil
}

// classifyConsumeFailure CAS 认领失败后查实际状态，返回哨兵错误。
func (m *Manager) classifyConsumeFailure(token string) error {
	s, err := m.GetSession(token)
	if err != nil {
		return err
	}
	if s == nil {
		return ErrTokenNotFound
	}
	switch s.Status {
	case "success", "consuming":
		return ErrTokenUsed
	case "expired", "revoked":
		return ErrTokenExpired
	default: // pending 且未过期却认领失败 —— 并发窗口理论不可达，防御性返回
		return fmt.Errorf("pairing session state conflict (status=%s)", s.Status)
	}
}

// issueDeviceCredentialsTx 在给定事务内 upsert 设备行 + 每设备凭据（单行旋转模式：
// 历史用 security_events 留痕，本表只保留最新一份 active 凭据）。
func issueDeviceCredentialsTx(tx *sql.Tx, deviceID, username, hash string, now int64) error {
	if _, err := tx.Exec(
		`INSERT INTO devices(device_id, boot_id, paired_at, is_paired, machine_anchor)
		 VALUES(?, '', ?, 1, '')
		 ON CONFLICT(device_id) DO UPDATE SET paired_at=excluded.paired_at, is_paired=1`,
		deviceID, now,
	); err != nil {
		return fmt.Errorf("upsert device: %w", err)
	}
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
		return fmt.Errorf("upsert creds: %w", err)
	}
	return nil
}

// IssueDeviceCredentials 为 deviceID 生成 dev_<device_id> + 32 字节随机密码。
// 撤销旧的 + 写入新 + upsert devices 行（is_paired=1）。返回明文密码（一次性）。
// 独立事务版本；CompleteSession 走 issueDeviceCredentialsTx 与 session 同事务。
func (m *Manager) IssueDeviceCredentials(deviceID string) (username, password string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("gen password: %w", err)
	}
	password = hex.EncodeToString(raw)
	username = "dev_" + deviceID

	tx, err := m.db.Begin()
	if err != nil {
		return "", "", err
	}
	if err := issueDeviceCredentialsTx(tx, deviceID, username, hashSha256(password), time.Now().Unix()); err != nil {
		_ = tx.Rollback()
		return "", "", err
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
