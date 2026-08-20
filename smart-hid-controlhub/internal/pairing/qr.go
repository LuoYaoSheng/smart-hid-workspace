package pairing

import "fmt"

// QRPayload 生成设备可识别的配对 deep-link URL：
//
//	shid://pair?token=<token>&host=<lanHost>&port=<port>
//
// 设备侧的 ESP32 Provision Mode 解析此 URL（protocols/ble/PROVISIONING_V1.md）。
// host 由调用方经 netaddr.Resolver 按请求路径解析（M1-G3 取代旧的
// GuessLANIP 首网卡猜测——多网卡/Docker/Tailscale 场景不再选错）。
func (m *Manager) QRPayload(token, lanHost string, pairingPort int) string {
	return fmt.Sprintf("shid://pair?token=%s&host=%s&port=%d",
		token, lanHost, pairingPort)
}
