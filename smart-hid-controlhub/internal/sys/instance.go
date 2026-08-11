// Package sys 提供跨平台系统级辅助。
//
// CH-P2：单实例锁（bind lockport）。
// CH-P7：machine_anchor 读取（Windows MachineGuid / macOS IOPlatformUUID / Linux machine-id）。
package sys

import (
	"fmt"
	"net"
)

// SingleInstance 通过 bind 一个 TCP lockport 实现进程唯一。
// 第二个实例启动时 bind 失败 → 返回 error，调用者应退出。
//
// 简化选择：相比文件锁 + PID 检查的方案，TCP lockport 同时还能用作"激活已有实例"
// 的 IPC 通道（CH-P3 Tray 接入时可在此 listener 上收 "raise window" 命令）。
type SingleInstance struct {
	ln net.Listener
}

// AcquireSingleInstance 尝试 bind 127.0.0.1:lockPort。
// 若已被占用（另一实例在跑）→ 返回 error。
// 返回的 SingleInstance 必须由调用者持有到进程退出（Close 释放锁）。
func AcquireSingleInstance(lockPort int) (*SingleInstance, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", lockPort))
	if err != nil {
		return nil, fmt.Errorf(
			"single-instance lock failed on 127.0.0.1:%d (another ControlHub instance running?): %w",
			lockPort, err,
		)
	}
	return &SingleInstance{ln: ln}, nil
}

// Close 释放锁。进程退出前调用。
func (s *SingleInstance) Close() error {
	if s == nil || s.ln == nil {
		return nil
	}
	return s.ln.Close()
}
