/**
 * provisioning.c — 配网状态机实现（纯编排，无 FreeRTOS/BLE/Wi-Fi 头）
 */
#include "provisioning.h"

#include <string.h>

static prov_adapter_t s_adapt;
static prov_state_t   s_state = PROV_BOOT;
static bool           s_provisioned = false;

int provisioning_init(const prov_adapter_t *adapter) {
    if (adapter == NULL) return -1;
    s_adapt = *adapter;
    s_state = PROV_LOAD_CONFIG;
    return 0;
}

prov_state_t provisioning_state(void) { return s_state; }
bool provisioning_is_provisioned(void) { return s_provisioned; }

static void set_state(prov_state_t st) {
    s_state = st;
    if (st == PROV_READY) s_provisioned = true;
}

static void progress(prov_state_t st, const char *step, prov_error_t err) {
    set_state(st);
    if (s_adapt.on_progress) s_adapt.on_progress(st, step, err);
}

const char *prov_state_str(prov_state_t st) {
    switch (st) {
        case PROV_BOOT:            return "boot";
        case PROV_LOAD_CONFIG:     return "load_config";
        case PROV_UNPROVISIONED:   return "unprovisioned";
        case PROV_PROVISIONING:    return "provisioning";
        case PROV_CONNECTING_WIFI: return "connecting_wifi";
        case PROV_PAIRING:         return "pairing";
        case PROV_CONNECTING_MQTT: return "mqtt_connecting";
        case PROV_READY:           return "ready";
        case PROV_RECOVERY:        return "recovery";
        case PROV_ERROR:           return "error";
        default:                   return "unknown";
    }
}

const char *prov_error_str(prov_error_t err) {
    switch (err) {
        case PROV_ERR_NONE:              return NULL;
        case PROV_ERR_INVALID_PAYLOAD:   return "invalid_payload";
        case PROV_ERR_WIFI_FAILED:       return "wifi_failed";
        case PROV_ERR_HUB_UNREACHABLE:   return "controlhub_unreachable";
        case PROV_ERR_PAIRING_INVALID:   return "pairing_invalid";
        case PROV_ERR_PAIRING_EXPIRED:   return "pairing_expired";
        case PROV_ERR_PAIRING_USED:      return "pairing_used";
        case PROV_ERR_MQTT_INVALID:      return "mqtt_invalid";
        case PROV_ERR_STORAGE_FAILED:    return "storage_failed";
        default:                         return "unknown_error";
    }
}

prov_config_src_t provisioning_boot_decide(runtime_config_t *cfg_out, bool version_corrupt) {
    if (version_corrupt) {
        /* 未知 schema 版本：不按旧结构乱读 → RECOVERY（active 保留只读） */
        if (cfg_out) memset(cfg_out, 0, sizeof(*cfg_out));
        progress(PROV_RECOVERY, "config_version_unknown", PROV_ERR_STORAGE_FAILED);
        return PROV_SRC_NONE;
    }
    int rc = runtime_config_load_active(cfg_out);
    if (rc == RC_OK && runtime_config_validate(cfg_out) == RC_OK && cfg_out->complete) {
        progress(PROV_BOOT, NULL, PROV_ERR_NONE);
        return PROV_SRC_NVS;
    }
    if (rc == RC_ERR_VERSION) {
        progress(PROV_RECOVERY, "config_version_unknown", PROV_ERR_STORAGE_FAILED);
        return PROV_SRC_NONE;
    }
    /* 无配置 / 不完整 → 由调用方决定 DEV_STATIC 或 UNPROVISIONED */
    if (cfg_out) memset(cfg_out, 0, sizeof(*cfg_out));
    progress(PROV_UNPROVISIONED, NULL, PROV_ERR_NONE);
    return PROV_SRC_NONE;
}

prov_state_t provisioning_run_normal(const runtime_config_t *cfg) {
    if (cfg == NULL || !cfg->complete) {
        progress(PROV_RECOVERY, "config_incomplete", PROV_ERR_STORAGE_FAILED);
        return PROV_RECOVERY;
    }

    /* Wi-Fi：有界重试（bounded retry + backoff 由适配器内部处理） */
    int fails = 0;
    while (fails < PROV_WIFI_FAIL_THRESHOLD) {
        progress(PROV_CONNECTING_WIFI, "connecting_wifi", PROV_ERR_NONE);
        if (s_adapt.wifi_connect(cfg->wifi_ssid, cfg->wifi_password, 15000) == 0) break;
        fails++;
    }
    if (fails >= PROV_WIFI_FAIL_THRESHOLD) {
        /* active 配置连不上（Wi-Fi 改过密码等）→ RECOVERY：BLE 开，active 保留 */
        progress(PROV_RECOVERY, "wifi_failed", PROV_ERR_WIFI_FAILED);
        return PROV_RECOVERY;
    }

    /* MQTT：等待连接 */
    progress(PROV_CONNECTING_MQTT, "mqtt_connecting", PROV_ERR_NONE);
    if (s_adapt.mqtt_start(cfg->mqtt_host, cfg->mqtt_port,
                           cfg->mqtt_username, cfg->mqtt_password, 15000) != 0) {
        progress(PROV_RECOVERY, "mqtt_invalid", PROV_ERR_MQTT_INVALID);
        return PROV_RECOVERY;
    }

    progress(PROV_READY, "ready", PROV_ERR_NONE);
    return PROV_READY;
}

