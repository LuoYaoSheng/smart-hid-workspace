// app_keylog_test.go — 防止 API Key 明文进日志的回归测试（M1-G2）。
package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestInitialKeyLogOmitsPlaintext —— 首启 key 日志只能含路径，不得含 chk_ 前缀明文。
// 结构性防线：logInitialKeyGenerated 无 key 参数；本测试再锁定输出内容。
func TestInitialKeyLogOmitsPlaintext(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	logInitialKeyGenerated(log, "/tmp/some/data/initial-api-key.txt")

	out := buf.String()
	if !strings.Contains(out, "/tmp/some/data/initial-api-key.txt") {
		t.Fatalf("log should contain key file path, got: %s", out)
	}
	if strings.Contains(out, apikeyPlaintextPrefix) {
		t.Fatalf("log must not contain API key plaintext prefix, got: %s", out)
	}
}

const apikeyPlaintextPrefix = "chk_"
