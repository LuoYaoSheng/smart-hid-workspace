/**
 * command_engine_publisher.h — 发布回调（避免组件循环依赖）
 *
 * command_engine 需要 publish ack/event，但不直接依赖 mqtt_manager
 * （否则 mqtt_manager → command_engine → mqtt_manager 循环）。
 *
 * 由 app_main 在 init 后通过 command_engine_set_publishers 装配。
 */
#pragma once

#include <stddef.h>
#include "smart_hid_protocol.h"

#ifdef __cplusplus
extern "C" {
#endif

/* 发布 ACK（QoS1，retain=false） */
typedef void (*ce_publish_ack_fn)(const smart_hid_ack_t *ack);
/* 发布 Event（QoS1，retain=false） */
typedef void (*ce_publish_event_fn)(const smart_hid_event_t *ev);

void command_engine_set_publishers(ce_publish_ack_fn ack_fn, ce_publish_event_fn event_fn);

#ifdef __cplusplus
}
#endif
