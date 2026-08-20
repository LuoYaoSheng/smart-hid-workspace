/**
 * hid_engine.h — USB Composite HID 报告引擎
 *
 * V1 Composite:
 *   Smart HID
 *   ├── Keyboard HID
 *   └── Mouse HID
 *
 * 核心职责（依据 docs/archive/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md §3-5）：
 *   - keyboard: tap / hotkey / key_down / key_up
 *     · key_down 必须带 lease_ms（lease 超时自动 key_up）
 *   - mouse: move / click / button_down / button_up / wheel
 *     · 大 dx/dy 自动拆多 report（HID 单 report dx/dy 范围 -127~127）
 *     · button_down 必须带 lease_ms
 *   - release_all: 释放所有 pressed keys / modifiers / mouse buttons（fail-safe）
 *
 * MQTT callback 不直接调本组件；command_engine worker task 串行调用。
 */
#pragma once

#include <stdbool.h>
#include <stdint.h>
#include "smart_hid_protocol.h"

#ifdef __cplusplus
extern "C" {
#endif

/**
 * 初始化 USB HID（TinyUSB Composite）。
 *
 * 必须在 TinyUSB 初始化（tusb_init）之后调用。
 * @return 0 成功；非 0 失败
 */
int hid_engine_init(void);

/**
 * 执行一条已解析的 Command（仅 keyboard / mouse / system 三类）。
 *
 * @param cmd  完整解析的 command（command_engine 已完成 dedup/boot_id/ttl 校验）
 * @param exec_ms_out  输出执行耗时（毫秒）
 * @return SMART_HID_CODE_OK 成功；非 0 失败码
 */
int hid_engine_execute(const smart_hid_command_t *cmd, uint32_t *exec_ms_out);

/**
 * 释放所有 pressed keys / modifiers / mouse buttons。
 *
 * fail-safe：任何控制链异常（MQTT disconnect / Wi-Fi disconnect / reboot / lease timeout）必须调用。
 * 幂等：多次调用安全。
 */
void hid_engine_release_all(void);

/**
 * 由定时器/看门狗 task 周期调用，检查并清理过期 lease。
 *
 * @param now_ms  当前时间（毫秒，xTaskGetTickCount * portTICK_PERIOD_MS）
 */
void hid_engine_tick_leases(uint32_t now_ms);

/**
 * USB HID 是否就绪（TinyUSB mounted & configured）。
 */
bool hid_engine_is_ready(void);

#ifdef __cplusplus
}
#endif
