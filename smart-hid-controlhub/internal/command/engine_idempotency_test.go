// engine_idempotency_test.go — M1-G2 request_id 幂等与并发安全回归测试。
// 全部事件驱动（barrier/channel），不依赖 sleep 猜并发时序。
package command

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"smart-hid-controlhub/internal/storage"
)

// TestConcurrentSameRequestIDSamePayload —— 同 request_id 同 payload 并发：
// 恰好 publish 一次，所有 waiter 获得同一终态结果，无泄漏。
func TestConcurrentSameRequestIDSamePayload(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	published := make(chan struct{}, 1)
	client.publishFn = func(topic string, qos byte, retained bool, payload any) pahomqtt.Token {
		select {
		case published <- struct{}{}:
		default:
		}
		go func() {
			<-published // 确保仅首次 publish 触发一次 ACK（防止重复投递）
			eng.HandleAck(nil, &mockMessage{
				topic:   AckTopic("HID-ABCD1234"),
				payload: ackJSON("req-conc", "HID-ABCD1234", AckExecuted, 0, 11),
			})
		}()
		return instantToken()
	}

	const n = 20
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]*SmartHidAck, n)
	terminals := make([]bool, n)
	var serrsMu sync.Mutex
	var serrCount int

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := validCmd()
			cmd.RequestID = "req-conc"
			<-start
			ack, terminal, serr := eng.Send(context.Background(), cmd)
			results[i], terminals[i] = ack, terminal
			if serr != nil {
				serrsMu.Lock()
				serrCount++
				serrsMu.Unlock()
			}
		}(i)
	}
	close(start)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent senders did not finish within 5s (deadlock?)")
	}

	if serrCount != 0 {
		t.Fatalf("%d senders returned error, expected 0", serrCount)
	}
	if got := len(client.publishCalls); got != 1 {
		t.Fatalf("expected exactly 1 publish, got %d", got)
	}
	for i := 0; i < n; i++ {
		if !terminals[i] {
			t.Fatalf("sender %d: expected terminal=true", i)
		}
		if results[i] == nil || results[i].Status != AckExecuted || results[i].ExecutionMs != 11 {
			t.Fatalf("sender %d: expected executed ack exec_ms=11, got %+v", i, results[i])
		}
	}

	// 无在途泄漏
	eng.mu.Lock()
	_, leak := eng.execs["req-conc"]
	eng.mu.Unlock()
	if leak {
		t.Error("in-flight execution leaked after all senders returned")
	}
}

// TestConcurrentSameRequestIDDifferentPayload —— 同 request_id 不同 payload 并发：
// 恰好一个 owner 执行，其余全部 request_id_conflict。
func TestConcurrentSameRequestIDDifferentPayload(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))
	client.publishFn = func(string, byte, bool, any) pahomqtt.Token { return instantToken() }

	const n = 20
	start := make(chan struct{})
	var wg sync.WaitGroup
	var conflicts int32
	var owners int32

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := validCmd()
			cmd.RequestID = "req-conflict"
			cmd.TTLMs = TTLMsMin // 100ms，owner 很快过期，冲突者走 registry 或 DB 任一路径
			// 每个请求 payload 不同（key 不同）
			cmd.Payload = KeyboardPayload{Key: string(rune('a' + i%26)), HoldMs: 40}
			<-start
			_, _, serr := eng.Send(context.Background(), cmd)
			if serr == nil {
				atomic.AddInt32(&owners, 1)
			} else if serr.Kind == ErrKindConflict {
				atomic.AddInt32(&conflicts, 1)
			} else {
				t.Errorf("sender %d: unexpected error kind %q (%v)", i, serr.Kind, serr)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if owners != 1 {
		t.Fatalf("expected exactly 1 owner, got %d", owners)
	}
	if conflicts != n-1 {
		t.Fatalf("expected %d conflicts, got %d", n-1, conflicts)
	}
	if got := len(client.publishCalls); got != 1 {
		t.Fatalf("expected exactly 1 publish (owner only), got %d", got)
	}
}

// TestTerminalRequestReplay —— 终态后同 request_id 同 payload 重放：
// 不再 publish，直接返回既有终态结果。
func TestTerminalRequestReplay(t *testing.T) {
	client := &mockClient{}
	eng, dm, _ := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	client.publishFn = func(string, byte, bool, any) pahomqtt.Token {
		go eng.HandleAck(nil, &mockMessage{
			topic:   AckTopic("HID-ABCD1234"),
			payload: ackJSON("req-replay", "HID-ABCD1234", AckExecuted, 0, 42),
		})
		return instantToken()
	}

	cmd := validCmd()
	cmd.RequestID = "req-replay"
	ack, terminal, serr := eng.Send(context.Background(), cmd)
	if serr != nil || !terminal || ack == nil || ack.Status != AckExecuted || ack.ExecutionMs != 42 {
		t.Fatalf("first send: ack=%+v terminal=%v serr=%v", ack, terminal, serr)
	}
	if got := len(client.publishCalls); got != 1 {
		t.Fatalf("first send should publish once, got %d", got)
	}

	// 第二次：同 request_id 同 payload → 终态重放，零 publish。
	ack2, terminal2, serr2 := eng.Send(context.Background(), cmd)
	if serr2 != nil {
		t.Fatalf("replay should not error: %v", serr2)
	}
	if !terminal2 || ack2 == nil || ack2.Status != AckExecuted || ack2.ExecutionMs != 42 {
		t.Fatalf("replay should return recorded terminal result, got ack=%+v terminal=%v", ack2, terminal2)
	}
	if got := len(client.publishCalls); got != 1 {
		t.Fatalf("replay must not re-publish, got %d publishes", got)
	}
}

