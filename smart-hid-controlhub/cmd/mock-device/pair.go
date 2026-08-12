package main

// mock-device --pair 支持：调用 ControlHub 配对端点拿 MQTT 凭据（CH-P5）。
//
// 流程：先正常解析 flag + NewDevice（生成 boot_id），
// 若 --pair-url 非空，调 POST <pair-url>，
// 用返回的 mqtt_username / mqtt_credential / mqtt_host / mqtt_port 覆盖 flag 值，
// 然后正常进入 MQTT 连接流程，验证 PerDeviceHook 鉴权。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

type pairResult struct {
	MQTTHost       string `json:"mqtt_host"`
	MQTTPort       int    `json:"mqtt_port"`
	MQTTUsername   string `json:"mqtt_username"`
	MQTTCredential string `json:"mqtt_credential"`
	DeviceID       string `json:"device_id"`
}

// doPair 调 ControlHub 设备侧配对端点（POST /api/v1/pairing/device）拿 MQTT 凭据。
// url 形如 "http://127.0.0.1:17892/api/v1/pairing/device"。
func doPair(url, token, deviceID, bootID string, log *slog.Logger) (*pairResult, error) {
	body, _ := json.Marshal(map[string]string{
		"token":    token,
		"device_id": deviceID,
		"boot_id":  bootID,
		"firmware": "1.0.0-mock",
		"hardware": "mock-v1",
	})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pairing HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var r pairResult
	if err := json.Unmarshal(respBody, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	log.Info("pairing succeeded",
		"mqtt_user", r.MQTTUsername, "mqtt_host", r.MQTTHost, "mqtt_port", r.MQTTPort)
	return &r, nil
}
