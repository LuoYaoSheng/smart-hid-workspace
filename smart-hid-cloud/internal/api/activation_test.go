// Package api: 激活码消费 + License 刷新测试（CL-6a）。
//
// 覆盖 PUBLIC endpoint（不走 JWT）：
//   - consume：成功/已用/过期/设备不符/通用码绑定/归一化/未知码
//   - refresh：成功/未知 license_id/非 ACTIVE
//   - admin extend → refresh 续期闭环
package api

import (
	"net/http/httptest"
	"testing"
	"time"

	cloudlic "smart-hid-cloud/pkg/license"
)

// consume 调 /api/v1/activation/consume，返 (状态码, raw body)。
func consume(t *testing.T, ts *httptest.Server, code, deviceID string) (int, []byte) {
	t.Helper()
	c, _, raw := doReq(t, ts, "POST", "/api/v1/activation/consume",
		map[string]string{"code": code, "device_id": deviceID}, "")
	return c, raw
}

// genCode 走 admin endpoint 生成激活码，返 (code, license_id)。deviceID="" 生成通用码。
func genCode(t *testing.T, ts *httptest.Server, adminTok, uid, deviceID string) (string, string) {
	t.Helper()
	body := map[string]string{"user_id": uid, "plan_id": "plan_test"}
	if deviceID != "" {
		body["device_id"] = deviceID
	}
	c, j, _ := doReq(t, ts, "POST", "/api/v1/admin/activation-codes", body, adminTok)
	if c != 201 {
		t.Fatalf("genCode: expected 201, got %d %v", c, j)
	}
	code, _ := j["code"].(string)
	licID, _ := j["license_id"].(string)
	return code, licID
}

// TestConsume_HappyPath 预绑定码 + 匹配设备 → 200 + 签名 License（验签通过）+ 码标记已用。
func TestConsume_HappyPath(t *testing.T) {
	ts, pub, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)

	code, licID := genCode(t, ts, adminTok, uid, "HID-AAAAAAAA")

	c, raw := consume(t, ts, code, "HID-AAAAAAAA")
	if c != 200 {
		t.Fatalf("consume: expected 200, got %d %s", c, raw)
	}
	// 验签返回的 License
	l, err := cloudlic.Decode(raw)
	if err != nil {
		t.Fatalf("decode returned license: %v", err)
	}
	if err := cloudlic.VerifyFull(l, pub, "HID-AAAAAAAA", time.Now().Unix()); err != nil {
		t.Fatalf("VerifyFull: %v", err)
	}
	if l.Payload.LicenseID != licID {
		t.Fatalf("license_id mismatch: got %s want %s", l.Payload.LicenseID, licID)
	}
	// DB：license ACTIVE，码 used
	dbLic, _ := st.GetLicense(licID)
	if dbLic.Status != "ACTIVE" {
		t.Fatalf("db license status: expected ACTIVE, got %s", dbLic.Status)
	}
	ac, _ := st.GetActivationCode(code)
	if ac.UsedAt == nil {
		t.Fatalf("activation code not marked used")
	}
}

// TestConsume_AlreadyUsed 同一码消费两次：第二次 409。
func TestConsume_AlreadyUsed(t *testing.T) {
	ts, _, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)
	code, _ := genCode(t, ts, adminTok, uid, "HID-AAAAAAAA")

	if c, _ := consume(t, ts, code, "HID-AAAAAAAA"); c != 200 {
		t.Fatalf("first consume: expected 200, got %d", c)
	}
	c, _ := consume(t, ts, code, "HID-AAAAAAAA")
	if c != 409 {
		t.Fatalf("second consume: expected 409, got %d", c)
	}
}

// TestConsume_Expired 码过期 → 409。
func TestConsume_Expired(t *testing.T) {
	ts, _, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)
	code, _ := genCode(t, ts, adminTok, uid, "HID-AAAAAAAA")

	// 手动把码过期时间改成过去
	if _, err := st.DB.Exec(
		`UPDATE activation_codes SET expires_at = ? WHERE code = ?`,
		time.Now().Unix()-1, code); err != nil {
		t.Fatal(err)
	}
	c, _ := consume(t, ts, code, "HID-AAAAAAAA")
	if c != 409 {
		t.Fatalf("expired code: expected 409, got %d", c)
	}
}

// TestConsume_DeviceMismatch 预绑定码 + 不符设备 → 409 device_mismatch。
func TestConsume_DeviceMismatch(t *testing.T) {
	ts, _, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)
	code, _ := genCode(t, ts, adminTok, uid, "HID-AAAAAAAA")

	c, _ := consume(t, ts, code, "HID-BBBBBBBB")
	if c != 409 {
		t.Fatalf("device mismatch: expected 409, got %d", c)
	}
}

// TestConsume_GenericCodeBinds 通用码（device_id 空）→ 任意设备可消费并绑定。
func TestConsume_GenericCodeBinds(t *testing.T) {
	ts, pub, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)
	code, licID := genCode(t, ts, adminTok, uid, "") // 无 device_id

	c, raw := consume(t, ts, code, "HID-CCCCCCCC")
	if c != 200 {
		t.Fatalf("generic code consume: expected 200, got %d", c)
	}
	l, _ := cloudlic.Decode(raw)
	if err := cloudlic.VerifyFull(l, pub, "HID-CCCCCCCC", time.Now().Unix()); err != nil {
		t.Fatalf("VerifyFull on bound device: %v", err)
	}
	dbLic, _ := st.GetLicense(licID)
	if dbLic.DeviceID != "HID-CCCCCCCC" {
		t.Fatalf("generic code should bind device, got device_id=%s", dbLic.DeviceID)
	}
}

