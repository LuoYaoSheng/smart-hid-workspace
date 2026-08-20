/**
 * mqtt_manager.h — MQTT 连接 + topic 订阅 + publish
 *
 * 依据：docs/archive/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md §6
 *       docs/archive/04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md
 *
 * 订阅：smart-hid/v1/devices/{device_id}/command   QoS1
 * 发布：ack（QoS1 retain=false）/ status（QoS1 retain=true）/ event（QoS1 retain=false）
 *
 * LWT（Last Will Testament）：发布 online=false status（retain=true），
 *                            ControlHub 启动时读 retained status 重建在线视图。
 *
 * Fail-safe：MQTT 断开 → command_engine 触发 hid_engine_release_all()。
 */
#pragma once

#include <stdbool.h>
#include <stdint.h>
#include "smart_hid_protocol.h"

#ifdef __cplusplus
extern "C" {
#endif

/**
 * 初始化 MQTT：
 *  - 注册 LWT（online=false，retain=true）
 *  - 连接 broker（broker/user/pass 来自 Kconfig；F3 后由配网写 NVS）
 *  - 连上后订阅 command topic
 *  - 注册收到 command 的回调（→ command_engine）
 *  - MQTT 断开回调（→ 触发 hid_engine_release_all via on_disconnect）
 *
 * @return 0 成功；非 0 失败
 */
int mqtt_manager_init(void);

/** 发布 ACK（QoS1，retain=false） */
void mqtt_manager_publish_ack(const smart_hid_ack_t *ack);

/** 发布 Status（QoS1，retain=true） */
void mqtt_manager_publish_status(const smart_hid_status_t *status);

/** 发布 Event（QoS1，retain=false） */
void mqtt_manager_publish_event(const smart_hid_event_t *ev);

/** 当前 MQTT 是否已连接 */
bool mqtt_manager_is_connected(void);

#ifdef __cplusplus
}
#endif
