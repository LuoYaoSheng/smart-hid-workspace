// realtime_concurrency_test.go — M1-G2 RealtimeHub 并发安全压力测试。
// 无 sleep：全部由 WaitGroup / channel 驱动，可重复。
package api

import (
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
)

func stressHub() *RealtimeHub {
	return NewRealtimeHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestRealtimeHub_ConcurrentStress —— 并发 subscribe / unsubscribe / broadcast
// 循环数千次：-race 下无 map 读写竞争、无 panic、无 send on closed channel。
func TestRealtimeHub_ConcurrentStress(t *testing.T) {
	hub := stressHub()
	const rounds = 3000

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 3 个广播者：持续广播
	for b := 0; b < 3; b++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				hub.Broadcast("stress", map[string]any{"i": i})
			}
		}()
	}
	// 4 个订阅者生命周期搅动者：反复 subscribe→（少量收/不收）→unsubscribe
	for s := 0; s < 4; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				ch := hub.subscribe()
				select { // 消费 0~1 条，制造慢/快消费者混合
				case <-ch:
				default:
				}
				hub.unsubscribe(ch)
			}
		}()
	}
	// 2 个长驻订阅者：只收不清（验证缓冲不被外部 close）
	living := make([]chan []byte, 2)
	for l := range living {
		living[l] = hub.subscribe()
	}
	var received atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, ch := range living {
			go func(ch chan []byte) {
				for {
					select {
					case <-ch:
						received.Add(1)
					case <-stop:
						return
					}
				}
			}(ch)
		}
	}()

	wg.Wait()
	close(stop)

	// 终态一致性：unsubscribe 后订阅者集合为空（living 也退订）
	for _, ch := range living {
		hub.unsubscribe(ch)
	}
	hub.mu.RLock()
	n := len(hub.subs)
	hub.mu.RUnlock()
	if n != 0 {
		t.Fatalf("subs after churn = %d, want 0", n)
	}
	if received.Load() == 0 {
		t.Fatal("long-lived subscribers received nothing")
	}
}

// TestRealtimeHub_BroadcastZeroSubscribers —— 零订阅广播为 no-op（锁内快照路径）。
func TestRealtimeHub_BroadcastZeroSubscribers(t *testing.T) {
	hub := stressHub()
	hub.Broadcast("x", "y") // 不应 panic
	hub.mu.RLock()
	n := len(hub.subs)
	hub.mu.RUnlock()
	if n != 0 {
		t.Fatalf("subs = %d, want 0", n)
	}
}

// TestRealtimeHub_SlowConsumerDrop —— 缓冲满时广播不阻塞（丢弃而非卡死）。
func TestRealtimeHub_SlowConsumerDrop(t *testing.T) {
	hub := stressHub()
	ch := hub.subscribe()
	defer hub.unsubscribe(ch)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ { // 缓冲 64，绝大多数被丢
			hub.Broadcast("flood", i)
		}
	}()
	<-done // 若广播阻塞慢消费者，这里会挂起并由 test timeout 暴露
	select {
	case <-ch:
	default:
		t.Fatal("at least one event should land in buffer")
	}
}
