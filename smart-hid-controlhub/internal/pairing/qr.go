package pairing

import (
	"fmt"
	"net"
)

// QRPayload 生成设备可识别的配对 deep-link URL：
//
//	shid://pair?token=<token>&host=<lanHost>&port=<port>
//
// 设备侧的 ESP32 Provision Mode 解析此 URL（docs/03 BLE Provisioning Protocol）。
// host 应是 ControlHub 在 LAN 中的可达 IP（不是 127.0.0.1）。
func (m *Manager) QRPayload(token, lanHost string, pairingPort int) string {
	return fmt.Sprintf("shid://pair?token=%s&host=%s&port=%d",
		token, lanHost, pairingPort)
}

// GuessLANIP 返回本机首个非环回 IPv4 地址，用于配对 QR 的 host 字段。
// 找不到时回退到 127.0.0.1（仅本机配对场景可用）。
func GuessLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if v4 := ipnet.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return "127.0.0.1"
}
