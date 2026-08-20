// validator_payload.go — command payload 深度校验（M1-G2）。
//
// 原则：HTTP API → server validation → MQTT → firmware validation 双重保护。
// 规则镜像固件事实（smart-hid-firmware）：
//   - 键名：单字母 a-z/A-Z 快速路径 + keymap 表（大小写不敏感，strcasecmp）
//   - keys ≤ 8 个（固件 keys[8][8]）
//   - dx/dy int32，固件按 ±127 分片
//   - wheel 单次 int8 强转（无分片）→ 必须 [-127,127]，否则设备端静默截断
//   - count uint8，固件 0/缺省按 1 处理
//   - button LEFT/RIGHT/MIDDLE（大小写不敏感，缺省 LEFT）
//   - lease 缺省：key_down 必带；button_down 缺省 5000ms（固件容错）
package command

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Payload 校验常量（与固件/TS schema 对齐）。
const (
	HoldMsMax     = 10000 // hold_ms 上限（默认 40）
	LeaseMsMin    = 1
	LeaseMsMax    = 60000 // lease 上限（TTL≤10s，lease 允许更长兜底）
	MoveAxisMax   = 4096  // dx/dy 上限（固件 int32 + ±127 分片，限 sane 值）
	WheelDeltaMax = 127   // 固件单次 int8 强转，超界即截断——硬限
	ClickCountMax = 10
	HotkeyKeysMax = 8 // 固件 keys[8][8]
	KeyNameMaxLen = 16
)

// knownKeys 固件 hid_keymap 表全集（大写规范化；单字母走快速路径，不在表内）。
var knownKeys = map[string]struct{}{}

func init() {
	for _, k := range []string{
		"CTRL", "CONTROL", "SHIFT", "ALT", "GUI", "META", "WIN", "CMD", "OPTION",
		"ENTER", "RETURN", "ESC", "ESCAPE",
		"BACKSPACE", "TAB", "SPACE", "CAPSLOCK", "CAPS",
		"LEFT", "RIGHT", "UP", "DOWN",
		"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12",
		"INSERT", "INS", "HOME", "PAGEUP", "PGUP", "DELETE", "DEL", "END", "PAGEDOWN", "PGDN",
		"DIGIT0", "DIGIT1", "DIGIT2", "DIGIT3", "DIGIT4",
		"DIGIT5", "DIGIT6", "DIGIT7", "DIGIT8", "DIGIT9",
	} {
		knownKeys[k] = struct{}{}
	}
}

// isValidKeyName 键名合法性：单字母 a-z/A-Z，或 keymap 表内名字（大小写不敏感）。
// 与固件 hid_keymap_lookup 行为一致。
func isValidKeyName(name string) bool {
	if name == "" || len(name) > KeyNameMaxLen {
		return false
	}
	if len(name) == 1 {
		c := name[0]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
		return false
	}
	_, ok := knownKeys[strings.ToUpper(name)]
	return ok
}

func isValidButtonName(name string) bool {
	switch strings.ToUpper(name) {
	case "LEFT", "RIGHT", "MIDDLE":
		return true
	}
	return false
}

