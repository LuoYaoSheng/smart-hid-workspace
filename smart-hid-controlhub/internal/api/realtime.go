// realtime.go — WebSocket 实时事件通道（OS-3b）。
//
// 端点：GET /api/v1/realtime?key=<api-key>
// 浏览器 WebSocket 无法携带 Authorization header，鉴权用 query 参数
// （LAN 场景可接受；键不出现在服务端日志——本 handler 不打印 query）。
//
// 下行事件（JSON）：
//
//	{"type":"hello","data":{"devices_total":N}}
//	{"type":"device","data":{device_id,boot_id,online,usb_hid_ready,firmware}}
//	{"type":"ack","data":{request_id,device_id,status,code,execution_ms}}
//
// 只下行：控制命令仍走 HTTP 闭环（调用方需要同步 ACK）。
// 保活：服务端 30s ping；客户端 pong 由 gorilla 自动回。
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  512,
	WriteBufferSize: 2048,
	// 同源由 API key 把关；LAN 内多端访问 origin 不定。
	CheckOrigin: func(*http.Request) bool { return true },
}

// RealtimeHub 维护 WS 订阅者并广播事件。零订阅时 Broadcast 为 no-op。
//
// 并发安全（M1-G2）：subs 由 RWMutex 保护；Broadcast 持读锁快照订阅者后
// 放锁再发送——不长时间持锁做可能阻塞的 channel send，也不在锁外读 map。
// 订阅 channel 只由 subscribe 创建、只从 subs map 移除、永不 close：
// 不存在 send on closed channel 路径。
type RealtimeHub struct {
	log  *slog.Logger
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
}

func NewRealtimeHub(log *slog.Logger) *RealtimeHub {
	return &RealtimeHub{log: log, subs: make(map[chan []byte]struct{})}
}

// Broadcast 向所有订阅者推送 type/data 事件（慢消费者丢弃该条，不阻塞广播方）。
func (h *RealtimeHub) Broadcast(eventType string, data any) {
	if h == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{"type": eventType, "data": data, "ts": time.Now().UnixMilli()})
	if err != nil {
		h.log.Warn("realtime marshal failed", "err", err)
		return
	}
	// 快照订阅者集合 → 放锁 → 向快照发送（发送期间订阅者增删不影响一致性）。
	h.mu.RLock()
	snapshot := make([]chan []byte, 0, len(h.subs))
	for ch := range h.subs {
		snapshot = append(snapshot, ch)
	}
	h.mu.RUnlock()
	if len(snapshot) == 0 {
		return
	}
	for _, ch := range snapshot {
		select {
		case ch <- payload:
		default: // 慢消费者：丢事件保广播不阻塞
		}
	}
}

func (h *RealtimeHub) subscribe() chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *RealtimeHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	// 注意：不 close(ch)——Broadcast 可能持有快照仍在发送；残留消息由 GC 回收。
}

// handleRealtime 升级 WS 并开始推送（注册于 /api/v1/realtime，需 web.realtime=true）。
func (s *Server) handleRealtime(w http.ResponseWriter, r *http.Request) {
	if s.realtimeHub == nil {
		http.NotFound(w, r)
		return
	}
	// 鉴权：query key（浏览器 WS 限制，见文件头说明）
	key := r.URL.Query().Get("key")
	if key == "" || !s.keys.Verify(key) {
		writeJSON(w, http.StatusUnauthorized, errBody{"unauthorized", "missing or invalid key"})
		return
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade 已写错误响应
	}
	ch := s.realtimeHub.subscribe()
	defer func() {
		s.realtimeHub.unsubscribe(ch)
		_ = conn.Close()
	}()

	// 读循环：丢弃客户端一切上行（协议只下行），同时驱动 close/pong 检测
	go func() {
		conn.SetReadLimit(512)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// hello
	if hello, err := json.Marshal(map[string]any{
		"type": "hello",
		"data": map[string]any{"server": "smart-hid-controlhub", "protocol": "1.0"},
	}); err == nil {
		_ = conn.WriteMessage(websocket.TextMessage, hello)
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
