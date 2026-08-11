// Package protocol 存放 ControlHub 私有侧的协议消息类型，
// 供多个内部包共享（device、command、api 等）。
//
// 这些类型镜像自事实源 smart-ble/core/protocols/hid-command-schema.ts。
// 之所以从 command 包拆出，是为了打破 device ↔ command 的 import 循环
// （device 需要 SmartHidStatus，command 需要 device.Manager）。
package protocol

// ProtocolVersion 协议版本（对应 TS COMMAND_PROTOCOL_VERSION）。
const ProtocolVersion = "1.0"

// SmartHidStatus 是设备状态消息。
// 注意：TS 与 JSON Schema 在此类型有分歧（TS 要求 usb_hid_ready+timestamp，
// JSON Schema 仅要 protocol/device_id/online）。Go 侧按 TS 主体 + 放宽额外字段。
type SmartHidStatus struct {
	Protocol    string `json:"protocol"`
	DeviceID    string `json:"device_id"`
	Online      bool   `json:"online"`
	BootID      string `json:"boot_id"`
	USBHIDReady bool   `json:"usb_hid_ready"`
	Firmware    string `json:"firmware,omitempty"`
	Timestamp   int64  `json:"timestamp"` // Unix 秒
}