// TestConsume_Normalize 带连字符输入 → 归一化后匹配。
func TestConsume_Normalize(t *testing.T) {
	ts, _, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)
	code, _ := genCode(t, ts, adminTok, uid, "HID-AAAAAAAA")

	// 插入连字符（归一化去除）
	mangled := code[:6] + "-" + code[6:]
	c, _ := consume(t, ts, mangled, "HID-AAAAAAAA")
	if c != 200 {
		t.Fatalf("normalized consume: expected 200, got %d (mangled=%q)", c, mangled)
	}
}

// TestConsume_UnknownCode 未知码 → 404。
func TestConsume_UnknownCode(t *testing.T) {
	ts, _, _ := newTestServer(t)
	c, _ := consume(t, ts, "NOPE12345678", "HID-AAAAAAAA")
	if c != 404 {
		t.Fatalf("unknown code: expected 404, got %d", c)
	}
}

// TestRefresh_HappyPath ACTIVE license 刷新 → 200 + 可验签的同 payload。
func TestRefresh_HappyPath(t *testing.T) {
	ts, pub, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)
	code, licID := genCode(t, ts, adminTok, uid, "HID-AAAAAAAA")

	// 先消费激活
	if c, _ := consume(t, ts, code, "HID-AAAAAAAA"); c != 200 {
		t.Fatalf("consume: expected 200, got %d", c)
	}
	// 刷新
	c, _, raw := doReq(t, ts, "POST", "/api/v1/license/refresh",
		map[string]string{"license_id": licID}, "")
	if c != 200 {
		t.Fatalf("refresh: expected 200, got %d %s", c, raw)
	}
	// 刷新结果应能验签
	l, err := cloudlic.Decode(raw)
	if err != nil {
		t.Fatalf("decode refreshed license: %v", err)
	}
	if err := cloudlic.VerifyFull(l, pub, "HID-AAAAAAAA", time.Now().Unix()); err != nil {
		t.Fatalf("VerifyFull refreshed: %v", err)
	}
}

// TestRefresh_UnknownLicense 未知 license_id → 404。
func TestRefresh_UnknownLicense(t *testing.T) {
	ts, _, _ := newTestServer(t)
	c, _, _ := doReq(t, ts, "POST", "/api/v1/license/refresh",
		map[string]string{"license_id": "lic_nonexistent"}, "")
	if c != 404 {
		t.Fatalf("refresh unknown: expected 404, got %d", c)
	}
}

// TestRefresh_NonActive UNUSED license（无 payload）刷新 → 409。
func TestRefresh_NonActive(t *testing.T) {
	ts, _, st := newTestServer(t)
	uid, _ := adminToken(t, ts, st)
	// 直接建 UNUSED license，不激活
	lic, err := st.CreateLicense(uid, "plan_test", "", []string{"hid_control"})
	if err != nil {
		t.Fatal(err)
	}
	c, _, _ := doReq(t, ts, "POST", "/api/v1/license/refresh",
		map[string]string{"license_id": lic.LicenseID}, "")
	if c != 409 {
		t.Fatalf("refresh non-active: expected 409, got %d", c)
	}
}

// TestAdminExtend_then_Refresh admin 续期 → 刷新拿到延长后的 expires_at。
func TestAdminExtend_then_Refresh(t *testing.T) {
	ts, pub, st := newTestServer(t)
	uid, adminTok := adminToken(t, ts, st)
	code, licID := genCode(t, ts, adminTok, uid, "HID-AAAAAAAA")
	if c, _ := consume(t, ts, code, "HID-AAAAAAAA"); c != 200 {
		t.Fatalf("consume: expected 200, got %d", c)
	}

	// 刷新拿原始 expires_at
	_, _, rawBefore := doReq(t, ts, "POST", "/api/v1/license/refresh",
		map[string]string{"license_id": licID}, "")
	before, _ := cloudlic.Decode(rawBefore)

	// admin extend +30 天
	c, j, _ := doReq(t, ts, "POST", "/api/v1/admin/licenses/"+licID+"/extend",
		map[string]int{"add_days": 30}, adminTok)
	if c != 200 {
		t.Fatalf("extend: expected 200, got %d %v", c, j)
	}

	// 刷新拿新 expires_at
	_, _, rawAfter := doReq(t, ts, "POST", "/api/v1/license/refresh",
		map[string]string{"license_id": licID}, "")
	after, _ := cloudlic.Decode(rawAfter)

	delta := after.Payload.ExpiresAt - before.Payload.ExpiresAt
	if delta != 30*86400 {
		t.Fatalf("extend: expected expires_at + %d, got +%d", 30*86400, delta)
	}
	// 续期后 issued_at 不应倒退（重签；同秒内可能相等），valid_from 不变
	if after.Payload.IssuedAt < before.Payload.IssuedAt {
		t.Fatalf("extend: issued_at went backwards, before=%d after=%d",
			before.Payload.IssuedAt, after.Payload.IssuedAt)
	}
	if after.Payload.ValidFrom != before.Payload.ValidFrom {
		t.Fatalf("extend: valid_from should not change")
	}
	// 验签仍通过
	if err := cloudlic.VerifyFull(after, pub, "HID-AAAAAAAA", time.Now().Unix()); err != nil {
		t.Fatalf("VerifyFull after extend: %v", err)
	}
}
