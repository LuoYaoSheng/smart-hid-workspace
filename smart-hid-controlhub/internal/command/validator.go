package command

import (
	"encoding/json"
	"fmt"
	"regexp"
)

var deviceIDRegex = regexp.MustCompile(DeviceIDPattern)

// ValidationError 表示 envelope 校验失败。HTTP 层据此映射 4xx。
type ValidationError struct {
	Field   string // 字段路径，如 "ttl_ms"
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation[%s]: %s", e.Field, e.Message)
}

// ValidateCommand 校验 SmartHidCommand envelope 的所有必填字段与约束。
// Phase 1 不校验 payload 内部结构（type/action 合法即可），
// 深度 payload 校验在设备侧（Phase 3 接入真实固件时强化）。
func ValidateCommand(cmd *SmartHidCommand) []*ValidationError {
	var errs []*ValidationError

	if cmd.Protocol != ProtocolVersion {
		errs = append(errs, &ValidationError{"protocol", fmt.Sprintf("must be %q, got %q", ProtocolVersion, cmd.Protocol)})
	}
	if cmd.RequestID == "" {
		errs = append(errs, &ValidationError{"request_id", "required"})
	} else if len(cmd.RequestID) > RequestIDMaxLen {
		errs = append(errs, &ValidationError{"request_id", fmt.Sprintf("max length %d, got %d", RequestIDMaxLen, len(cmd.RequestID))})
	}
	if cmd.DeviceID == "" {
		errs = append(errs, &ValidationError{"device_id", "required"})
	} else if !deviceIDRegex.MatchString(cmd.DeviceID) {
		errs = append(errs, &ValidationError{"device_id", fmt.Sprintf("must match %s, got %q", DeviceIDPattern, cmd.DeviceID)})
	}
	if cmd.TargetBootID == "" {
		errs = append(errs, &ValidationError{"target_boot_id", "required"})
	}
	if cmd.TTLMs < TTLMsMin || cmd.TTLMs > TTLMsMax {
		errs = append(errs, &ValidationError{"ttl_ms", fmt.Sprintf("must be in [%d, %d], got %d", TTLMsMin, TTLMsMax, cmd.TTLMs)})
	}

	// type + action 合法性
	switch cmd.Type {
	case TypeKeyboard:
		if !isValidKeyboardAction(cmd.Action) {
			errs = append(errs, &ValidationError{"action", fmt.Sprintf("invalid keyboard action %q", cmd.Action)})
		}
	case TypeMouse:
		if !isValidMouseAction(cmd.Action) {
			errs = append(errs, &ValidationError{"action", fmt.Sprintf("invalid mouse action %q", cmd.Action)})
		}
	case TypeSystem:
		if SystemAction(cmd.Action) != SysReleaseAll {
			errs = append(errs, &ValidationError{"action", fmt.Sprintf("invalid system action %q", cmd.Action)})
		}
	default:
		errs = append(errs, &ValidationError{"type", fmt.Sprintf("must be keyboard|mouse|system, got %q", cmd.Type)})
	}

	// payload 大小（序列化后 ≤ PayloadMaxBytes）
	if cmd.Payload != nil {
		if b, err := json.Marshal(cmd.Payload); err == nil && len(b) > PayloadMaxBytes {
			errs = append(errs, &ValidationError{"payload", fmt.Sprintf("serialized %d bytes > max %d", len(b), PayloadMaxBytes)})
		}
	}

	return errs
}

func isValidKeyboardAction(a string) bool {
	switch KeyboardAction(a) {
	case KBTap, KBHotkey, KBKeyDown, KBKeyUp:
		return true
	}
	return false
}

func isValidMouseAction(a string) bool {
	switch MouseAction(a) {
	case MouseMove, MouseClick, MouseButtonDown, MouseButtonUp, MouseWheel:
		return true
	}
	return false
}

// HasErrors 便捷判断 ValidateCommand 返回值是否含错误。
func HasErrors(errs []*ValidationError) bool { return len(errs) > 0 }
