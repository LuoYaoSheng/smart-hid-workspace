// manager_atomic_test.go — M1-G2 配对 token 原子消费回归测试。
package pairing

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
)

// TestPairingTokenConcurrentConsume —— 50 goroutine 同时消费同一 token：
// 恰好 1 个成功；其余 ErrTokenUsed；DB 终态一致（单设备关联、单 active 凭据）。
func TestPairingTokenConcurrentConsume(t *testing.T) {
	m, db := newMgr(t)
	token, _, _ := m.CreateSession()

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	var success, used, other int32
	var successUser atomic.Value // 赢家的 MQTTUsername

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// 一半用设备 A，一半用设备 B：无论谁赢，凭据只属于一个设备。
			dev := "HID-AAAA0000"
			if i%2 == 1 {
				dev = "HID-BBBB0001"
			}
			<-start
			res, err := m.CompleteSession(token, dev, "boot-1", "1.0.0", "v1")
			switch {
			case err == nil:
				atomic.AddInt32(&success, 1)
				successUser.Store(res.MQTTUsername)
			case err == ErrTokenUsed:
				atomic.AddInt32(&used, 1)
			default:
				atomic.AddInt32(&other, 1)
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if success != 1 {
		t.Fatalf("success = %d, want exactly 1", success)
	}
	if used != n-1 {
		t.Fatalf("used = %d, want %d", used, n-1)
	}
	if other != 0 {
		t.Fatalf("other errors = %d, want 0", other)
	}

	// DB 终态：session success 且只关联一个设备。
	var status string
	var devID string
	if err := db.QueryRow(`SELECT status, device_id FROM pairing_sessions WHERE token=?`, token).Scan(&status, &devID); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if status != "success" || devID == "" {
		t.Fatalf("session status=%q device_id=%q, want success + winner device", status, devID)
	}
	// 赢家用户名与 session 关联设备一致。
	if u, ok := successUser.Load().(string); !ok || u != "dev_"+devID {
		t.Fatalf("winner username %v mismatches session device %s", successUser.Load(), devID)
	}
	// 凭据：赢家设备恰好 1 条 active；输家设备无凭据。
	var activeWinner int
	_ = db.QueryRow(`SELECT COUNT(*) FROM device_credentials WHERE device_id=? AND revoked_at IS NULL`, devID).Scan(&activeWinner)
	if activeWinner != 1 {
		t.Fatalf("winner active creds = %d, want 1", activeWinner)
	}
	loser := "HID-AAAA0000"
	if devID == loser {
		loser = "HID-BBBB0001"
	}
	var activeLoser int
	_ = db.QueryRow(`SELECT COUNT(*) FROM device_credentials WHERE device_id=?`, loser).Scan(&activeLoser)
	if activeLoser != 0 {
		t.Fatalf("loser creds = %d, want 0", activeLoser)
	}
}

// TestPairingTokenExpired —— 过期 token：ErrTokenExpired，不签发任何凭据。
func TestPairingTokenExpired(t *testing.T) {
	m, db := newMgr(t)
	token, _, _ := m.CreateSession()
	_, _ = db.Exec(`UPDATE pairing_sessions SET expires_at=1 WHERE token=?`, token)

	if _, err := m.CompleteSession(token, "HID-AAAA0000", "boot-1", "", ""); err != ErrTokenExpired {
		t.Fatalf("err = %v, want ErrTokenExpired", err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM device_credentials`).Scan(&n)
	if n != 0 {
		t.Fatalf("expired consume issued %d creds, want 0", n)
	}
}

// TestPairingTokenInvalid —— 不存在 token：ErrTokenNotFound。
func TestPairingTokenInvalid(t *testing.T) {
	m, _ := newMgr(t)
	if _, err := m.CompleteSession("nonexistent", "HID-AAAA0000", "boot-1", "", ""); err != ErrTokenNotFound {
		t.Fatalf("err = %v, want ErrTokenNotFound", err)
	}
}

// TestPairingTokenAlreadyConsumed —— 已消费 token：ErrTokenUsed。
func TestPairingTokenAlreadyConsumed(t *testing.T) {
	m, _ := newMgr(t)
	token, _, _ := m.CreateSession()
	if _, err := m.CompleteSession(token, "HID-AAAA0000", "boot-1", "", ""); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := m.CompleteSession(token, "HID-BBBB0001", "boot-1", "", ""); err != ErrTokenUsed {
		t.Fatalf("err = %v, want ErrTokenUsed", err)
	}
}

// TestPairingCredentialDBFailureRollback —— 凭据写入失败：
// session 必须回滚到 pending（无半状态），可被再次消费。
func TestPairingCredentialDBFailureRollback(t *testing.T) {
	m, db := newMgr(t)
	token, _, _ := m.CreateSession()

	// 拆掉凭据表 → 事务第 2 步失败。
	if _, err := db.Exec(`DROP TABLE device_credentials`); err != nil {
		t.Fatalf("drop: %v", err)
	}

	if _, err := m.CompleteSession(token, "HID-AAAA0000", "boot-1", "", ""); err == nil {
		t.Fatal("expected failure with dropped table")
	}

	// 回滚验证：session 应回到 pending（不是 consuming/success 半状态）。
	var status string
	if err := db.QueryRow(`SELECT status FROM pairing_sessions WHERE token=?`, token).Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "pending" {
		t.Fatalf("session status = %q after rollback, want pending (no half state)", status)
	}

	// 修复表后同 token 仍可成功消费（原子回滚的意义）。
	if _, err := db.Exec(`CREATE TABLE device_credentials (
		device_id TEXT PRIMARY KEY,
		mqtt_username TEXT NOT NULL,
		mqtt_credential_hash TEXT NOT NULL,
		issued_at INTEGER NOT NULL DEFAULT 0,
		revoked_at INTEGER,
		FOREIGN KEY (device_id) REFERENCES devices(device_id)
	)`); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if _, err := m.CompleteSession(token, "HID-AAAA0000", "boot-1", "", ""); err != nil {
		t.Fatalf("retry after rollback should succeed: %v", err)
	}
}

// 引用 sql 包以锁定接口签名（complete 未来若改用 *sql.Tx 参数可直接扩展）。
var _ = sql.ErrNoRows
