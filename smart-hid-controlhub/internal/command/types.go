// Package command 实现 Command Engine：HTTP → MQTT → ACK 闭环。
//
// 类型定义镜像自事实源 smart-ble/core/protocols/hid-command-schema.ts。
// 任何字段变更必须先改 TS，再同步本文件。二者须保持一致。
package command

// ProtocolVersion 是 SmartHidCommand envelope 的协议版本（对应 TS COMMAND_PROTOCOL_VERSION）。
const ProtocolVersion = "1.0"

// 镜像自 hid-command-schema.ts COMMAND_CONSTANTS。
const (
	QueueSize          = 32   // ESP32 侧命令队列深度
	PayloadMaxBytes    = 2048 // 单条命令 payload 最大字节
	DedupCacheSize     = 256  // request_id 去重缓存条目（设备侧）
	TTLMsMin           = 100
	TTLMsMax           = 10000
	RequestIDMaxLen    = 96
	DeviceIDPattern    = `^HID-[A-Z0-9]{8}$`
	StaleDeviceSession = "STALE_DEVICE_SESSION"
)

// CommandType 命令类型枚举。
type CommandType string

const (
	TypeKeyboard CommandType = "keyboard"
	TypeMouse    CommandType = "mouse"
	TypeSystem   CommandType = "system"
)

// KeyboardAction / MouseAction / SystemAction 镜像 TS action 联合类型。
type KeyboardAction string

const (
	KBTap     KeyboardAction = "tap"
	KBHotkey  KeyboardAction = "hotkey"
	KBKeyDown KeyboardAction = "key_down"
	KBKeyUp   KeyboardAction = "key_up"
)

type MouseAction string

const (
	MouseMove       MouseAction = "move"
	MouseClick      MouseAction = "click"
	MouseButtonDown MouseAction = "button_down"
	MouseButtonUp   MouseAction = "button_up"
	MouseWheel      MouseAction = "wheel"
)

type SystemAction string

const (
	SysReleaseAll SystemAction = "release_all"
)

// KeyboardPayload 键盘载荷。
// Key 用于 tap/key_down/key_up；Keys 用于 hotkey。
// LeaseMs 仅 key_down 有效（到期自动 key_up，防卡键）。
type KeyboardPayload struct {
	Key     string   `json:"key,omitempty"`
	Keys    []string `json:"keys,omitempty"`
	HoldMs  int      `json:"hold_ms,omitempty"`
	LeaseMs int      `json:"lease_ms,omitempty"`
}

// MousePayload 鼠标载荷（V1 仅相对移动）。
// Button: left/right/middle。LeaseMs 仅 button_down 有效。
type MousePayload struct {
	DX      int    `json:"dx,omitempty"`
	DY      int    `json:"dy,omitempty"`
	Button  string `json:"button,omitempty"`
	Count   int    `json:"count,omitempty"`
	Delta   int    `json:"delta,omitempty"`
	LeaseMs int    `json:"lease_ms,omitempty"`
}

// SmartHidCommand 是 MQTT command envelope（8 必填字段）。
// 镜像自 hid-command-schema.ts SmartHidCommand。
type SmartHidCommand struct {
	Protocol     string      `json:"protocol"`       // 必填，const "1.0"
	RequestID    string      `json:"request_id"`     // 必填，≤96 字符
	DeviceID     string      `json:"device_id"`      // 必填，^HID-[A-Z0-9]{8}$
	TargetBootID string      `json:"target_boot_id"` // 必填
	Type         CommandType `json:"type"`           // 必填 keyboard|mouse|system
	Action       string      `json:"action"`         // 必填，依 type 校验
	TTLMs        int         `json:"ttl_ms"`         // 必填 [100,10000]
	Payload      interface{} `json:"payload"`        // 必填，依 type/action 解析
}

// AckStatus 是 ACK 状态枚举（6 值）。
type AckStatus string

const (
	AckReceived  AckStatus = "received"
	AckExecuting AckStatus = "executing"
	AckExecuted  AckStatus = "executed"
	AckRejected  AckStatus = "rejected"
	AckExpired   AckStatus = "expired"
	AckDuplicate AckStatus = "duplicate"
)

// IsTerminal 返回该 ACK 状态是否为终态（不再变化）。
// received/executing 是中间态，其余终态。
func (s AckStatus) IsTerminal() bool {
	switch s {
	case AckExecuted, AckRejected, AckExpired, AckDuplicate:
		return true
	}
	return false
}

// IsSuccess 返回是否成功终态（仅 executed）。
func (s AckStatus) IsSuccess() bool {
	return s == AckExecuted
}

// SmartHidAck 是 MQTT ack envelope（6 必填字段 + 可选 execution_ms）。
// 镜像自 hid-command-schema.ts SmartHidAck。
type SmartHidAck struct {
	Protocol    string    `json:"protocol"`   // "1.0"
	RequestID   string    `json:"request_id"` // 回显 Command
	DeviceID    string    `json:"device_id"`
	BootID      string    `json:"boot_id"` // 设备当前 boot_id
	Status      AckStatus `json:"status"`  // 6 值之一
	Code        int       `json:"code"`    // 0 成功，非 0 错误
	ExecutionMs int       `json:"execution_ms,omitempty"`
}

// 注：SmartHidStatus 已移到 internal/protocol 包，避免 device ↔ command 循环引用。
