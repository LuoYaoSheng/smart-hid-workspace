package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSettings_LAN_DefaultFalse(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	status, j, _ := req(t, base, "GET", "/api/v1/settings/lan-mode", nil, true)
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	if j["enabled"] != false {
		t.Errorf("default enabled = %v, want false", j["enabled"])
	}
}

func TestSettings_LAN_ToggleOnOff(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})

	// 开
	status, j, _ := req(t, base, "POST", "/api/v1/settings/lan-mode",
		map[string]any{"enabled": true}, true)
	if status != 200 {
		t.Fatalf("POST on status = %d, body=%v", status, j)
	}
	if j["enabled"] != true {
		t.Errorf("after POST on, enabled = %v", j["enabled"])
	}
	// 回读
	_, j, _ = req(t, base, "GET", "/api/v1/settings/lan-mode", nil, true)
	if j["enabled"] != true {
		t.Errorf("GET after POST on, enabled = %v", j["enabled"])
	}

	// 关
	_, j, _ = req(t, base, "POST", "/api/v1/settings/lan-mode",
		map[string]any{"enabled": false}, true)
	if j["enabled"] != false {
		t.Errorf("after POST off, enabled = %v", j["enabled"])
	}
}

func TestSettings_LAN_BadJSON(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	status, j, _ := reqRaw(t, base, "POST", "/api/v1/settings/lan-mode", "{bad", true)
	if status != 400 {
		t.Fatalf("status = %d, want 400", status)
	}
	if j["error"] != "bad_request" {
		t.Errorf("error = %v", j["error"])
	}
}

func TestSettings_LAN_MethodNotAllowed(t *testing.T) {
	base, _, _, _ := newTestServer(t, &mockClient{})
	httpReq, _ := http.NewRequest("DELETE", base+"/api/v1/settings/lan-mode", nil)
	httpReq.Header.Set("Authorization", "Bearer "+testAPIKey)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Errorf("DELETE status = %d, want 405", resp.StatusCode)
	}
	// 验证 body 是合法 JSON
	var raw map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&raw)
	if raw["error"] != "method_not_allowed" {
		t.Errorf("error = %v", raw["error"])
	}
}
