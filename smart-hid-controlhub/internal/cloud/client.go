// Package cloud 实现 ControlHub → Smart HID Cloud 的出站 HTTP 客户端（CL-6b）。
//
// 用途：在线激活（激活码换签名 License）+ License 刷新（拉最新签名 License 续期）。
// 返回的原始 License JSON 字节直接喂给 licmgr.Import（含 VerifyFull + upsert）。
//
// 设计：
//   - 纯 HTTP 客户端，无状态，不依赖 api/license 包（避免循环依赖）。
//   - Cloud 的 consume/refresh 是 PUBLIC endpoint（码/license_id 即凭据），无需鉴权头。
//   - 网络失败/超时返回 error，调用方负责降级（离线时本地 License 继续有效）。
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Client 是 Smart HID Cloud 的出站客户端。
type Client struct {
	baseURL string // 形如 http://host:port/api/v1（无尾斜杠）
	http    *http.Client
	log     *slog.Logger
}

// New 构造 Client。baseURL 为空则返 nil（纯离线模式，调用方应判 nil）。
func New(baseURL string, log *slog.Logger) *Client {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
		log: log,
	}
}

// postJSON 发 POST，返响应 body。非 2xx → 带 status + Cloud error message 的 error。
func (c *Client) postJSON(ctx context.Context, path string, body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call cloud %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 尝试解析 Cloud 的 {error, message} 体
		var eb struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &eb)
		msg := eb.Message
		if msg == "" {
			msg = eb.Error
		}
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return nil, &APIError{Status: resp.StatusCode, Path: path, Message: msg}
	}
	return raw, nil
}

// APIError 表示 Cloud 返回的非 2xx 错误。
type APIError struct {
	Status  int
	Path    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("cloud %s: HTTP %d: %s", e.Path, e.Status, e.Message)
}

// ConsumeActivationCode 调 POST /activation/consume，返回签名 License JSON 原始字节。
// code 在客户端做大写 + 去分隔符（- / 空格）；完整 Crockford 归一化（I/L→1 等）由 Cloud 端权威处理。
func (c *Client) ConsumeActivationCode(ctx context.Context, code, deviceID string) ([]byte, error) {
	normalized := strings.NewReplacer("-", "", " ", "").Replace(strings.ToUpper(strings.TrimSpace(code)))
	return c.postJSON(ctx, "/activation/consume", map[string]string{
		"code":      normalized,
		"device_id": deviceID,
	})
}

// RefreshLicense 调 POST /license/refresh，返回签名 License JSON 原始字节。
func (c *Client) RefreshLicense(ctx context.Context, licenseID string) ([]byte, error) {
	return c.postJSON(ctx, "/license/refresh", map[string]string{
		"license_id": licenseID,
	})
}
