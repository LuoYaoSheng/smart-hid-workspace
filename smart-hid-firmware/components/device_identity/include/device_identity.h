/**
 * device_identity.h — Device ID + Boot ID 管理
 *
 * Device ID：首次烧录由 Kconfig 提供默认值，写入 NVS namespace=identity。
 *            普通复位不清除。工厂烧录工具可写入正式 ID（F5）。
 * Boot ID：每次启动新生成（B-XXXXXX，hex）。Command 中 target_boot_id 必须匹配。
 *
 * 依据：docs/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md §8 / §11
 */
#pragma once

#include <stdbool.h>
#include "smart_hid_protocol.h"

#ifdef __cplusplus
extern "C" {
#endif

/**
 * 初始化 device_identity：
 *  - 从 NVS 读 device_id；若无则用 fallback_default 写入并持久化。
 *  - 生成新 boot_id（本次启动会话）。
 *
 * @param fallback_default  Kconfig SMART_HID_DEVICE_ID（NVS 未命中时用）
 * @return 0 成功；非 0 失败（NVS 错误码）
 */
int device_identity_init(const char *fallback_default);

/** 当前 device_id（init 后有效） */
const char *device_identity_get_device_id(void);

/** 本次启动 boot_id（init 后有效） */
const char *device_identity_get_boot_id(void);

/** 固件版本字符串 */
const char *device_identity_get_firmware(void);

#ifdef __cplusplus
}
#endif
