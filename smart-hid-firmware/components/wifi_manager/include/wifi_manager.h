/**
 * wifi_manager.h — Wi-Fi STA 模式
 *
 * M1-G3：SSID/密码来自 NVS runtime config（BLE Provisioning 写入）；
 * Kconfig 仅在 CONFIG_SMART_HID_DEV_STATIC_CONFIG 显式开启且无 NVS 配置时
 * 作为开发 fallback（仅内存，绝不写 NVS）。
 *
 * Fail-safe：Wi-Fi 断开 → 触发 hid_engine_release_all()（弱符号 hook）+ 自动重连。
 */
#pragma once

#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* netif + wifi 驱动 + 事件 handler + esp_wifi_start（不连接）。 */
int wifi_manager_init(void);

/* 用运行时配置的 SSID/密码连接并阻塞等 IP（timeout_ms 有界）。
 * 0 = 拿到 IP；<0 = 超时/失败。重复调用会先断开旧连接。 */
int wifi_manager_connect_sta(const char *ssid, const char *password, uint32_t timeout_ms);

/** 当前是否已连接（已拿到 IP） */
bool wifi_manager_is_connected(void);

#ifdef __cplusplus
}
#endif
