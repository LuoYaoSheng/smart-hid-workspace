// Package trial 实现 ControlHub 的 Trial 引擎（Phase 5 核心）。
//
// 设计源：
//   - docs/05_CONTROLHUB_DETAIL_DESIGN_V1.0.md §7（Trial：基于 Device ID，不按 Command 计费）
//   - docs/10 验收清单 D1-D6
//   - docs/04 §11 / docs/05 §6 命令引擎 pipeline 顺序：
//     Entitlement 在 Publish MQTT 前完成；Trial Update 在 ACK 后
//
// 触发与计数语义：
//   - D1：安装/启动 ControlHub **不**开始倒计时（active map 启动时为空）。
//   - D2：**第一条 executed ACK** 才开始 Session（OnCommandExecuted 启动）。
//   - D3：无操作 → idle 超时 → Session 结束（active map 移除，累积量持久化）。
//   - D4：status / config / diagnostics 不触发 OnCommandExecuted → 不消耗。
//   - D5：Trial 过期后 IsControlAllowed 返 false，Engine 拒绝 control 调用，
//          但 /devices、/health、/usage 等仍可用（非 control 路径不走 Entitlement）。
//   - D6（CH-P7 实装）：machine_anchor 让 ControlHub 重装后不能"白嫖"新 Trial。
//
// 计量：累计"有效控制时间"（executed ACK 的 execution_ms 累加），不是 Command 数。
// 入库：trial_usage(device_id, machine_anchor, used_seconds, session_count, last_session_at)。
//
// CH-P6 用 stubAnchor = "local-stub"；CH-P7 替换为真实 machine GUID。
package trial

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"smart-hid-controlhub/internal/settings"
)

// stubAnchor 占位 machine anchor（CH-P6）；CH-P7 替换为真实 OS GUID。
const stubAnchor = "local-stub"

// 秒，每这么久检查一次 idle session。
const idleCheckInterval = 10 * time.Second

// activeSession 是 in-memory 的当前进行中 Session。
type activeSession struct {
	sessionID     string
	deviceID      string
	anchor        string
	startedAt     time.Time
	lastActiveAt  time.Time
	accumulatedSec float64 // 自上次 flush 以来的累计
	flushed       bool     // 是否已 flush 至少一次
}

