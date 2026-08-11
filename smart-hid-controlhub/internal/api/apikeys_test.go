package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestAPIKeys_ListAndRotate 端到端：seed testAPIKey → GET list → POST rotate → 验证旧 key 失效、新 key 生效。
func TestAPIKeys_ListAndRotate(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})

	// 1. GET list 应该看到 1 个 active key
	status, j, _ := req(t, base, "GET", "/api/v1/api-keys", nil, true)
	if status != 200 {
		t.Fatalf("list status = %d, want 200", status)
	}
	keys, ok := j["keys"].([]any)
	if !ok || len(keys) != 1 {
		t.Fatalf("keys = %v, want 1 entry", j["keys"])
	}
	first := keys[0].(map[string]any)
	if first["active"] != true {
		t.Errorf("first key active = %v, want true", first["active"])
	}

	// 2. POST rotate
	status, j, _ = req(t, base, "POST", "/api/v1/api-keys/rotate", map[string]any{"label": "test-rotate"}, true)
	if status != 200 {
		t.Fatalf("rotate status = %d, want 200; body=%v", status, j)
	}
	newKey, ok := j["api_key"].(string)
	if !ok || !strings.HasPrefix(newKey, "chk_") {
		t.Fatalf("rotate api_key = %v", j["api_key"])
	}

	// 3. 旧 key 应失效（GET list 401）
	status, _, _ = req(t, base, "GET", "/api/v1/api-keys", nil, true) // 仍用 testAPIKey
	if status != 401 {
		t.Errorf("old key status = %d, want 401 after rotate", status)
	}

	// 4. 新 key 应可用（构造一个手动请求带新 key）
	httpReq, _ := http.NewRequest("GET", base+"/api/v1/api-keys", nil)
	httpReq.Header.Set("Authorization", "Bearer "+newKey)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("new key status = %d, want 200", resp.StatusCode)
	}
	var raw map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&raw)
	list, _ := raw["keys"].([]any)
	if len(list) != 2 {
		t.Errorf("after rotate, list len = %d, want 2", len(list))
	}
}

func TestAPIKeys_NoAuth(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	status, _, _ := req(t, base, "GET", "/api/v1/api-keys", nil, false)
	if status != 401 {
		t.Errorf("GET /api-keys without auth status = %d, want 401", status)
	}
	status, _, _ = req(t, base, "POST", "/api/v1/api-keys/rotate", nil, false)
	if status != 401 {
		t.Errorf("POST /api-keys/rotate without auth status = %d, want 401", status)
	}
}
