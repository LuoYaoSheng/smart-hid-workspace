// Package sys: machine_anchor 跨平台读取（CH-P7）。
//
// 用途：trial_usage 主键 = (device_id, machine_anchor)。
// 重装 ControlHub（保留 OS）→ anchor 不变 → 用量保留 → 防绕过（验收 D6）。
// 重装 OS / 换机 → anchor 变化 → Phase 6 Cloud 设备绑定接管。
//
// 实现：
//   - darwin:  ioreg 读 IOPlatformUUID（已 macOS 验证）
//   - linux:   /etc/machine-id 或 /var/lib/dbus/machine-id
//   - windows: reg query HKLM\SOFTWARE\Microsoft\Cryptography MachineGuid
//
// 任一失败时返回 "fallback-<GOOS>"，trial 仍工作（仅失去重装防绕过能力）。
package sys

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// GetMachineAnchor 返回当前机器的稳定标识符。
// 总是非空；失败时返回 fallback 字符串。
func GetMachineAnchor() string {
	switch runtime.GOOS {
	case "darwin":
		return firstNonEmpty(darwinUUID(), "fallback-darwin")
	case "linux":
		return firstNonEmpty(linuxMachineID(), "fallback-linux")
	case "windows":
		return firstNonEmpty(windowsMachineGUID(), "fallback-windows")
	}
	return "fallback-" + runtime.GOOS
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// darwinUUID 从 ioreg 输出提取 IOPlatformUUID。
func darwinUUID() string {
	out, err := exec.Command("ioreg", "-d2", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "IOPlatformUUID") {
			continue
		}
		// 行形如:     "IOPlatformUUID" = "DEAD-BEEF-..."
		parts := strings.Split(line, "\"")
		if len(parts) >= 4 {
			return strings.TrimSpace(parts[3])
		}
	}
	return ""
}

// linuxMachineID 读 /etc/machine-id 或 /var/lib/dbus/machine-id。
func linuxMachineID() string {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(b))
		if s != "" {
			return s
		}
	}
	return ""
}

// windowsMachineGUID 用 reg query 读 MachineGuid（无需 syscall/注册表库依赖）。
// 注意：reg.exe 在 PATH 中（C:\Windows\System32\reg.exe）。
func windowsMachineGUID() string {
	out, err := exec.Command("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Cryptography`,
		"/v", "MachineGuid").Output()
	if err != nil {
		return ""
	}
	// 输出末行: "    MachineGuid    REG_SZ    <guid>"
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "MachineGuid") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			return fields[len(fields)-1]
		}
	}
	return ""
}
