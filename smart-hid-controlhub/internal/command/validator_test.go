package command

import (
	"strings"
	"testing"
)

// validCmd 返回一个全字段合法的 SmartHidCommand，每个用例在其基础上破坏单个字段。
func validCmd() *SmartHidCommand {
	return &SmartHidCommand{
		Protocol:     ProtocolVersion,
		RequestID:    "req-abc123",
		DeviceID:     "HID-ABCD1234",
		TargetBootID: "boot-001",
		Type:         TypeKeyboard,
		Action:       string(KBTap),
		TTLMs:        3000,
		Payload:      KeyboardPayload{Key: "ENTER", HoldMs: 40},
	}
}

// errField 把校验错误列表压成 field 集合，便于断言。
func errFields(errs []*ValidationError) map[string]bool {
	m := make(map[string]bool, len(errs))
	for _, e := range errs {
		m[e.Field] = true
	}
	return m
}

// hasField 断言 errs 中存在指定 field。
func hasField(t *testing.T, errs []*ValidationError, field string, msgAndArgs ...any) {
	t.Helper()
	if !errFields(errs)[field] {
		t.Helper()
		if len(msgAndArgs) > 0 {
			t.Fatalf("expected validation error on field %q, got %v — %v", field, fields(errs), msgAndArgs[0])
		}
		t.Fatalf("expected validation error on field %q, got %v", field, fields(errs))
	}
}

func fields(errs []*ValidationError) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Field)
	}
	return out
}

// ---------- ValidateCommand 表驱动 ----------

func TestValidateCommand_ValidBaseline(t *testing.T) {
	// 全字段合法的命令不应有任何校验错误。
	for name, mutate := range map[string]func(*SmartHidCommand){
		"keyboard tap":   func(c *SmartHidCommand) { c.Type = TypeKeyboard; c.Action = string(KBTap) },
		"keyboard hotkey": func(c *SmartHidCommand) {
			c.Type = TypeKeyboard; c.Action = string(KBHotkey)
			c.Payload = KeyboardPayload{Keys: []string{"CTRL", "C"}}
		},
		"keyboard key_down": func(c *SmartHidCommand) { c.Type = TypeKeyboard; c.Action = string(KBKeyDown) },
		"keyboard key_up":   func(c *SmartHidCommand) { c.Type = TypeKeyboard; c.Action = string(KBKeyUp) },
		"mouse move":        func(c *SmartHidCommand) { c.Type = TypeMouse; c.Action = string(MouseMove); c.Payload = MousePayload{DX: 10, DY: -5} },
		"mouse click":       func(c *SmartHidCommand) { c.Type = TypeMouse; c.Action = string(MouseClick); c.Payload = MousePayload{Button: "left"} },
		"mouse button_down": func(c *SmartHidCommand) { c.Type = TypeMouse; c.Action = string(MouseButtonDown) },
		"mouse button_up":   func(c *SmartHidCommand) { c.Type = TypeMouse; c.Action = string(MouseButtonUp) },
		"mouse wheel":       func(c *SmartHidCommand) { c.Type = TypeMouse; c.Action = string(MouseWheel) },
		"system release_all": func(c *SmartHidCommand) { c.Type = TypeSystem; c.Action = string(SysReleaseAll) },
		"ttl at min boundary":  func(c *SmartHidCommand) { c.TTLMs = TTLMsMin },
		"ttl at max boundary":  func(c *SmartHidCommand) { c.TTLMs = TTLMsMax },
		"device_id all digits": func(c *SmartHidCommand) { c.DeviceID = "HID-12345678" },
		"nil payload":          func(c *SmartHidCommand) { c.Payload = nil },
	} {
		t.Run(name, func(t *testing.T) {
			c := validCmd()
			mutate(c)
			if errs := ValidateCommand(c); HasErrors(errs) {
				t.Fatalf("expected no errors, got %v", errs)
			}
		})
	}
}

