/**
 * provisioning.h — 首次配网 / 正常启动 / 重配网恢复状态机（M1-G3）
 *
 * 状态（protocols/ble/PROVISIONING_V1.md 为契约事实源）：
 *
 *   BOOT → LOAD_CONFIG ─┬─ valid active ──→ CONNECTING_WIFI → CONNECTING_MQTT → READY
 *                       ├─ no config ─────→ UNPROVISIONED → PROVISIONING（BLE 开）
 *                       └─ version/corrupt → RECOVERY（BLE 开，active 只读）
 *
 *   PROVISIONING：BLE candidate → validate → stage pending → CONNECTING_WIFI
 *     → PAIRING（hub HTTP）→ 凭据补全 pending（complete=1，先落盘！crash C 窗口）
 *     → promote → CONNECTING_MQTT → READY
 *     失败：discard pending（active 不动）→ 回 PROVISIONING 报 error
 *
 *   RECOVERY：active 连续失败达阈值 → BLE Provision 开（active 保留），
 *     新 candidate 走同一 provision 流程——只有 pairing 成功才替换 active。
 *
 * 本组件是纯编排层（不 include FreeRTOS/NimBLE/Wi-Fi 头），适配器注入：
 * host 测试可全量驱动（spec §32 Host-side Testability）。
 * 并发（BLE 回调 → 队列 → worker）由 main.c 负责。
 */
#ifndef PROVISIONING_H
#define PROVISIONING_H

#include "runtime_config.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef enum {
    PROV_BOOT = 0,
    PROV_LOAD_CONFIG,
    PROV_UNPROVISIONED,
    PROV_PROVISIONING,
    PROV_CONNECTING_WIFI,
    PROV_PAIRING,
    PROV_CONNECTING_MQTT,
    PROV_READY,
    PROV_RECOVERY,
    PROV_ERROR,
} prov_state_t;

const char *prov_state_str(prov_state_t st);

/* 稳定错误码（BLE status 上报字符串，见 prov_error_str；勿改值） */
typedef enum {
    PROV_ERR_NONE = 0,
    PROV_ERR_INVALID_PAYLOAD = 1,   /* BLE candidate JSON/字段无效 */
    PROV_ERR_WIFI_FAILED = 2,
    PROV_ERR_HUB_UNREACHABLE = 3,   /* pairing endpoint 网络不可达 / 5xx */
    PROV_ERR_PAIRING_INVALID = 4,   /* HTTP 404 token 不存在 */
    PROV_ERR_PAIRING_EXPIRED = 5,   /* HTTP 410 */
    PROV_ERR_PAIRING_USED = 6,      /* HTTP 409 */
    PROV_ERR_MQTT_INVALID = 7,      /* MQTT 连接失败（凭据/端点） */
    PROV_ERR_STORAGE_FAILED = 8,    /* NVS 写失败 */
} prov_error_t;

const char *prov_error_str(prov_error_t err);

/* pairing 响应（hub_pairing 适配器产出） */
typedef struct {
    char    mqtt_host[RC_HOST_MAX];
    uint16_t mqtt_port;
    char    mqtt_username[RC_USER_MAX];
    char    mqtt_password[RC_PASS_MAX];
} prov_creds_t;

/* 适配器：真实实现见 main.c（wifi_manager / hub_pairing / mqtt_manager 包装）；
 * host 测试注入假实现。全部阻塞式，返回 0 成功。 */
typedef struct prov_adapter {
    /* 连 Wi-Fi（阻塞至拿到 IP 或超时）。返回 0=ok，<0=失败 */
    int (*wifi_connect)(const char *ssid, const char *password, uint32_t timeout_ms);
    /* 调 ControlHub pairing。成功填 creds；失败返回 -HTTP_status（-404/-410/-409）
     * 或 -1（网络不可达）。 */
    int (*hub_pair)(const runtime_candidate_t *cand, prov_creds_t *creds_out);
    /* 用给定凭据启动 MQTT 并等连接（阻塞有界）。0=ok */
    int (*mqtt_start)(const char *host, uint16_t port,
                      const char *username, const char *password, uint32_t timeout_ms);
    /* 状态推送（BLE status notify / 日志 / LED）。step 是过渡步骤名（可为 NULL） */
    void (*on_progress)(prov_state_t st, const char *step, prov_error_t err);
} prov_adapter_t;

/* 配置来源（boot 决策用） */
typedef enum {
    PROV_SRC_NONE = 0,      /* 无配置 → Provision Mode */
    PROV_SRC_NVS = 1,       /* NVS active（生产路径） */
    PROV_SRC_DEV_STATIC = 2,/* DEV 静态 Kconfig（仅开发，绝不写 NVS） */
} prov_config_src_t;

int provisioning_init(const prov_adapter_t *adapter);

/* boot 决策：boot_reconcile 已由调用方执行。返回应走的来源 + 填 cfg。 */
prov_config_src_t provisioning_boot_decide(runtime_config_t *cfg_out, bool version_corrupt);

/* 正常启动路径（active / dev-static）：Wi-Fi 有界重试 → MQTT 有界等待。
 * 返回终态：PROV_READY / PROV_RECOVERY。 */
prov_state_t provisioning_run_normal(const runtime_config_t *cfg);

/* 完整 provision 尝试（BLE candidate 触发；阻塞；线程模型由调用方负责）。
 * 返回终态；失败时 active 保持原样。 */
prov_state_t provisioning_process_candidate(const runtime_candidate_t *cand);

/* 当前状态（BLE info/status 用；只读原子性由单写者保证——prov worker） */
prov_state_t provisioning_state(void);
bool provisioning_is_provisioned(void);

/* RECOVERY 阈值：正常路径 Wi-Fi 连续失败次数达到该值进 RECOVERY */
#define PROV_WIFI_FAIL_THRESHOLD 5

#ifdef __cplusplus
}
#endif
#endif /* PROVISIONING_H */
