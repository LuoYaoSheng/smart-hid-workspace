package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"smart-hid-controlhub/internal/apikey"
	"smart-hid-controlhub/internal/storage"
)

func newRealtimeServer(t *testing.T, key string) *httptest.Server {
	t.Helper()
	store, err := storage.New(filepath.Join(t.TempDir(), "rt.db"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	keys := apikey.New(store.DB, slog.Default())
	if key != "" {
		if err := keys.InsertTesting(key, "test"); err != nil {
			t.Fatal(err)
		}
	}
	srv := New(nil, nil, keys, nil, nil, slog.Default()).
		WithRealtimeHub(NewRealtimeHub(slog.Default()))
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts
}

func wsDial(t *testing.T, url, key string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(url, "http") + "/api/v1/realtime"
	if key != "" {
		wsURL += "?key=" + key
	}
	return websocket.DefaultDialer.Dial(wsURL, nil)
}

func TestRealtime_Auth(t *testing.T) {
	ts := newRealtimeServer(t, testAPIKey)

	// 无 key / 错 key：HTTP 拒绝（升级前）
	if _, res, err := wsDial(t, ts.URL, ""); err == nil {
		t.Fatal("no-key dial should fail")
	} else if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-key status = %d, want 401", res.StatusCode)
	}
	if _, res, err := wsDial(t, ts.URL, "chk_wrong"); err == nil {
		t.Fatal("wrong-key dial should fail")
	} else if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong-key status = %d, want 401", res.StatusCode)
	}

	// 对 key：升级成功 + 收到 hello
	conn, _, err := wsDial(t, ts.URL, testAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var ev struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(msg, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != "hello" {
		t.Fatalf("first event = %q, want hello", ev.Type)
	}
}

func TestRealtime_HubBroadcast(t *testing.T) {
	hub := NewRealtimeHub(slog.Default())

	// 零订阅：no-op 不 panic
	hub.Broadcast("ack", map[string]string{"x": "1"})

	ch := hub.subscribe()
	hub.Broadcast("ack", map[string]any{"request_id": "r1", "status": "executed"})
	select {
	case msg := <-ch:
		var ev struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(msg, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Type != "ack" || ev.Data["request_id"] != "r1" {
			t.Fatalf("event = %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast received")
	}

	// 慢消费者（不读）：Broadcast 不阻塞
	stuck := hub.subscribe()
	done := make(chan struct{})
	go func() { hub.Broadcast("device", map[string]string{"d": "1"}); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Broadcast blocked on slow subscriber")
	}
	_ = stuck

	hub.unsubscribe(ch)
	hub.unsubscribe(stuck)
}