// Manager 跟踪所有设备的 active trial session + 提供闸门与查询。
type Manager struct {
	db       trialStore  // 解耦 db 层（便于测试）
	log      *slog.Logger
	settings *settings.Store
	anchor   string // 当前机器的 anchor（CH-P7 改为 OS GUID）

	mu        sync.Mutex
	active    map[string]*activeSession // device_id → session
	closeOnce sync.Once

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// trialStore 是 Manager 依赖的最小存储接口（解耦 *sql.DB，便于测试 mock）。
type trialStore interface {
	GetUsage(deviceID, anchor string) (usedSec float64, sessionCount int, err error)
	UpsertUsage(deviceID, anchor string, addSec float64, sessionCountDelta int) error
	InsertSession(sessionID, deviceID, anchor string, startedAt int64) error
	UpdateSession(sessionID string, endedAt int64, accumulated float64) error
}

// Usage 是 GET /api/v1/usage 返回结构。
type Usage struct {
	DeviceID         string  `json:"device_id"`
	UsedSeconds      float64 `json:"used_seconds"`
	QuotaSeconds     int     `json:"quota_seconds"`
	RemainingSeconds float64 `json:"remaining_seconds"`
	SessionActive    bool    `json:"session_active"`
	SessionStartedAt *int64  `json:"session_started_at,omitempty"`
	Expired          bool    `json:"expired"`
	MachineAnchor    string  `json:"machine_anchor"`
}

// New 创建 Manager。Start() 启动 idle checker goroutine。
func New(db trialStore, setStore *settings.Store, log *slog.Logger) *Manager {
	return &Manager{
		db:       db,
		log:      log,
		settings: setStore,
		anchor:   stubAnchor,
		active:   make(map[string]*activeSession),
		stopCh:   make(chan struct{}),
	}
}

// Start 启动 idle checker goroutine。
func (m *Manager) Start() {
	m.wg.Add(1)
	go m.idleLoop()
}

// Close 停止 goroutine 并 flush 所有 active session。幂等。
func (m *Manager) Close() {
	m.closeOnce.Do(func() { close(m.stopCh) })
	m.wg.Wait()
	m.Flush()
}

// --- Entitlement 接口（被 Engine.Send 调用） ---

// IsControlAllowed 在 Engine.Send 入队前调用。
// 返回 false 表示 Trial 已耗尽，应拒绝 control 请求（D5: 配置/查询仍可用）。
func (m *Manager) IsControlAllowed(deviceID string) bool {
	quota := m.settings.GetInt(settings.KeyTrialQuotaSeconds, 1800)
	m.mu.Lock()
	defer m.mu.Unlock()
	used, _, err := m.db.GetUsage(deviceID, m.anchor)
	if err != nil {
		m.log.Warn("trial IsControlAllowed get usage failed", "device_id", deviceID, "err", err)
		return true // 数据库错误时降级放行（避免误拒）
	}
	// 加上当前 active session 的累积
	if sess, ok := m.active[deviceID]; ok {
		used += sess.accumulatedSec
	}
	remaining := float64(quota) - used
	return remaining > 0
}

// --- Trial Hook 接口（被 Engine.HandleAck 调用） ---

// OnCommandExecuted 在 ACK status=executed 时调用。execMs 是执行毫秒。
// 累加到当前 session；若无 active session 则启动一个。
func (m *Manager) OnCommandExecuted(deviceID string, execMs int) {
	if execMs < 0 {
		execMs = 0
	}
	addSec := float64(execMs) / 1000.0

	m.mu.Lock()
	sess, exists := m.active[deviceID]
	if !exists {
		sess = &activeSession{
			sessionID:     genSessionID(),
			deviceID:      deviceID,
			anchor:        m.anchor,
			startedAt:     time.Now(),
			lastActiveAt:  time.Now(),
			accumulatedSec: 0,
		}
		m.active[deviceID] = sess
		// 持久化 session 开端（best-effort）
		if err := m.db.InsertSession(sess.sessionID, deviceID, m.anchor, sess.startedAt.Unix()); err != nil {
			m.log.Warn("trial insert session", "device_id", deviceID, "err", err)
			// 失败不致命，in-memory 仍跟踪
		} else {
			m.log.Info("trial session started", "device_id", deviceID, "session_id", sess.sessionID)
		}
	}
	sess.accumulatedSec += addSec
	sess.lastActiveAt = time.Now()
	m.mu.Unlock()
}

// --- 查询接口 ---

// Usage 返回当前用量（含 active session 的实时累计）。
func (m *Manager) Usage(deviceID string) Usage {
	quota := m.settings.GetInt(settings.KeyTrialQuotaSeconds, 1800)
	m.mu.Lock()
	defer m.mu.Unlock()

	used, _, _ := m.db.GetUsage(deviceID, m.anchor)
	sess, isActive := m.active[deviceID]
	if isActive {
		used += sess.accumulatedSec
	}
	remaining := float64(quota) - used
	out := Usage{
		DeviceID:         deviceID,
		UsedSeconds:      used,
		QuotaSeconds:     quota,
		RemainingSeconds: remaining,
		SessionActive:    isActive,
		Expired:          remaining <= 0,
		MachineAnchor:    m.anchor,
	}
	if isActive {
		ts := sess.startedAt.Unix()
		out.SessionStartedAt = &ts
	}
	return out
}

// Flush 把所有 active session 的累计量持久化到 trial_usage 表。
// 调用时机：idle 超时 / 程序退出。
func (m *Manager) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for deviceID, sess := range m.active {
		if sess.accumulatedSec <= 0 {
			continue
		}
		if err := m.db.UpsertUsage(deviceID, sess.anchor, sess.accumulatedSec, 0); err != nil {
			m.log.Warn("trial flush upsert", "device_id", deviceID, "err", err)
			continue
		}
		// 更新 session 行的 ended_at + accumulated
		_ = m.db.UpdateSession(sess.sessionID, time.Now().Unix(), sess.accumulatedSec)
		sess.accumulatedSec = 0
		sess.flushed = true
	}
}

// idleLoop 周期性检查 idle session，超时则 endSession（持久化 + 移除）。
func (m *Manager) idleLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.expireIdleSessions()
		}
	}
}

func (m *Manager) expireIdleSessions() {
	idleTimeout := time.Duration(m.settings.GetInt(settings.KeyTrialIdleTimeoutSeconds, 300)) * time.Second
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for deviceID, sess := range m.active {
		if now.Sub(sess.lastActiveAt) < idleTimeout {
			continue
		}
		// idle 超时：flush + 移除
		if sess.accumulatedSec > 0 {
			if err := m.db.UpsertUsage(deviceID, sess.anchor, sess.accumulatedSec, 1); err != nil {
				m.log.Warn("trial idle flush", "device_id", deviceID, "err", err)
			} else {
				_ = m.db.UpdateSession(sess.sessionID, now.Unix(), sess.accumulatedSec)
				m.log.Info("trial session idle-ended",
					"device_id", deviceID,
					"session_id", sess.sessionID,
					"accumulated_sec", sess.accumulatedSec)
			}
		}
		delete(m.active, deviceID)
	}
}

func genSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "ts_" + hex.EncodeToString(b)
}

// 引用 fmt 避免 unused（如果未来加日志格式化）
var _ = fmt.Sprintf