// TestReplayLegacyEmptyFingerprint —— 升级前旧行 fingerprint=”：
// 不判冲突，按既有终态回放。
func TestReplayLegacyEmptyFingerprint(t *testing.T) {
	client := &mockClient{}
	eng, dm, store := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))
	seedCommandRow(t, store, "req-legacy", "HID-ABCD1234", string(AckExecuted)) // fingerprint=''

	cmd := validCmd()
	cmd.RequestID = "req-legacy"
	ack, terminal, serr := eng.Send(context.Background(), cmd)
	if serr != nil {
		t.Fatalf("legacy replay should not conflict: %v", serr)
	}
	if !terminal || ack == nil || ack.Status != AckExecuted {
		t.Fatalf("legacy replay should return executed, got %+v terminal=%v", ack, terminal)
	}
	if got := len(client.publishCalls); got != 0 {
		t.Fatalf("legacy replay must not publish, got %d", got)
	}
}

// TestReplayNonTerminalAsAccepted —— DB 行非终态（已 publish 未 ACK）：
// 不重新执行，按 202 语义返回。
func TestReplayNonTerminalAsAccepted(t *testing.T) {
	client := &mockClient{}
	eng, dm, store := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	cmd := validCmd()
	cmd.RequestID = "req-nonterm"
	seedCommandRowFP(t, store, "req-nonterm", "HID-ABCD1234", string(AckReceived), Fingerprint(cmd))

	ack, terminal, serr := eng.Send(context.Background(), cmd)
	if serr != nil {
		t.Fatalf("non-terminal replay should not error: %v", serr)
	}
	if terminal || ack != nil {
		t.Fatalf("non-terminal replay should be 202-style (nil,false), got ack=%+v terminal=%v", ack, terminal)
	}
	if got := len(client.publishCalls); got != 0 {
		t.Fatalf("non-terminal replay must not re-publish, got %d", got)
	}
}

// TestReplayDifferentFingerprintConflict —— DB 行指纹不同 → 409 冲突。
func TestReplayDifferentFingerprintConflict(t *testing.T) {
	client := &mockClient{}
	eng, dm, store := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	seedCommandRowFP(t, store, "req-diff", "HID-ABCD1234", string(AckExecuted), "deadbeef")

	cmd := validCmd()
	cmd.RequestID = "req-diff"
	_, _, serr := eng.Send(context.Background(), cmd)
	if serr == nil || serr.Kind != ErrKindConflict {
		t.Fatalf("expected request_id_conflict, got %+v", serr)
	}
	if got := len(client.publishCalls); got != 0 {
		t.Fatalf("conflicting replay must not publish, got %d", got)
	}
}

// TestRequestIDDatabaseInsertFailure —— DB 写入失败：
// 不得注册在途、不得 publish、返回 internal 错误、无泄漏。
func TestRequestIDDatabaseInsertFailure(t *testing.T) {
	client := &mockClient{}
	eng, dm, store := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))

	// 设备注册后拆掉 commands 表 → INSERT 必然失败（非 UNIQUE）。
	if _, err := store.DB.Exec(`DROP TABLE commands`); err != nil {
		t.Fatalf("drop commands table: %v", err)
	}

	cmd := validCmd()
	cmd.RequestID = "req-dbfail"
	_, _, serr := eng.Send(context.Background(), cmd)
	if serr == nil || serr.Kind != ErrKindInternal {
		t.Fatalf("expected internal error, got %+v", serr)
	}
	if got := len(client.publishCalls); got != 0 {
		t.Fatalf("DB failure must not publish, got %d", got)
	}
	eng.mu.Lock()
	_, leak := eng.execs["req-dbfail"]
	eng.mu.Unlock()
	if leak {
		t.Error("in-flight execution leaked after DB failure")
	}
}

