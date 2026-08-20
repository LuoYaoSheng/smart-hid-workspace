// validator_payload_test.go — payload 深度校验回归（M1-G2）。
// 合法 fixtures 收集自当前真实用法：demo.js、控制台 API 测试页、
// openapi.yaml 示例、README 示例、mock-device、固件 e2e 脚本 —— 全部必须 PASS。
package command

import "testing"

func payloadCmd(typ CommandType, action string, payload any) *SmartHidCommand {
	return &SmartHidCommand{
		Protocol: ProtocolVersion, RequestID: "req-p", DeviceID: "HID-ABCD1234",
		TargetBootID: "boot-1", Type: typ, Action: action, TTLMs: 3000, Payload: payload,
	}
}

// TestValidatePayload_ValidFixtures —— 现有全部合法 payload 形态（兼容性 gate）。
func TestValidatePayload_ValidFixtures(t *testing.T) {
	valid := map[string]*SmartHidCommand{
		// 键盘：README/openapi/demo 常用形态
		"tap-enter":        payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "ENTER"}),
		"tap-enter-hold":   payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "ENTER", "hold_ms": 40}),
		"tap-letter":       payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "a"}),
		"tap-letter-upper": payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "Z"}),
		"tap-digit":        payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "DIGIT5"}),
		"tap-pgdn-alias":   payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "PGDN"}),
		"hotkey-3":         payloadCmd(TypeKeyboard, "hotkey", map[string]any{"keys": []any{"CTRL", "SHIFT", "S"}}),
		"hotkey-mod-alias": payloadCmd(TypeKeyboard, "hotkey", map[string]any{"keys": []any{"WIN", "R"}}),
		"hotkey-single":    payloadCmd(TypeKeyboard, "hotkey", map[string]any{"keys": []any{"F5"}, "hold_ms": 60}),
		"key-down-lease":   payloadCmd(TypeKeyboard, "key_down", map[string]any{"key": "SHIFT", "lease_ms": 2000}),
		"key-up":           payloadCmd(TypeKeyboard, "key_up", map[string]any{"key": "SHIFT"}),
		// 鼠标：demo 触控板/按键/滚轮
		"move-both":          payloadCmd(TypeMouse, "move", map[string]any{"dx": -320, "dy": 180}),
		"move-dx-only":       payloadCmd(TypeMouse, "move", map[string]any{"dx": 12}),
		"move-max":           payloadCmd(TypeMouse, "move", map[string]any{"dx": 4096, "dy": -4096}),
		"click-left":         payloadCmd(TypeMouse, "click", map[string]any{"button": "left", "count": 1}),
		"click-default":      payloadCmd(TypeMouse, "click", map[string]any{}),
		"click-middle-count": payloadCmd(TypeMouse, "click", map[string]any{"button": "MIDDLE", "count": 3}),
		"button-down-lease":  payloadCmd(TypeMouse, "button_down", map[string]any{"button": "right", "lease_ms": 1500}),
		"button-up":          payloadCmd(TypeMouse, "button_up", map[string]any{"button": "RIGHT"}),
		"wheel-up":           payloadCmd(TypeMouse, "wheel", map[string]any{"delta": -1}),
		"wheel-boundary":     payloadCmd(TypeMouse, "wheel", map[string]any{"delta": 127}),
		// 系统
		"release-all-empty": payloadCmd(TypeSystem, "release_all", map[string]any{}),
		"release-all-nil":   payloadCmd(TypeSystem, "release_all", nil),
		// 键名大小写不敏感（固件 strcasecmp）
		"tap-lowercase-name": payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "enter"}),
	}
	for name, cmd := range valid {
		if errs := ValidatePayload(cmd); HasErrors(errs) {
			t.Errorf("fixture %q should be valid, got %v", name, fields(errs))
		}
	}
}

