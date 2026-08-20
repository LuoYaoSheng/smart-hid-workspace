package mqtt

import (
	"strings"
	"testing"
)

// TestDefaultInternalMQTTCredentialIsNotStatic（spec M1-G3 §13）：
// 生成的内部凭据每次不同，且不再是任何固定默认值（历史 "change-me-in-production"
// 已从代码库消失）。
func TestDefaultInternalMQTTCredentialIsNotStatic(t *testing.T) {
	u1, p1, err := GenerateInternalCredential()
	if err != nil {
		t.Fatal(err)
	}
	u2, p2, err := GenerateInternalCredential()
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatal("per-boot random credential must differ across generations")
	}
	if u1 != InternalUsername || u2 != InternalUsername {
		t.Fatalf("username = %q/%q, want %q", u1, u2, InternalUsername)
	}
	if len(p1) != 48 { // 24 bytes hex
		t.Fatalf("password length = %d, want 48", len(p1))
	}
	if strings.Contains(p1, "change-me") || p1 == "test-pass-f2" {
		t.Fatal("generated credential must never be a known fixed value")
	}
}