prov_state_t provisioning_process_candidate(const runtime_candidate_t *cand) {
    if (cand == NULL) {
        progress(PROV_PROVISIONING, "invalid_payload", PROV_ERR_INVALID_PAYLOAD);
        return PROV_PROVISIONING;
    }

    progress(PROV_PROVISIONING, "received", PROV_ERR_NONE);

    /* 1) 校验 + stage pending（active 不动） */
    if (runtime_config_validate_candidate(cand) != RC_OK) {
        progress(PROV_PROVISIONING, "invalid_payload", PROV_ERR_INVALID_PAYLOAD);
        return PROV_PROVISIONING;
    }
    if (runtime_config_stage_pending(cand) != RC_OK) {
        progress(PROV_PROVISIONING, "storage_failed", PROV_ERR_STORAGE_FAILED);
        return PROV_PROVISIONING;
    }

    /* 2) Wi-Fi（candidate 的 SSID/密码） */
    progress(PROV_CONNECTING_WIFI, "connecting_wifi", PROV_ERR_NONE);
    if (s_adapt.wifi_connect(cand->wifi_ssid, cand->wifi_password, 15000) != 0) {
        (void)runtime_config_discard_pending(); /* 失败：丢 pending，active 不动 */
        progress(PROV_PROVISIONING, "wifi_failed", PROV_ERR_WIFI_FAILED);
        return PROV_PROVISIONING;
    }
    progress(PROV_CONNECTING_WIFI, "wifi_connected", PROV_ERR_NONE);

    /* 3) ControlHub pairing */
    progress(PROV_PAIRING, "pairing", PROV_ERR_NONE);
    prov_creds_t creds = {0};
    int prc = s_adapt.hub_pair(cand, &creds);
    if (prc != 0) {
        (void)runtime_config_discard_pending();
        prov_error_t err;
        switch (prc) {
            case -404: err = PROV_ERR_PAIRING_INVALID; break;
            case -410: err = PROV_ERR_PAIRING_EXPIRED; break;
            case -409: err = PROV_ERR_PAIRING_USED;    break;
            default:   err = PROV_ERR_HUB_UNREACHABLE; break;
        }
        const char *step = (err == PROV_ERR_HUB_UNREACHABLE) ? "controlhub_unreachable"
                        : (err == PROV_ERR_PAIRING_INVALID)  ? "pairing_invalid"
                        : (err == PROV_ERR_PAIRING_EXPIRED)  ? "pairing_expired"
                        : "pairing_used";
        progress(PROV_PROVISIONING, step, err);
        return PROV_PROVISIONING;
    }
    progress(PROV_PAIRING, "pairing_success", PROV_ERR_NONE);

    /* 4) 凭据先落盘（complete=1）——crash C 窗口在此之后关闭 */
    if (runtime_config_pending_set_creds(creds.mqtt_host, creds.mqtt_port,
                                         creds.mqtt_username, creds.mqtt_password) != RC_OK) {
        /* token 已消费但凭据没持久化：pending 里还有 token，重试用同一 token 会 409。
         * ControlHub 侧凭据已签发；设备侧唯一路径是重新扫码新 token。 */
        progress(PROV_PROVISIONING, "storage_failed", PROV_ERR_STORAGE_FAILED);
        return PROV_PROVISIONING;
    }

    /* 5) promote（active = candidate + creds；generation+1） */
    if (runtime_config_promote_pending() != RC_OK) {
        progress(PROV_PROVISIONING, "storage_failed", PROV_ERR_STORAGE_FAILED);
        return PROV_PROVISIONING;
    }

    /* 6) MQTT */
    progress(PROV_CONNECTING_MQTT, "mqtt_connecting", PROV_ERR_NONE);
    if (s_adapt.mqtt_start(creds.mqtt_host, creds.mqtt_port,
                           creds.mqtt_username, creds.mqtt_password, 15000) != 0) {
        /* active 已是新凭据；MQTT 连不上 → RECOVERY（BLE 可见，可重配）。
         * 不回退 active：服务端凭据已轮换，旧凭据已失效。 */
        progress(PROV_RECOVERY, "mqtt_invalid", PROV_ERR_MQTT_INVALID);
        return PROV_RECOVERY;
    }

    progress(PROV_READY, "ready", PROV_ERR_NONE);
    return PROV_READY;
}
