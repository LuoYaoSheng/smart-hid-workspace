/**
 * wifi_manager.h — Wi-Fi STA 模式
 *
 * F1/F2：SSID/Password 来自 Kconfig（固定）。
 * F3+：改由 BLE Provisioning 写 NVS namespace=network 后从这里读取。
 *
 * Fail-safe：Wi-Fi 断开 → 触发 hid_engine_release_all()（弱符号 hook）。
 */
#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

int wifi_manager_init(void);

/** 当前是否已连接（已拿到 IP） */
bool wifi_manager_is_connected(void);

#ifdef __cplusplus
}
#endif
