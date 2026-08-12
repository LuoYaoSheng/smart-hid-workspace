package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
	"time"
)

// 生成测试用 keypair（每个测试独立）。
func testKeypair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	priv, pub, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("gen keypair: %v", err)
	}
	return priv, pub
}

func samplePayload() Payload {
	return Payload{
		LicenseID:      "lic_abcd1234efgh5678ijkl90",
		AccountID:      "acc_test1234567890abcdefghij",
		PlanID:         "plan_basic_yearly",
		DeviceID:       "HID-AAAA1111",
		IssuedAt:       1723440000,
		ValidFrom:      1723440000,
		ExpiresAt:      1754976000,
		Features:       []string{"hid_control"},
		LicenseVersion: Version,
	}
}

func TestSignVerify_HappyPath(t *testing.T) {
	priv, pub := testKeypair(t)
	p := samplePayload()
	l, err := Sign(p, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if l.Signature == "" {
		t.Fatal("empty signature")
	}
	// 验签通过（在有效期内、device 匹配）
	if err := VerifyFull(l, pub, "HID-AAAA1111", 1730000000); err != nil {
		t.Errorf("verify: %v", err)
	}
}

func TestVerify_TamperedPayload(t *testing.T) {
	priv, pub := testKeypair(t)
	l, _ := Sign(samplePayload(), priv)
	// 篡改 payload
	l.Payload.DeviceID = "HID-HACKED0"
	if err := VerifySignature(l, pub); err == nil {
		t.Errorf("VerifySignature passed on tampered payload")
	}
}

func TestVerify_WrongPublicKey(t *testing.T) {
	priv1, _ := testKeypair(t)
	_, pub2 := testKeypair(t)
	l, _ := Sign(samplePayload(), priv1)
	if err := VerifySignature(l, pub2); err == nil {
		t.Errorf("VerifySignature passed with wrong public key")
	}
}

func TestVerifyFull_Expired(t *testing.T) {
	priv, pub := testKeypair(t)
	l, _ := Sign(samplePayload(), priv)
	// now = 1800000000 > expires_at = 1754976000
	err := VerifyFull(l, pub, "HID-AAAA1111", 1800000000)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected expired, got %v", err)
	}
}

func TestVerifyFull_FutureStart(t *testing.T) {
	priv, pub := testKeypair(t)
	p := samplePayload()
	p.ValidFrom = time.Now().Unix() + 3600 // 1 小时后生效
	l, _ := Sign(p, priv)
	err := VerifyFull(l, pub, "HID-AAAA1111", time.Now().Unix())
	if err == nil || !strings.Contains(err.Error(), "future") {
		t.Errorf("expected future start, got %v", err)
	}
}

func TestVerifyFull_WrongDevice(t *testing.T) {
	priv, pub := testKeypair(t)
	l, _ := Sign(samplePayload(), priv)
	err := VerifyFull(l, pub, "HID-OTHER01", 1730000000)
	if err == nil || !strings.Contains(err.Error(), "device_id mismatch") {
		t.Errorf("expected device mismatch, got %v", err)
	}
}

func TestVerifyFull_AllowAnyDevice(t *testing.T) {
	priv, pub := testKeypair(t)
	l, _ := Sign(samplePayload(), priv)
	// expectedDeviceID="" 跳过 device 检查（管理场景）
	if err := VerifyFull(l, pub, "", 1730000000); err != nil {
		t.Errorf("expected pass with empty device_id, got %v", err)
	}
}

func TestVerifyFull_BadVersion(t *testing.T) {
	priv, pub := testKeypair(t)
	p := samplePayload()
	p.LicenseVersion = 99
	l, _ := Sign(p, priv)
	err := VerifyFull(l, pub, "HID-AAAA1111", 1730000000)
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("expected version mismatch, got %v", err)
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	priv, _ := testKeypair(t)
	l, _ := Sign(samplePayload(), priv)
	enc, err := Encode(l)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := Decode(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(dec.Payload, l.Payload) {
		t.Errorf("payload mismatch:\n got  %+v\n want %+v", dec.Payload, l.Payload)
	}
	if dec.Signature != l.Signature {
		t.Errorf("signature mismatch")
	}
}

// 验证签名输入确实是 base64。
func TestSignature_IsBase64(t *testing.T) {
	priv, _ := testKeypair(t)
	l, _ := Sign(samplePayload(), priv)
	if _, err := base64.StdEncoding.DecodeString(l.Signature); err != nil {
		t.Errorf("signature not base64: %v", err)
	}
}

// Canonical 必须稳定（同 payload 两次调用结果相同）。
func TestCanonical_Stable(t *testing.T) {
	p := samplePayload()
	c1, _ := Canonical(p)
	c2, _ := Canonical(p)
	if string(c1) != string(c2) {
		t.Errorf("canonical not stable")
	}
}

// 字段顺序无关：调整 struct 字段值不影响 canonical（同 payload）。
// 但不同 payload 必须产生不同 canonical。
func TestCanonical_Distinct(t *testing.T) {
	p1 := samplePayload()
	p2 := samplePayload()
	p2.DeviceID = "HID-BBBB2222"
	c1, _ := Canonical(p1)
	c2, _ := Canonical(p2)
	if string(c1) == string(c2) {
		t.Errorf("distinct payloads produced same canonical")
	}
}

func TestKeypair_SaveLoad(t *testing.T) {
	priv, pub := testKeypair(t)
	tmpFile := t.TempDir() + "/priv.key"
	if err := SavePrivateKey(tmpFile, priv); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPrivateKey(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytesEqual(priv, loaded) {
		t.Errorf("loaded priv != original")
	}
	// 公钥 hex 转换
	hexStr := PublicKeyHex(pub)
	parsed, err := ParsePublicKeyHex(hexStr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytesEqual(pub, parsed) {
		t.Errorf("parsed pub != original")
	}
}

func TestNewPayload_GeneratesIDs(t *testing.T) {
	p := NewPayload("acc_x", "plan_x", "HID-AAAA1111", 0, 999999999, []string{"x"})
	if !strings.HasPrefix(p.LicenseID, LicenseIDPrefix) {
		t.Errorf("LicenseID missing prefix: %s", p.LicenseID)
	}
	if p.LicenseVersion != Version {
		t.Errorf("version = %d", p.LicenseVersion)
	}
	if p.IssuedAt == 0 || p.ValidFrom == 0 {
		t.Errorf("timestamps zero")
	}
	// 两次生成 LicenseID 不同（随机）
	p2 := NewPayload("acc_x", "plan_x", "HID-AAAA1111", 0, 999999999, []string{"x"})
	if p.LicenseID == p2.LicenseID {
		t.Errorf("LicenseID collision")
	}
}

func TestPublicFromPrivate(t *testing.T) {
	priv, pub := testKeypair(t)
	extracted := PublicFromPrivate(priv)
	if !bytesEqual(pub, extracted) {
		t.Errorf("PublicFromPrivate mismatch")
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
