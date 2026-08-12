package entitle

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	cloudlic "smart-hid-cloud/pkg/license"
	"smart-hid-controlhub/internal/license"
	"smart-hid-controlhub/internal/settings"
	"smart-hid-controlhub/internal/storage"
	"smart-hid-controlhub/internal/trial"
)

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// 装配测试 license + trial + storage。
type testDeps struct {
	db        *sql.DB
	license   *licmgr.Manager
	trial     *trial.Manager
	privKey   []byte
}

func newDeps(t *testing.T) testDeps {
	t.Helper()
	store, err := storage.New(filepath.Join(t.TempDir(), "entitle.db"), silentLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	priv, pub, err := cloudlic.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	lm := licmgr.NewWithPublicKey(store.DB, pub, silentLog())

	setStore := settings.New(store.DB)
	_ = setStore.SetInt(settings.KeyTrialQuotaSeconds, 10)   // 10 秒 trial
	_ = setStore.SetInt(settings.KeyTrialIdleTimeoutSeconds, 60)
	tm := trial.New(trial.NewSQLStore(store.DB), setStore, "test-anchor", silentLog())

	return testDeps{db: store.DB, license: lm, trial: tm, privKey: priv}
}

func signLicense(t *testing.T, priv []byte, deviceID string) []byte {
	t.Helper()
	now := time.Now().Unix()
	p := cloudlic.Payload{
		LicenseID: "lic_" + deviceID, AccountID: "acc_x", PlanID: "plan_x",
		DeviceID: deviceID, IssuedAt: now, ValidFrom: now,
		ExpiresAt: now + 86400, Features: []string{"hid_control"},
		LicenseVersion: cloudlic.Version,
	}
	l, _ := cloudlic.Sign(p, priv)
	b, _ := cloudlic.Encode(l)
	return b
}

// 无 License + Trial 充足 → 允许（Trial 决策）
func TestGate_TrialOnly_AllowsWhenQuota(t *testing.T) {
	d := newDeps(t)
	g := New(d.license, d.trial, silentLog())
	// Trial 默认 quota=10s，used=0 → 允许
	if !g.IsControlAllowed("HID-AAAA1111") {
		t.Error("should be allowed by trial")
	}
	dec, ok := g.DecisionFor("HID-AAAA1111")
	if !ok || dec != DecisionTrial {
		t.Errorf("decision = %v ok=%v, want Trial/true", dec, ok)
	}
}

// 无 License + Trial 耗尽 → 拒绝
func TestGate_TrialExhausted_Denies(t *testing.T) {
	d := newDeps(t)
	// 模拟 trial 用尽（消耗 11 秒 > quota 10 秒）
	for i := 0; i < 22; i++ {
		d.trial.OnCommandExecuted("HID-AAAA1111", 500)
	}
	g := New(d.license, d.trial, silentLog())
	if g.IsControlAllowed("HID-AAAA1111") {
		t.Error("should be denied (trial exhausted)")
	}
}

// 有有效 License → 允许（License 决策，trial 状态无关）
func TestGate_LicenseAllows_EvenWhenTrialExhausted(t *testing.T) {
	d := newDeps(t)
	// 先耗尽 trial
	for i := 0; i < 22; i++ {
		d.trial.OnCommandExecuted("HID-AAAA1111", 500)
	}
	// 导入有效 license
	raw := signLicense(t, d.privKey, "HID-AAAA1111")
	if _, err := d.license.Import(raw, "HID-AAAA1111"); err != nil {
		t.Fatal(err)
	}
	g := New(d.license, d.trial, silentLog())
	if !g.IsControlAllowed("HID-AAAA1111") {
		t.Error("license should override exhausted trial")
	}
	dec, _ := g.DecisionFor("HID-AAAA1111")
	if dec != DecisionLicense {
		t.Errorf("decision = %v, want License", dec)
	}
}

// License 过期 → 回退 Trial
func TestGate_LicenseExpired_FallsBackToTrial(t *testing.T) {
	d := newDeps(t)
	// 签一个已过期的 license
	now := time.Now().Unix()
	p := cloudlic.Payload{
		LicenseID: "lic_expired", AccountID: "acc_x", PlanID: "plan_x",
		DeviceID: "HID-AAAA1111",
		IssuedAt: now - 86400, ValidFrom: now - 86400, ExpiresAt: now - 3600,
		Features: []string{"hid_control"}, LicenseVersion: cloudlic.Version,
	}
	l, _ := cloudlic.Sign(p, d.privKey)
	raw, _ := cloudlic.Encode(l)
	_, _ = d.license.Import(raw, "HID-AAAA1111") // 会被 IsEffective=false（过期）

	g := New(d.license, d.trial, silentLog())
	// trial 还充足 → 允许
	if !g.IsControlAllowed("HID-AAAA1111") {
		t.Error("expired license should fall back to trial")
	}
	dec, _ := g.DecisionFor("HID-AAAA1111")
	if dec != DecisionTrial {
		t.Errorf("decision = %v, want Trial (license expired)", dec)
	}
}

// 都无（License 没 + trial 未启用）→ 拒绝
func TestGate_None_Denies(t *testing.T) {
	g := New(nil, nil, silentLog())
	if g.IsControlAllowed("HID-AAAA1111") {
		t.Error("nil license + nil trial should deny")
	}
}

// License 跨设备：A 设备有 license，B 设备无 → A 允许，B 走 Trial
func TestGate_PerDeviceIsolation(t *testing.T) {
	d := newDeps(t)
	raw := signLicense(t, d.privKey, "HID-AAAA1111")
	_, _ = d.license.Import(raw, "HID-AAAA1111")
	g := New(d.license, d.trial, silentLog())

	decA, okA := g.DecisionFor("HID-AAAA1111")
	decB, okB := g.DecisionFor("HID-BBBB2222")
	if !okA || decA != DecisionLicense {
		t.Errorf("A decision = %v ok=%v, want License", decA, okA)
	}
	if !okB || decB != DecisionTrial {
		t.Errorf("B decision = %v ok=%v, want Trial", decB, okB)
	}
}
