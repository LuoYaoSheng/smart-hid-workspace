/**
 * ble_provision.h — BLE Provisioning GATT 服务（NimBLE，M1-G3）
 *
 * UUID / 分帧 / JSON 契约见 protocols/ble/PROVISIONING_V1.md（canonical）。
 * 安全：bonding + LE Secure Connections（Just Works——无 IO 能力故无 MITM 抗性，
 * 如实记录，见协议文档 Security 节）；provision 写特征要求加密链路。
 */
#ifndef BLE_PROVISION_H
#define BLE_PROVISION_H

#include <stdbool.h>
#include <stdint.h>

#include "ble_proto.h"
#include "provisioning.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Canonical UUID（128-bit，base 9f1d10xx-e73b-4c8f-9d2a-6f0b5e8a1c04） */
#define BLE_PROV_UUID_SVC     "9f1d1001-e73b-4c8f-9d2a-6f0b5e8a1c04"
#define BLE_PROV_UUID_INFO    "9f1d1002-e73b-4c8f-9d2a-6f0b5e8a1c04"
#define BLE_PROV_UUID_WRITE   "9f1d1003-e73b-4c8f-9d2a-6f0b5e8a1c04"
#define BLE_PROV_UUID_STATUS  "9f1d1004-e73b-4c8f-9d2a-6f0b5e8a1c04"

/* candidate 完整回调（main 注册：入队转 provisioning worker；BLE 栈回调上下文） */
typedef void (*ble_provision_candidate_cb)(const runtime_candidate_t *cand);

/* 初始化 NimBLE host + GATT（不广播；由 set_advertising 控制） */
int ble_provision_init(const char *device_name, ble_provision_candidate_cb on_candidate);

/* 开/关可连接广播（Provision/Recovery 态开；READY 后关） */
void ble_provision_set_advertising(bool on);

/* 推送状态到 status char（notify）并刷新 info char 读值 */
void ble_provision_publish(prov_state_t st, const char *step, prov_error_t err);

/* NimBLE host 任务栈深（默认够用；显式声明便于调优） */
#define BLE_PROV_HOST_TASK_STACK 4096

#ifdef __cplusplus
}
#endif
#endif /* BLE_PROVISION_H */