func TestValidateCommand_ProtocolMismatch(t *testing.T) {
	c := validCmd()
	c.Protocol = "2.0"
	errs := ValidateCommand(c)
	if !HasErrors(errs) {
		t.Fatal("expected protocol error")
	}
	hasField(t, errs, "protocol")
	if errs[0].Error() == "" {
		t.Fatal("ValidationError.Error() should be non-empty")
	}
}

func TestValidateCommand_RequestID(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		c := validCmd()
		c.RequestID = ""
		hasField(t, ValidateCommand(c), "request_id")
	})
	t.Run("too_long", func(t *testing.T) {
		c := validCmd()
		c.RequestID = strings.Repeat("a", RequestIDMaxLen+1)
		hasField(t, ValidateCommand(c), "request_id")
	})
	t.Run("max_length_ok", func(t *testing.T) {
		c := validCmd()
		c.RequestID = strings.Repeat("a", RequestIDMaxLen)
		if HasErrors(ValidateCommand(c)) {
			t.Fatalf("request_id at max length %d should be valid", RequestIDMaxLen)
		}
	})
}

func TestValidateCommand_DeviceID(t *testing.T) {
	cases := []struct {
		name    string
		devID   string
		wantErr bool
	}{
		{"empty", "", true},
		{"lowercase_prefix", "hid-ABCD1234", true},
		{"too_short_7", "HID-ABCD123", true},
		{"too_long_9", "HID-ABCD12345", true},
		{"lowercase_hex", "HID-abcdefgh", true},
		{"special_char", "HID-ABCD!234", true},
		{"space", "HID-ABCD 234", true},
		{"valid_upper", "HID-ABCD1234", false},
		{"valid_alnum", "HID-1A2B3C4D", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validCmd()
			c.DeviceID = tc.devID
			errs := ValidateCommand(c)
			if tc.wantErr {
				hasField(t, errs, "device_id")
			} else if HasErrors(errs) {
				t.Fatalf("expected %q valid, got %v", tc.devID, errs)
			}
		})
	}
}

func TestValidateCommand_TargetBootID(t *testing.T) {
	c := validCmd()
	c.TargetBootID = ""
	hasField(t, ValidateCommand(c), "target_boot_id")
}

func TestValidateCommand_TTLMs(t *testing.T) {
	cases := []struct {
		name    string
		ttl     int
		wantErr bool
	}{
		{"below_min", TTLMsMin - 1, true},
		{"zero", 0, true},
		{"negative", -1, true},
		{"above_max", TTLMsMax + 1, true},
		{"min_ok", TTLMsMin, false},
		{"max_ok", TTLMsMax, false},
		{"mid_ok", (TTLMsMin + TTLMsMax) / 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validCmd()
			c.TTLMs = tc.ttl
			errs := ValidateCommand(c)
			if tc.wantErr {
				hasField(t, errs, "ttl_ms")
			} else if HasErrors(errs) {
				t.Fatalf("expected ttl=%d valid, got %v", tc.ttl, errs)
			}
		})
	}
}

func TestValidateCommand_TypeAndAction(t *testing.T) {
	t.Run("unknown_type", func(t *testing.T) {
		c := validCmd()
		c.Type = CommandType("joystick")
		hasField(t, ValidateCommand(c), "type")
	})
	t.Run("empty_type", func(t *testing.T) {
		c := validCmd()
		c.Type = ""
		hasField(t, ValidateCommand(c), "type")
	})
	t.Run("keyboard_with_mouse_action", func(t *testing.T) {
		c := validCmd()
		c.Type = TypeKeyboard
		c.Action = string(MouseClick)
		hasField(t, ValidateCommand(c), "action")
	})
	t.Run("mouse_with_keyboard_action", func(t *testing.T) {
		c := validCmd()
		c.Type = TypeMouse
		c.Action = string(KBTap)
		hasField(t, ValidateCommand(c), "action")
	})
	t.Run("system_with_wrong_action", func(t *testing.T) {
		c := validCmd()
		c.Type = TypeSystem
		c.Action = string(KBTap)
		hasField(t, ValidateCommand(c), "action")
	})
	t.Run("empty_action_keyboard", func(t *testing.T) {
		c := validCmd()
		c.Type = TypeKeyboard
		c.Action = ""
		hasField(t, ValidateCommand(c), "action")
	})
}

