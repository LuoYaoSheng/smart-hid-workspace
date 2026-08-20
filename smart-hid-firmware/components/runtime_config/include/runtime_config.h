/**
 * runtime_config.h — 设备运行时配置（M1-G3）
 *
 * 事实源：NVS（namespace rt_active / rt_pending）。Kconfig 网络参数仅作为
 * 显式 DEV 静态开发模式（CONFIG_SMART_HID_DEV_STATIC_CONFIG）的 fallback，
 * 绝不覆盖 NVS。
 *
 * Active / Pending 模型（防止配网失败把设备变砖）：
 *   BLE candidate → stage pending → Wi-Fi → Pairing → 凭据补全置 complete
 *   → promote 为 active（单次 commit）；失败 discard pending，active 不动。
 *
 * Crash 恢复规则（provisioning 状态机在 boot 时调用 boot_reconcile）：
 *   pending 存在且 complete → 直接 promote（token 已在 ControlHub 消费，
 *   凭据已持久化，这是唯一无死状态的恢复路径）。
 *
 * schema_version 守卫：读到未知未来版本 → RC_ERR_VERSION（进 Recovery/
 * Provision，不按旧结构乱读）。
 */
#ifndef RUNTIME_CONFIG_H
#define RUNTIME_CONFIG_H

#include <stddef.h>
#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

#define RUNTIME_CONFIG_SCHEMA_VERSION 1

/* 字段容量（与 Wi-Fi/MQTT 协议上限对齐；NUL 结尾含在内） */
#define RC_SSID_MAX     33   /* 802.11 SSID ≤32 字节 */
#define RC_PASS_MAX     65   /* WPA2 ≤64；MQTT 凭据 64 hex */
#define RC_HOST_MAX     64
#define RC_USER_MAX     24   /* dev_HID-XXXXXXXX */
#define RC_TOKEN_MAX    65   /* pairing token 32 hex */

/* 错误码（稳定，勿改值） */
enum {
    RC_OK = 0,
    RC_ERR_NO_CONFIG = 1,   /* 无 active 配置（正常新机状态） */
    RC_ERR_INVALID   = 2,   /* 字段校验失败 */
    RC_ERR_VERSION   = 3,   /* 未知 schema 版本（未来固件写入的配置） */
    RC_ERR_NVS       = 4,   /* NVS 读写失败 */
    RC_ERR_ARGS      = 5,   /* 参数为空 */
    RC_ERR_NO_PENDING = 6,  /* 无 pending 配置 */
};

/* 运行时配置（wifi + hub pairing endpoint + pairing 响应的 MQTT 凭据） */
typedef struct {
    uint8_t schema_version;                 /* 写入时 = RUNTIME_CONFIG_SCHEMA_VERSION */
    char    wifi_ssid[RC_SSID_MAX];
    char    wifi_password[RC_PASS_MAX];
    char    hub_host[RC_HOST_MAX];          /* ControlHub pairing endpoint（QR/scan 提供） */
    uint16_t hub_port;                      /* 默认 17892 */
    char    mqtt_host[RC_HOST_MAX];         /* pairing 响应的 advertised broker（§29 设备不自己猜） */
    uint16_t mqtt_port;
    char    mqtt_username[RC_USER_MAX];     /* dev_<device_id> */
    char    mqtt_password[RC_PASS_MAX];
    uint32_t generation;                    /* 每次 promote +1 */
    bool    complete;                       /* pairing 成功 + MQTT 凭据齐（promote 前置条件） */
} runtime_config_t;

/* pairing 前的 candidate（BLE 下发；MQTT 字段空） */
typedef struct {
    char    wifi_ssid[RC_SSID_MAX];
    char    wifi_password[RC_PASS_MAX];
    char    hub_host[RC_HOST_MAX];
    uint16_t hub_port;
    char    token[RC_TOKEN_MAX];            /* 一次性，绝不持久化到 active */
} runtime_candidate_t;

int runtime_config_init(void);

/* active 读取。RC_ERR_NO_CONFIG = 新机；RC_ERR_VERSION = 未知版本（→ Recovery） */
int runtime_config_load_active(runtime_config_t *out);

/* pending 读取（不存在 → RC_ERR_NO_PENDING） */
int runtime_config_load_pending(runtime_config_t *out);

/* candidate → pending（不动 active；schema_version/generation 由本函数填写） */
int runtime_config_stage_pending(const runtime_candidate_t *cand);

/* pairing 成功后把 MQTT 凭据补进 pending 并置 complete（token 不写入） */
int runtime_config_pending_set_creds(const char *mqtt_host, uint16_t mqtt_port,
                                     const char *username, const char *password);

/* pending → active（要求 complete；单 commit 原子替换；generation+1；清 pending） */
int runtime_config_promote_pending(void);

/* 丢弃 pending（active 不动） */
int runtime_config_discard_pending(void);

/* boot 恢复：pending complete → promote（幂等）；返回是否有 promote 发生 */
int runtime_config_boot_reconcile(bool *promoted);

/* 清空 active + pending（factory reset 底层能力；不暴露任何远程触发，见
 * docs/current/ACCEPTANCE 硬件验收登记） */
int runtime_config_clear(void);

/* 字段校验：wifi 字段必填；complete 时 MQTT 字段必填、端口范围合法 */
int runtime_config_validate(const runtime_config_t *cfg);

/* candidate 字段校验（SSID/host/token 非空、端口范围） */
int runtime_config_validate_candidate(const runtime_candidate_t *cand);

const char *runtime_config_strerror(int err);

/* 日志辅助：只含非敏感字段（ssid/host/port/generation/state）；密码类绝不出现 */
void runtime_config_log_fields(char *buf, size_t buflen, const runtime_config_t *cfg);

#ifdef __cplusplus
}
#endif
#endif /* RUNTIME_CONFIG_H */
