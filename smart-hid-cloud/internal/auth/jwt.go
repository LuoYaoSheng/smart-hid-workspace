// Package auth 实现 HS256 JWT（自实现，避免引第三方库）。
//
// V1 简化：仅支持 HS256（对称密钥），claims 固定 {sub, iat, exp}。
// 生产场景如需 RS256/非对称，可换 golang-jwt 或自扩展。
//
// 设计：
//   token = base64url(header) + "." + base64url(payload) + "." + base64url(HMAC-SHA256(signing_input))
//   signing_input = header + "." + payload
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Claims 是 JWT payload。
type Claims struct {
	UserID string `json:"sub"` // user_id (acc_<22hex>)
	IAT    int64  `json:"iat"` // 签发时间
	EXP    int64  `json:"exp"` // 过期时间
}

const (
	// DefaultTTL 默认 24 小时。
	DefaultTTL = 24 * time.Hour
	tokenTmpl  = `{"alg":"HS256","typ":"JWT"}`
)

// 错误。
var (
	ErrMalformed = errors.New("jwt: malformed token")
	ErrExpired   = errors.New("jwt: expired")
	ErrInvalidSig = errors.New("jwt: invalid signature")
)

// Sign 用 secret 签发 token（默认 24h TTL）。
func Sign(c Claims, secret []byte) (string, error) {
	return SignWithTTL(c, secret, DefaultTTL)
}

// SignWithTTL 自定义 TTL。
func SignWithTTL(c Claims, secret []byte, ttl time.Duration) (string, error) {
	now := time.Now().Unix()
	if c.IAT == 0 {
		c.IAT = now
	}
	if c.EXP == 0 {
		c.EXP = now + int64(ttl.Seconds())
	}
	headerB := base64.RawURLEncoding.EncodeToString([]byte(tokenTmpl))
	payloadJSON, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payloadB := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := headerB + "." + payloadB
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig, nil
}

// Verify 校验 token 签名 + 过期时间，返回 Claims。
func Verify(token string, secret []byte) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrMalformed
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return Claims{}, ErrInvalidSig
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: decode payload: %v", ErrMalformed, err)
	}
	var c Claims
	if err := json.Unmarshal(payloadBytes, &c); err != nil {
		return Claims{}, fmt.Errorf("%w: unmarshal: %v", ErrMalformed, err)
	}
	if time.Now().Unix() > c.EXP {
		return c, ErrExpired
	}
	return c, nil
}