func TestValidateCommand_PayloadOversized(t *testing.T) {
	c := validCmd()
	// 构造一个序列化后超过 PayloadMaxBytes 的 payload。
	c.Payload = map[string]any{"blob": strings.Repeat("x", PayloadMaxBytes+100)}
	hasField(t, ValidateCommand(c), "payload")
}

func TestValidateCommand_PayloadAtMaxOK(t *testing.T) {
	c := validCmd()
	// ~PayloadMaxBytes 以内应放行（留余量给 JSON 框架字符）。
	c.Payload = map[string]any{"blob": strings.Repeat("x", PayloadMaxBytes-200)}
	if HasErrors(ValidateCommand(c)) {
		t.Fatal("payload under max should be valid")
	}
}

func TestValidateCommand_MultipleErrorsAtOnce(t *testing.T) {
	// 同时破坏多个字段，每个都应被报告（不能短路）。
	c := validCmd()
	c.Protocol = "0.9"
	c.RequestID = ""
	c.DeviceID = "bad"
	c.TargetBootID = ""
	c.TTLMs = 5
	c.Type = CommandType("joystick")

	errs := ValidateCommand(c)
	f := errFields(errs)
	for _, want := range []string{"protocol", "request_id", "device_id", "target_boot_id", "ttl_ms", "type"} {
		if !f[want] {
			t.Errorf("expected error field %q in %v", want, fields(errs))
		}
	}
}

// ---------- action 校验器（内部函数） ----------

func TestActionValidators(t *testing.T) {
	for _, a := range []KeyboardAction{KBTap, KBHotkey, KBKeyDown, KBKeyUp} {
		if !isValidKeyboardAction(string(a)) {
			t.Errorf("isValidKeyboardAction(%q) = false, want true", a)
		}
	}
	if isValidKeyboardAction("click") {
		t.Error("isValidKeyboardAction should reject mouse action")
	}
	if isValidKeyboardAction("") {
		t.Error("isValidKeyboardAction should reject empty")
	}

	for _, a := range []MouseAction{MouseMove, MouseClick, MouseButtonDown, MouseButtonUp, MouseWheel} {
		if !isValidMouseAction(string(a)) {
			t.Errorf("isValidMouseAction(%q) = false, want true", a)
		}
	}
	if isValidMouseAction("tap") {
		t.Error("isValidMouseAction should reject keyboard action")
	}
}

// ---------- AckStatus 方法 ----------

func TestAckStatus_IsTerminal(t *testing.T) {
	terminal := []AckStatus{AckExecuted, AckRejected, AckExpired, AckDuplicate}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%q should be terminal", s)
		}
	}
	nonTerminal := []AckStatus{AckReceived, AckExecuting}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%q should NOT be terminal", s)
		}
	}
}

func TestAckStatus_IsSuccess(t *testing.T) {
	if !AckExecuted.IsSuccess() {
		t.Error("executed should be success")
	}
	for _, s := range []AckStatus{AckReceived, AckExecuting, AckRejected, AckExpired, AckDuplicate} {
		if s.IsSuccess() {
			t.Errorf("%q should NOT be success", s)
		}
	}
}

func TestValidationError_Error(t *testing.T) {
	e := &ValidationError{Field: "ttl_ms", Message: "out of range"}
	s := e.Error()
	if !strings.Contains(s, "ttl_ms") || !strings.Contains(s, "out of range") {
		t.Fatalf("Error() = %q, expected to contain field and message", s)
	}
}
