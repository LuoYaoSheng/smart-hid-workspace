// Package entitle 实现 ControlHub 的统一 Entitlement 闸门（CL-3c）。
//
// 实现 command.Entitlement interface，按优先级检查：
//  1. License（licmgr.IsEffective）—— 有效 license 优先放行
//  2. Trial（trial.IsControlAllowed）—— 无 license 时回退到 trial 配额
//  3. 都无 → 拒绝（Engine 返 402）
//
// 设计源：docs/04 §11（Entitlement 在 Publish MQTT 前完成）+
// docs/05 §6（命令引擎 pipeline）+ docs/10 §D5/E（Trial/License 互斥优先级）。
package entitle

import (
	"log/slog"

	"smart-hid-controlhub/internal/license"
	"smart-hid-controlhub/internal/trial"
)

// Gate 是 Entitlement 闸门，组合 license + trial。
type Gate struct {
	license *licmgr.Manager
	trial   *trial.Manager
	log     *slog.Logger
}

// New 创建 Gate。任一参数可为 nil（对应模块未启用）。
func New(l *licmgr.Manager, t *trial.Manager, log *slog.Logger) *Gate {
	return &Gate{license: l, trial: t, log: log}
}

// IsControlAllowed 实现 command.Entitlement interface。
// 优先 License，回退 Trial，都无则拒绝。
func (g *Gate) IsControlAllowed(deviceID string) bool {
	// 1. License 优先
	if g.license != nil && g.license.IsEffective(deviceID) {
		return true
	}
	// 2. Trial 回退
	if g.trial != nil {
		return g.trial.IsControlAllowed(deviceID)
	}
	return false
}

// Reason 返回决策原因（管理/调试用，非 interface 要求）。
type Decision int

const (
	DecisionNone Decision = iota
	DecisionLicense
	DecisionTrial
	DecisionDenied
)

// DecisionFor 返回 deviceID 的决策结果 + 是否允许。
func (g *Gate) DecisionFor(deviceID string) (Decision, bool) {
	if g.license != nil && g.license.IsEffective(deviceID) {
		return DecisionLicense, true
	}
	if g.trial != nil && g.trial.IsControlAllowed(deviceID) {
		return DecisionTrial, true
	}
	return DecisionDenied, false
}