// TestValidatePayload_Invalid —— 非法 payload 矩阵。
func TestValidatePayload_Invalid(t *testing.T) {
	invalid := map[string]*SmartHidCommand{
		"tap-no-key":         payloadCmd(TypeKeyboard, "tap", map[string]any{}),
		"tap-unknown-key":    payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "PUNCTUATION"}),
		"tap-symbol-key":     payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "!"}),
		"tap-key-not-string": payloadCmd(TypeKeyboard, "tap", map[string]any{"key": 123}),
		"tap-hold-negative":  payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "A", "hold_ms": -1}),
		"tap-hold-too-big":   payloadCmd(TypeKeyboard, "tap", map[string]any{"key": "A", "hold_ms": 999999}),
		"hotkey-no-keys":     payloadCmd(TypeKeyboard, "hotkey", map[string]any{}),
		"hotkey-keys-empty":  payloadCmd(TypeKeyboard, "hotkey", map[string]any{"keys": []any{}}),
		"hotkey-9-keys":      payloadCmd(TypeKeyboard, "hotkey", map[string]any{"keys": []any{"CTRL", "A", "S", "D", "F", "G", "H", "J", "K"}}),
		"hotkey-bad-member":  payloadCmd(TypeKeyboard, "hotkey", map[string]any{"keys": []any{"CTRL", "???"}}),
		"hotkey-keys-string": payloadCmd(TypeKeyboard, "hotkey", map[string]any{"keys": "CTRL"}),
		"keydown-no-lease":   payloadCmd(TypeKeyboard, "key_down", map[string]any{"key": "A"}),
		"keydown-lease-max":  payloadCmd(TypeKeyboard, "key_down", map[string]any{"key": "A", "lease_ms": 999999}),
		"keyup-no-key":       payloadCmd(TypeKeyboard, "key_up", map[string]any{}),
		"move-no-axis":       payloadCmd(TypeMouse, "move", map[string]any{}),
		"move-dx-float":      payloadCmd(TypeMouse, "move", map[string]any{"dx": 1.5}),
		"move-dx-too-big":    payloadCmd(TypeMouse, "move", map[string]any{"dx": 99999}),
		"move-dx-string":     payloadCmd(TypeMouse, "move", map[string]any{"dx": "10"}),
		"click-bad-button":   payloadCmd(TypeMouse, "click", map[string]any{"button": "side"}),
		"click-count-zero":   payloadCmd(TypeMouse, "click", map[string]any{"count": 0}),
		"click-count-11":     payloadCmd(TypeMouse, "click", map[string]any{"count": 11}),
		"bd-lease-negative":  payloadCmd(TypeMouse, "button_down", map[string]any{"lease_ms": -5}),
		"wheel-no-delta":     payloadCmd(TypeMouse, "wheel", map[string]any{}),
		"wheel-128":          payloadCmd(TypeMouse, "wheel", map[string]any{"delta": 128}),
		"wheel--200":         payloadCmd(TypeMouse, "wheel", map[string]any{"delta": -200}),
		"wheel-float":        payloadCmd(TypeMouse, "wheel", map[string]any{"delta": 1.2}),
		"release-all-junk":   payloadCmd(TypeSystem, "release_all", map[string]any{"key": "A"}),
		"payload-not-object": payloadCmd(TypeKeyboard, "tap", "ENTER"),
		"payload-null-kb":    payloadCmd(TypeKeyboard, "tap", nil),
	}
	for name, cmd := range invalid {
		if errs := ValidatePayload(cmd); !HasErrors(errs) {
			t.Errorf("case %q should be rejected, but passed validation", name)
		}
	}
}

// TestValidatePayload_TypedStructPayload —— 类型化 payload（测试/内部调用方）同样校验。
func TestValidatePayload_TypedStructPayload(t *testing.T) {
	ok := payloadCmd(TypeKeyboard, "tap", KeyboardPayload{Key: "ENTER", HoldMs: 40})
	if errs := ValidatePayload(ok); HasErrors(errs) {
		t.Fatalf("typed payload should pass, got %v", fields(errs))
	}
	bad := payloadCmd(TypeKeyboard, "tap", KeyboardPayload{Key: "@@"})
	if errs := ValidatePayload(bad); !HasErrors(errs) {
		t.Fatal("typed payload with unknown key should fail")
	}
}
