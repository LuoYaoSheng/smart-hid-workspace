/**
 * command_engine.h — Command Queue / Dedup / Boot ID / TTL / Worker
 *
 * 依据：docs/archive/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md §6-9 / §15
 *
 * 处理路径：
 *   MQTT RX → parse → dedup → boot_id 校验 → TTL 校验 → enqueue
 *                                                              ↓
 *                                                          worker task
 *                                                              ↓
 *                                                          hid_engine
 *                                                              ↓
 *                                                        publish ack
 *
 * 关键约束：
 *   - MQTT callback 不直接调 hid_engine，必须走 queue（§6）
 *   - queue size 32, bounded, serial execution（§7）
 *   - dedup 256 request_id RAM 缓存，同 id 第二次返回 duplicate（§9）
 *   - target_boot_id 不匹配 → rejected(STALE_DEVICE_SESSION)（§8）
 *   - ttl_ms 过期 → expired
 */
#pragma once

#include <stdint.h>
#include <stdbool.h>
#include <stddef.h>
#include "smart_hid_protocol.h"

#ifdef __cplusplus
extern "C" {
#endif

/**
 * 初始化 command_engine：创建 queue、worker task、lease tick task。
 * 依赖：device_identity 已 init、hid_engine 已 init。
 *
 * @return 0 成功；非 0 失败
 */
int command_engine_init(void);

/**
 * 投递收到的 MQTT command payload（由 mqtt_manager 在 command topic handler 调用）。
 *
 * 同步返回的 ack_t 表示已确定的即时结果（duplicate / rejected-stale /
 * rejected-bad / rejected-queue_full）；执行类（executed/expired）由 worker
 * 异步处理，通过 command_engine_set_ack_publisher 推到 MQTT。
 *
 * @param topic       收到时的 topic（仅用于日志）
 * @param payload     MQTT payload 字节
 * @param payload_len 字节长度
 * @param immediate_ack_out 输出：即时 ack（duplicate / rejected / etc），由调用方直接 publish
 * @return true 表示已填 immediate_ack_out（需立即 publish）；
 *         false 表示已入队，由 worker 异步 publish ack
 */
bool command_engine_handle_raw(const char *topic,
                               const char *payload, size_t payload_len,
                               smart_hid_ack_t *immediate_ack_out);

#ifdef __cplusplus
}
#endif
