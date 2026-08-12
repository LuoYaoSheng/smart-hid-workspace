package sys

import (
	"strings"
	"testing"
)

func TestGetMachineAnchor_NonEmpty(t *testing.T) {
	a := GetMachineAnchor()
	if a == "" {
		t.Fatal("anchor empty")
	}
	// 不应是 fallback（除非真的取不到；macOS 应该能取到 UUID）
	if strings.HasPrefix(a, "fallback-") {
		t.Logf("WARNING: anchor is fallback: %s (OS command may have failed)", a)
	}
	t.Logf("machine anchor: %s", a)
}

func TestGetMachineAnchor_StableAcrossCalls(t *testing.T) {
	a1 := GetMachineAnchor()
	a2 := GetMachineAnchor()
	if a1 != a2 {
		t.Errorf("anchor unstable: %q vs %q", a1, a2)
	}
}