// TestAckWrongDeviceIgnored —— A 设备在途请求收到 B 设备 ACK（topic/载荷均 B）：
// 不得认领；A 的正确 ACK 到达后才结束。
func TestAckWrongDeviceIgnored(t *testing.T) {
	client := &mockClient{}
	eng, dm, store := newTestEngine(t, client)
	dm.UpsertStatus(readyStatus("HID-ABCD1234"))
	dm.UpsertStatus(readyStatus("HID-EEEE0000"))

	published := make(chan struct{}, 1)
	client.publishFn = func(string, byte, bool, any) pahomqtt.Token {
		published <- struct{}{}
		return instantToken()
	}

	cmd := validCmd()
	cmd.RequestID = "req-hijack"
	cmd.TTLMs = 3000
	sendDone := make(chan struct{})
	var ack *SmartHidAck
	var terminal bool
	go func() {
		defer close(sendDone)
		ack, terminal, _ = eng.Send(context.Background(), cmd)
	}()

	<-published
	// B 设备试图用同 request_id 认领。
	eng.HandleAck(nil, &mockMessage{
		topic:   AckTopic("HID-EEEE0000"),
		payload: ackJSON("req-hijack", "HID-EEEE0000", AckExecuted, 0, 99),
	})
	select {
	case <-sendDone:
		t.Fatal("wrong-device ack must NOT complete device-A waiter")
	default:
	}
	// DB 也不应被 B 的 ACK 污染。
	var st string
	_ = store.DB.QueryRow(`SELECT status FROM commands WHERE request_id=?`, "req-hijack").Scan(&st)
	if st != string(AckReceived) {
		t.Fatalf("wrong-device ack must not touch DB, status=%q", st)
	}

	// A 的正确 ACK 到达 → 结束。
	eng.HandleAck(nil, &mockMessage{
		topic:   AckTopic("HID-ABCD1234"),
		payload: ackJSON("req-hijack", "HID-ABCD1234", AckExecuted, 0, 5),
	})
	<-sendDone
	if !terminal || ack == nil || ack.Status != AckExecuted || ack.DeviceID != "HID-ABCD1234" {
		t.Fatalf("expected device-A executed ack, got %+v terminal=%v", ack, terminal)
	}
}

// TestAckTopicPayloadMismatchDropped —— topic 设备与 payload device_id 不一致：
// 直接丢弃。
func TestAckTopicPayloadMismatchDropped(t *testing.T) {
	eng, _, _ := newTestEngine(t, nil)
	ex := &execution{fingerprint: "fp", deviceID: "HID-ABCD1234", done: make(chan struct{})}
	eng.mu.Lock()
	eng.execs["req-mm"] = ex
	eng.mu.Unlock()

	// topic=A payload=B
	eng.HandleAck(nil, &mockMessage{
		topic:   AckTopic("HID-ABCD1234"),
		payload: ackJSON("req-mm", "HID-EEEE0000", AckExecuted, 0, 1),
	})
	select {
	case <-ex.done:
		t.Fatal("mismatched ack must be dropped")
	default:
	}
}

// TestAckDuplicateTerminalSafe —— 终态后重复 ACK：安全处理（幂等落库，无 panic）。
func TestAckDuplicateTerminalSafe(t *testing.T) {
	eng, _, store := newTestEngine(t, nil)
	seedCommandRow(t, store, "req-dup", "HID-ABCD1234", string(AckReceived))

	for i := 0; i < 3; i++ {
		eng.HandleAck(nil, &mockMessage{
			topic:   AckTopic("HID-ABCD1234"),
			payload: ackJSON("req-dup", "HID-ABCD1234", AckExecuted, 0, 6),
		})
	}
	var st string
	if err := store.DB.QueryRow(`SELECT status FROM commands WHERE request_id=?`, "req-dup").Scan(&st); err != nil {
		t.Fatalf("query: %v", err)
	}
	if st != string(AckExecuted) {
		t.Fatalf("DB status=%q, want executed", st)
	}
}

// TestFingerprintCanonical —— 键序无关：不同 JSON 键序的等价 payload 指纹一致。
func TestFingerprintCanonical(t *testing.T) {
	base := &SmartHidCommand{
		Protocol: "1.0", RequestID: "r", DeviceID: "HID-ABCD1234", TargetBootID: "b",
		Type: TypeMouse, Action: "move", TTLMs: 1000,
		Payload: map[string]any{"dx": float64(10), "dy": float64(-5)}, // map 序由 marshal 排序
	}
	same := &SmartHidCommand{
		Protocol: "1.0", RequestID: "r", DeviceID: "HID-ABCD1234", TargetBootID: "b",
		Type: TypeMouse, Action: "move", TTLMs: 1000,
		Payload: map[string]any{"dy": float64(-5), "dx": float64(10)}, // 键序不同
	}
	diff := &SmartHidCommand{
		Protocol: "1.0", RequestID: "r", DeviceID: "HID-ABCD1234", TargetBootID: "b",
		Type: TypeMouse, Action: "move", TTLMs: 1000,
		Payload: map[string]any{"dx": float64(11), "dy": float64(-5)},
	}
	if Fingerprint(base) != Fingerprint(same) {
		t.Fatal("equivalent payloads with different key order must share fingerprint")
	}
	if Fingerprint(base) == Fingerprint(diff) {
		t.Fatal("different payload values must differ in fingerprint")
	}
}

// seedCommandRowFP 同 seedCommandRow，但带指纹。
func seedCommandRowFP(t *testing.T, store *storage.Store, reqID, devID, status, fp string) {
	t.Helper()
	seedCommandRow(t, store, reqID, devID, status)
	if _, err := store.DB.Exec(`UPDATE commands SET fingerprint=? WHERE request_id=?`, fp, reqID); err != nil {
		t.Fatalf("seed fingerprint: %v", err)
	}
}
