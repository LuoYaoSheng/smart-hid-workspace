package trial

// Anchor 返回当前 ControlHub 实例的 machine anchor 字符串。
//
// CH-P6 stub：固定返回 "local-stub"。
// CH-P7 实装：
//   - Windows: HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid
//   - macOS:   IOPlatformUUID（ioreg -d2 -c IOPlatformExpertDevice）
//   - Linux:   /etc/machine-id
//
// 作用：trial_usage 主键 = (device_id, machine_anchor)。
// 重装 ControlHub（保留 OS）→ machine_anchor 不变 → 用量保留 → 防绕过（D6）。
// 重装 OS / 换机 → machine_anchor 变化 → Phase 6 Cloud 设备绑定接管。
func (m *Manager) Anchor() string {
	return m.anchor
}
