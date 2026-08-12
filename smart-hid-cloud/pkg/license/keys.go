// Package license: Ed25519 keypair 生成与存储。
//
// 私钥文件格式：hex 编码的 64 字节 Ed25519 私钥（seed 32 + pub 32）。
// 公钥 hex 字符串：32 字节，嵌入 ControlHub binary。

package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// GenerateKeypair 生成 Ed25519 keypair。
// 返回 (privateKey 64 bytes, publicKey 32 bytes)。
func GenerateKeypair() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("ed25519 generate: %w", err)
	}
	return priv, pub, nil
}

// SavePrivateKey 把私钥以 hex 写到 path（mode 0600）。
// 仅供 smart-hid-cloud 使用，ControlHub 不持有私钥。
func SavePrivateKey(path string, priv ed25519.PrivateKey) error {
	if len(priv) != ed25519.PrivateKeySize {
		return fmt.Errorf("private key size = %d, want %d",
			len(priv), ed25519.PrivateKeySize)
	}
	hexStr := hex.EncodeToString(priv)
	return os.WriteFile(path, []byte(hexStr), 0o600)
}

// LoadPrivateKey 从 path 读 hex 编码的私钥。
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("private key size = %d, want %d",
			len(raw), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(raw), nil
}

// PublicKeyHex 把公钥序列化为 hex 字符串（用于 embed 到 ControlHub）。
func PublicKeyHex(pub ed25519.PublicKey) string {
	return hex.EncodeToString(pub)
}

// ParsePublicKeyHex 把 hex 字符串解析为公钥。
func ParsePublicKeyHex(s string) (ed25519.PublicKey, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key size = %d, want %d",
			len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// PublicFromPrivate 从 64 字节私钥提取公钥（后 32 字节）。
func PublicFromPrivate(priv ed25519.PrivateKey) ed25519.PublicKey {
	return priv.Public().(ed25519.PublicKey)
}