// ValidatePayload 校验 payload 的字段级内容（envelope 之后的深度校验）。
// payload 可能是 JSON 解码出的 map[string]any（HTTP 真实路径），也可能是
// 调用方构造的类型化 struct（测试）——统一先规范化为 map 再检查。
func ValidatePayload(cmd *SmartHidCommand) []*ValidationError {
	var errs []*ValidationError
	add := func(field, msg string) {
		errs = append(errs, &ValidationError{Field: field, Message: msg})
	}

	var m map[string]any
	if cmd.Payload != nil {
		if mm, ok := cmd.Payload.(map[string]any); ok {
			m = mm
		} else {
			// 类型化 payload：round-trip 经 JSON 规范化为 map。
			b, err := json.Marshal(cmd.Payload)
			if err != nil {
				add("payload", "must be a JSON object")
				return errs
			}
			if err := json.Unmarshal(b, &m); err != nil || m == nil {
				add("payload", "must be a JSON object")
				return errs
			}
		}
	}

	str := func(field string) (string, bool, bool) { // value, present, typeOK
		v, ok := m[field]
		if !ok {
			return "", false, true
		}
		s, ok := v.(string)
		return s, true, ok
	}
	intVal := func(field string) (int, bool, bool) {
		v, ok := m[field]
		if !ok {
			return 0, false, true
		}
		switch n := v.(type) {
		case float64: // JSON 解码路径
			if n != math.Trunc(n) {
				return 0, true, false
			}
			return int(n), true, true
		case int: // Go 字面量 / 内部构造路径
			return n, true, true
		case int64:
			return int(n), true, true
		default:
			return 0, true, false
		}
	}

	keyField := func(field string) string {
		s, present, ok := str(field)
		if !present {
			return fmt.Sprintf("%s required", field)
		}
		if !ok {
			return fmt.Sprintf("%s must be a string", field)
		}
		if !isValidKeyName(s) {
			return fmt.Sprintf("%s: unknown key name %q", field, s)
		}
		return ""
	}
	holdField := func() string {
		v, present, ok := intVal("hold_ms")
		if !present {
			return ""
		}
		if !ok {
			return "hold_ms must be an integer"
		}
		if v < 0 || v > HoldMsMax {
			return fmt.Sprintf("hold_ms must be in [0, %d], got %d", HoldMsMax, v)
		}
		return ""
	}
	leaseField := func(required bool) string {
		v, present, ok := intVal("lease_ms")
		if !present {
			if required {
				return "lease_ms required"
			}
			return ""
		}
		if !ok {
			return "lease_ms must be an integer"
		}
		if v < LeaseMsMin || v > LeaseMsMax {
			return fmt.Sprintf("lease_ms must be in [%d, %d], got %d", LeaseMsMin, LeaseMsMax, v)
		}
		return ""
	}
	buttonField := func() string {
		s, present, ok := str("button")
		if !present {
			return ""
		}
		if !ok {
			return "button must be a string"
		}
		if !isValidButtonName(s) {
			return fmt.Sprintf("button must be LEFT/RIGHT/MIDDLE (case-insensitive), got %q", s)
		}
		return ""
	}

	switch cmd.Type {
	case TypeKeyboard:
		switch KeyboardAction(cmd.Action) {
		case KBTap:
			if msg := keyField("key"); msg != "" {
				add("payload.key", msg)
			}
			if msg := holdField(); msg != "" {
				add("payload.hold_ms", msg)
			}
		case KBHotkey:
			v, ok := m["keys"]
			if !ok {
				add("payload.keys", "keys required for hotkey")
				break
			}
			arr, isArr := v.([]any)
			if !isArr {
				add("payload.keys", "keys must be an array of key names")
				break
			}
			if len(arr) < 1 || len(arr) > HotkeyKeysMax {
				add("payload.keys", fmt.Sprintf("keys must have 1..%d entries, got %d", HotkeyKeysMax, len(arr)))
				break
			}
			for i, item := range arr {
				s, ok := item.(string)
				if !ok || !isValidKeyName(s) {
					add("payload.keys", fmt.Sprintf("keys[%d]: unknown or invalid key name %v", i, item))
				}
			}
			if msg := holdField(); msg != "" {
				add("payload.hold_ms", msg)
			}
		case KBKeyDown:
			if msg := keyField("key"); msg != "" {
				add("payload.key", msg)
			}
			if msg := leaseField(true); msg != "" {
				add("payload.lease_ms", msg)
			}
		case KBKeyUp:
			if msg := keyField("key"); msg != "" {
				add("payload.key", msg)
			}
		default:
			add("action", fmt.Sprintf("invalid keyboard action %q", cmd.Action))
		}

	case TypeMouse:
		switch MouseAction(cmd.Action) {
		case MouseMove:
			dx, dxPresent, dxOK := intVal("dx")
			dy, dyPresent, dyOK := intVal("dy")
			if !dxPresent && !dyPresent {
				add("payload", "move requires dx and/or dy")
			}
			checkAxis := func(name string, v int, present, ok bool) {
				if !present || !ok {
					if present {
						add("payload."+name, name+" must be an integer")
					}
					return
				}
				if v < -MoveAxisMax || v > MoveAxisMax {
					add("payload."+name, fmt.Sprintf("%s must be in [-%d, %d], got %d", name, MoveAxisMax, MoveAxisMax, v))
				}
			}
			checkAxis("dx", dx, dxPresent, dxOK)
			checkAxis("dy", dy, dyPresent, dyOK)
		case MouseClick:
			if msg := buttonField(); msg != "" {
				add("payload.button", msg)
			}
			v, present, ok := intVal("count")
			if present {
				if !ok {
					add("payload.count", "count must be an integer")
				} else if v < 1 || v > ClickCountMax {
					add("payload.count", fmt.Sprintf("count must be in [1, %d], got %d", ClickCountMax, v))
				}
			}
		case MouseButtonDown:
			if msg := buttonField(); msg != "" {
				add("payload.button", msg)
			}
			if msg := leaseField(false); msg != "" {
				add("payload.lease_ms", msg)
			}
		case MouseButtonUp:
			if msg := buttonField(); msg != "" {
				add("payload.button", msg)
			}
		case MouseWheel:
			v, present, ok := intVal("delta")
			if !present {
				add("payload.delta", "delta required for wheel")
			} else if !ok {
				add("payload.delta", "delta must be an integer")
			} else if v < -WheelDeltaMax || v > WheelDeltaMax {
				// 固件单次 int8 强转：超界会被静默截断，服务端硬性拒绝。
				add("payload.delta", fmt.Sprintf("delta must be in [-%d, %d], got %d", WheelDeltaMax, WheelDeltaMax, v))
			}
		default:
			add("action", fmt.Sprintf("invalid mouse action %q", cmd.Action))
		}

	case TypeSystem:
		if SystemAction(cmd.Action) == SysReleaseAll {
			if len(m) > 0 {
				add("payload", "release_all takes no payload fields")
			}
		} else {
			add("action", fmt.Sprintf("invalid system action %q", cmd.Action))
		}
	}

	return errs
}
