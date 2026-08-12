// Package license 实现 Smart HID License 载荷、Ed25519 签发与验签。
//
// 设计源：smart-hid-cloud/docs/license-format.md（CL-1 替换占位）
//
// 载荷（Payload）字段参与签名；序列化规则：JSON Marshal → 转 map → 重新 Marshal
// （Go 的 json.Marshal 对 map[string]any 按 key 字典序输出，保证签名可重现）。
//
// 完整 License = payload + base64 signature（Ed25519，64 字节签名）。
// 离线 .license 文件 = 完整 License 的 JSON（缩进 2 空格）。
package license

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Version 是当前 License 载荷版本。验签时必须匹配。
const Version = 1

// ID 前缀
const (
	LicenseIDPrefix = "lic_"
	AccountIDPrefix = "acc_"
)

// Payload 是 License 的待签名内容。所有字段都参与签名（顺序无关，canonical
// 阶段会重排为字典序）。
type Payload struct {
	LicenseID      string   `json:"license_id"`
	AccountID      string   `json:"account_id"`
	PlanID         string   `json:"plan_id"`
	DeviceID       string   `json:"device_id"`
	IssuedAt       int64    `json:"issued_at"`
	ValidFrom      int64    `json:"valid_from"`
	ExpiresAt      int64    `json:"expires_at"`
	Features       []string `json:"features"`
	LicenseVersion int      `json:"license_version"`
}

// License 是完整 License 文件（payload + signature）。
type License struct {
	Payload   Payload `json:"payload"`
	Signature string  `json:"signature"` // base64(Ed25519 64-byte)
}

// 验签错误。验签方据错误类型映射 HTTP/日志。
var (
	ErrNoSignature = errors.New("license: missing signature")
	ErrBadEncoding = errors.New("license: signature bad encoding")
	ErrInvalidSig  = errors.New("license: invalid signature")
	ErrExpired     = errors.New("license: expired")
	ErrFutureStart = errors.New("license: valid_from is in the future")
	ErrWrongDevice = errors.New("license: device_id mismatch")
	ErrVersion     = errors.New("license: unsupported license_version")
)

// Canonical 把 Payload 序列化为签名的输入字节。
// 规则：struct marshal → 解码为 map[string]any → 重新 marshal
// （Go 的 json.Marshal 对 map 按 key 字典序输出，结果稳定可重现）。
// 输出为紧凑 JSON（无缩进空格）。
func Canonical(p Payload) ([]byte, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode to map: %w", err)
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal map: %w", err)
	}
	return out, nil
}

// Sign 用 Ed25519 private key 签 payload，返回完整 License。
func Sign(p Payload, privateKey ed25519.PrivateKey) (License, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return License{}, fmt.Errorf("private key size = %d, want %d",
			len(privateKey), ed25519.PrivateKeySize)
	}
	canon, err := Canonical(p)
	if err != nil {
		return License{}, err
	}
	sig := ed25519.Sign(privateKey, canon)
	return License{
		Payload:   p,
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// VerifySignature 仅检查签名（不含时间窗口/device_id/version）。
func VerifySignature(l License, publicKey ed25519.PublicKey) error {
	if l.Signature == "" {
		return ErrNoSignature
	}
	sig, err := base64.StdEncoding.DecodeString(l.Signature)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadEncoding, err)
	}
	canon, err := Canonical(l.Payload)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, canon, sig) {
		return ErrInvalidSig
	}
	return nil
}

// VerifyFull 全面校验：版本 + 签名 + 时间窗口 + device_id 绑定。
// expectedDeviceID 为空字符串时跳过 device_id 检查（管理/调试场景）。
// now 是当前 Unix 秒，便于测试注入时间。
func VerifyFull(l License, publicKey ed25519.PublicKey, expectedDeviceID string, now int64) error {
	if l.Payload.LicenseVersion != Version {
		return fmt.Errorf("%w: got %d want %d",
			ErrVersion, l.Payload.LicenseVersion, Version)
	}
	if err := VerifySignature(l, publicKey); err != nil {
		return err
	}
	if expectedDeviceID != "" && l.Payload.DeviceID != expectedDeviceID {
		return fmt.Errorf("%w: got %s want %s",
			ErrWrongDevice, l.Payload.DeviceID, expectedDeviceID)
	}
	if now < l.Payload.ValidFrom {
		return fmt.Errorf("%w: now=%d valid_from=%d",
			ErrFutureStart, now, l.Payload.ValidFrom)
	}
	if now > l.Payload.ExpiresAt {
		return fmt.Errorf("%w: now=%d expires=%d",
			ErrExpired, now, l.Payload.ExpiresAt)
	}
	return nil
}

// Encode 把 License 序列化为 JSON（缩进 2 空格，便于 .license 文件人读）。
func Encode(l License) ([]byte, error) {
	return json.MarshalIndent(l, "", "  ")
}

// Decode 反序列化 .license 文件内容。
func Decode(b []byte) (License, error) {
	var l License
	if err := json.Unmarshal(b, &l); err != nil {
		return License{}, fmt.Errorf("decode license: %w", err)
	}
	return l, nil
}

// NewPayload 工厂：生成新 Payload（LicenseID 自动生成；IssuedAt/ValidFrom 默认 now）。
// expiresAt 必须由调用方指定（来自 plan.duration_days）。
func NewPayload(accountID, planID, deviceID string, validFrom, expiresAt int64, features []string) Payload {
	now := time.Now().Unix()
	if validFrom == 0 {
		validFrom = now
	}
	return Payload{
		LicenseID:      LicenseIDPrefix + randHex(11), // 22 hex chars
		AccountID:      accountID,
		PlanID:         planID,
		DeviceID:       deviceID,
		IssuedAt:       now,
		ValidFrom:      validFrom,
		ExpiresAt:      expiresAt,
		Features:       features,
		LicenseVersion: Version,
	}
}

// NewAccountID 生成新 account_id（acc_ + 22 hex）。
func NewAccountID() string {
	return AccountIDPrefix + randHex(11)
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// rand.Read 极少失败；fallback 用时间戳，确保不返回空
		return fmt.Sprintf("%022x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
