package cloud

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func discLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestClient_ConsumeSuccess 验证请求方法/路径/body + 返回原始字节。
func TestClient_ConsumeSuccess(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"payload":{"license_id":"lic_x"},"signature":"sig"}`))
	}))
	defer ts.Close()

	c := New(ts.URL+"/api/v1/", discLogger())
	if c == nil {
		t.Fatal("New returned nil for non-empty baseURL")
	}
	raw, err := c.ConsumeActivationCode(context.Background(), "abcd-1234", "HID-AAAAAAAA")
	if err != nil {
		t.Fatalf("ConsumeActivationCode: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/activation/consume" {
		t.Fatalf("request: got %s %s, want POST /api/v1/activation/consume", gotMethod, gotPath)
	}
	// 归一化：大写 + 去连字符
	if gotBody["code"] != "ABCD1234" {
		t.Fatalf("code normalization: got %q want ABCD1234", gotBody["code"])
	}
	if gotBody["device_id"] != "HID-AAAAAAAA" {
		t.Fatalf("device_id: got %q", gotBody["device_id"])
	}
	if string(raw) != `{"payload":{"license_id":"lic_x"},"signature":"sig"}` {
		t.Fatalf("response bytes mismatch: %q", raw)
	}
}

// TestClient_RefreshSuccess 验证 refresh 请求体 + 返回。
func TestClient_RefreshSuccess(t *testing.T) {
	var gotBody map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/license/refresh" {
			t.Fatalf("path: got %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"payload":{"license_id":"lic_y"}}`))
	}))
	defer ts.Close()

	c := New(ts.URL+"/api/v1", discLogger())
	raw, err := c.RefreshLicense(context.Background(), "lic_y")
	if err != nil {
		t.Fatalf("RefreshLicense: %v", err)
	}
	if gotBody["license_id"] != "lic_y" {
		t.Fatalf("license_id body: got %q", gotBody["license_id"])
	}
	if len(raw) == 0 {
		t.Fatal("empty response")
	}
}

// TestClient_HTTPError 非 2xx → APIError 带 status + message。
func TestClient_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"conflict","message":"activation code already used"}`))
	}))
	defer ts.Close()

	c := New(ts.URL, discLogger())
	_, err := c.ConsumeActivationCode(context.Background(), "X", "HID-AAAAAAAA")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 409 {
		t.Fatalf("status: got %d want 409", apiErr.Status)
	}
	if apiErr.Message != "activation code already used" {
		t.Fatalf("message: got %q", apiErr.Message)
	}
}

// TestClient_NetworkError 服务器关闭 → 网络错误（非 *APIError）。
func TestClient_NetworkError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close() // 立即关闭

	c := New(ts.URL, discLogger())
	_, err := c.ConsumeActivationCode(context.Background(), "X", "HID-AAAAAAAA")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if _, ok := err.(*APIError); ok {
		t.Fatalf("network error should not be *APIError: %v", err)
	}
}

// TestClient_EmptyBaseURLNil 空 baseURL → nil（纯离线模式）。
func TestClient_EmptyBaseURLNil(t *testing.T) {
	if c := New("", discLogger()); c != nil {
		t.Fatalf("empty baseURL should return nil, got %v", c)
	}
	if c := New("   ", discLogger()); c != nil {
		t.Fatalf("whitespace baseURL should return nil, got %v", c)
	}
}

// TestClient_TrailingSlashTrimmed baseURL 尾斜杠被去掉，路径仍正确拼接。
func TestClient_TrailingSlashTrimmed(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	c := New(ts.URL+"/api/v1///", discLogger())
	_, _ = c.RefreshLicense(context.Background(), "lic_z")
	if gotPath != "/api/v1/license/refresh" {
		t.Fatalf("path with trailing slashes: got %q", gotPath)
	}
}
