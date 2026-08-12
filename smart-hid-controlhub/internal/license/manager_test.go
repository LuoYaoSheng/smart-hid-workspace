package licmgr

import (
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	cloudlic "smart-hid-cloud/pkg/license"
	"smart-hid-controlhub/internal/storage"
)

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestMgr(t *testing.T) (*Manager, []byte) {
	t.Helper()
	store, err := storage.New(filepath.Join(t.TempDir(), "lic.db"), silentLog())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	priv, pub, err := cloudlic.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewWithPublicKey(store.DB, pub, silentLog())
	return mgr, priv
}

// 签一个测试 License（用测试私钥）。
func signTest(t *testing.T, priv []byte, deviceID string) []byte {
	t.Helper()
	// priv 是 ed25519.PrivateKey（64 字节）
	now := time.Now().Unix()
	p := cloudlic.Payload{
		LicenseID:      "lic_test" + deviceID,
		AccountID:      "acc_test",
		PlanID:         "plan_test",
		DeviceID:       deviceID,
		IssuedAt:       now,
		ValidFrom:      now,
		ExpiresAt:      now + 365*86400,
		Features:       []string{"hid_control"},
		LicenseVersion: cloudlic.Version,
	}
	l, err := cloudlic.Sign(p, priv)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := cloudlic.Encode(l)
	return b
}

func TestImport_HappyPath(t *testing.T) {
	mgr, priv := newTestMgr(t)
	raw := signTest(t, priv, "HID-AAAA1111")
	l, err := mgr.Import(raw, "HID-AAAA1111")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if l.Payload.DeviceID != "HID-AAAA1111" {
		t.Errorf("device = %s", l.Payload.DeviceID)
	}
}

func TestImport_WrongDevice(t *testing.T) {
	mgr, priv := newTestMgr(t)
	raw := signTest(t, priv, "HID-AAAA1111")
	// 用错 device 验签
	_, err := mgr.Import(raw, "HID-WRONG01")
	if err == nil {
		t.Errorf("import with wrong device should fail")
	}
}

func TestImport_TamperedSignature(t *testing.T) {
	mgr, priv := newTestMgr(t)
	raw := signTest(t, priv, "HID-AAAA1111")
	// 篡改：改 signature 末尾
	var l map[string]any
	_ = json.Unmarshal(raw, &l)
	sig, _ := l["signature"].(string)
	if len(sig) > 5 {
		l["signature"] = sig[:len(sig)-5] + "XXXXX"
	}
	tampered, _ := json.Marshal(l)
	_, err := mgr.Import(tampered, "HID-AAAA1111")
	if err == nil {
		t.Errorf("import tampered signature should fail")
	}
}

func TestLoadForDevice_AfterImport(t *testing.T) {
	mgr, priv := newTestMgr(t)
	_, _ = mgr.Import(signTest(t, priv, "HID-AAAA1111"), "HID-AAAA1111")

	info, err := mgr.LoadForDevice("HID-AAAA1111")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !info.Effective {
		t.Errorf("expected effective, status=%s", info.Status)
	}
	if info.LicenseID != "lic_testHID-AAAA1111" {
		t.Errorf("license_id = %s", info.LicenseID)
	}
	if info.TimeRemaining <= 0 {
		t.Errorf("time_remaining = %d", info.TimeRemaining)
	}
}

func TestLoadForDevice_NoLicense(t *testing.T) {
	mgr, _ := newTestMgr(t)
	_, err := mgr.LoadForDevice("HID-XXXX0000")
	if err != ErrNoLicense {
		t.Errorf("expected ErrNoLicense, got %v", err)
	}
}

func TestIsEffective(t *testing.T) {
	mgr, priv := newTestMgr(t)
	if mgr.IsEffective("HID-AAAA1111") {
		t.Error("should be false before import")
	}
	_, _ = mgr.Import(signTest(t, priv, "HID-AAAA1111"), "HID-AAAA1111")
	if !mgr.IsEffective("HID-AAAA1111") {
		t.Error("should be true after import")
	}
	if mgr.IsEffective("HID-BBBB2222") {
		t.Error("should be false for unrelated device")
	}
}

func TestImport_Twice_Overwrites(t *testing.T) {
	mgr, priv := newTestMgr(t)
	_, _ = mgr.Import(signTest(t, priv, "HID-AAAA1111"), "HID-AAAA1111")
	// 第二次（同 license_id 因为 signTest 用 device_id 拼）
	_, _ = mgr.Import(signTest(t, priv, "HID-AAAA1111"), "HID-AAAA1111")
	info, _ := mgr.LoadForDevice("HID-AAAA1111")
	if !info.Effective {
		t.Errorf("should remain effective after re-import")
	}
}

func TestNew_UsesEmbeddedKey(t *testing.T) {
	// 验证 New 用 EmbeddedPublicKeyHex（不报错）
	store, _ := storage.New(filepath.Join(t.TempDir(), "x.db"), silentLog())
	defer store.Close()
	mgr, err := New(store.DB, silentLog())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if mgr.PublicKeyHex() != EmbeddedPublicKeyHex {
		t.Errorf("public key hex mismatch")
	}
}
