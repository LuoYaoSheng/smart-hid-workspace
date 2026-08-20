/**
 * runtime_config.c — NVS 运行时配置实现
 *
 * 存储：两个 namespace —— rt_active（生效配置）/ rt_pending（staging）。
 * 每字段独立 key（字符串/整型）；一次 nvs_commit 批量落盘保证原子性。
 * 崩溃窗口分析：
 *   promote = [写 active 全字段 + commit] → [清 pending + commit]。
 *   若两 commit 之间掉电：active 已新，pending 残留 → boot_reconcile 再次
 *   promote（幂等，同值重写），无死状态。
 */
#include "runtime_config.h"

#include <stdio.h>
#include <string.h>

#include "esp_log.h"
#include "nvs.h"

static const char *TAG = "runtime_config";

#define NS_ACTIVE  "rt_active"
#define NS_PENDING "rt_pending"

/* key 常量（active 与 pending 同名，靠 namespace 区分） */
#define K_VER   "ver"      /* u8  schema_version */
#define K_SSID  "ssid"
#define K_WPASS "wpass"
#define K_HHOST "hhost"    /* pairing endpoint host */
#define K_HPORT "hport"    /* u16 */
#define K_MHOST "mhost"    /* advertised mqtt host */
#define K_MPORT "mport"    /* u16 */
#define K_MUSER "muser"
#define K_MPASS "mpass"
#define K_GEN   "gen"      /* u32 */
#define K_DONE  "done"     /* u8  complete 标记 */
#define K_TOKEN "token"    /* 仅 pending：一次性 pairing token，promote 不复制 */

static void runtime_config_log_simple(const char *tagmsg, const runtime_config_t *cfg);

static nvs_handle_t s_h_active = 0;
static nvs_handle_t s_h_pending = 0;

int runtime_config_init(void) {
    esp_err_t err = nvs_open(NS_ACTIVE, NVS_READWRITE, &s_h_active);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "nvs_open(%s): %s", NS_ACTIVE, esp_err_to_name(err));
        return RC_ERR_NVS;
    }
    err = nvs_open(NS_PENDING, NVS_READWRITE, &s_h_pending);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "nvs_open(%s): %s", NS_PENDING, esp_err_to_name(err));
        return RC_ERR_NVS;
    }
    ESP_LOGI(TAG, "runtime config ready (ns=%s/%s schema=%d)", NS_ACTIVE, NS_PENDING,
             RUNTIME_CONFIG_SCHEMA_VERSION);
    return RC_OK;
}

/* ---- 内部：字段读写（namespace 抽象） ---- */

typedef struct {
    const char *ns;   /* 日志用 */
    nvs_handle_t h;
} ns_ref_t;

static ns_ref_t ns_active(void) { return (ns_ref_t){NS_ACTIVE, s_h_active}; }
static ns_ref_t ns_pending(void) { return (ns_ref_t){NS_PENDING, s_h_pending}; }

static int get_str(const ns_ref_t *ns, const char *key, char *out, size_t outlen) {
    size_t required = 0;
    esp_err_t err = nvs_get_str(ns->h, key, NULL, &required);
    if (err == ESP_ERR_NVS_NOT_FOUND) return RC_ERR_NO_CONFIG;
    if (err != ESP_OK) return RC_ERR_NVS;
    if (required > outlen) return RC_ERR_INVALID; /* 超长按损坏处理 */
    err = nvs_get_str(ns->h, key, out, &required);
    return err == ESP_OK ? RC_OK : RC_ERR_NVS;
}

static int get_u8(const ns_ref_t *ns, const char *key, uint8_t *out) {
    esp_err_t err = nvs_get_u8(ns->h, key, out);
    if (err == ESP_ERR_NVS_NOT_FOUND) return RC_ERR_NO_CONFIG;
    return err == ESP_OK ? RC_OK : RC_ERR_NVS;
}
static int get_u16(const ns_ref_t *ns, const char *key, uint16_t *out) {
    esp_err_t err = nvs_get_u16(ns->h, key, out);
    if (err == ESP_ERR_NVS_NOT_FOUND) return RC_ERR_NO_CONFIG;
    return err == ESP_OK ? RC_OK : RC_ERR_NVS;
}
static int get_u32(const ns_ref_t *ns, const char *key, uint32_t *out) {
    esp_err_t err = nvs_get_u32(ns->h, key, out);
    if (err == ESP_ERR_NVS_NOT_FOUND) return RC_ERR_NO_CONFIG;
    return err == ESP_OK ? RC_OK : RC_ERR_NVS;
}

static int set_str(const ns_ref_t *ns, const char *key, const char *val) {
    return nvs_set_str(ns->h, key, val) == ESP_OK ? RC_OK : RC_ERR_NVS;
}
static int set_u8(const ns_ref_t *ns, const char *key, uint8_t v) {
    return nvs_set_u8(ns->h, key, v) == ESP_OK ? RC_OK : RC_ERR_NVS;
}
static int set_u16(const ns_ref_t *ns, const char *key, uint16_t v) {
    return nvs_set_u16(ns->h, key, v) == ESP_OK ? RC_OK : RC_ERR_NVS;
}
static int set_u32(const ns_ref_t *ns, const char *key, uint32_t v) {
    return nvs_set_u32(ns->h, key, v) == ESP_OK ? RC_OK : RC_ERR_NVS;
}

/* 写一个完整 config 到指定 namespace（不含 token）。 */
static int write_config(const ns_ref_t *ns, const runtime_config_t *cfg) {
    int rc;
    if ((rc = set_u8(ns, K_VER, cfg->schema_version)) != RC_OK) return rc;
    if ((rc = set_str(ns, K_SSID, cfg->wifi_ssid)) != RC_OK) return rc;
    if ((rc = set_str(ns, K_WPASS, cfg->wifi_password)) != RC_OK) return rc;
    if ((rc = set_str(ns, K_HHOST, cfg->hub_host)) != RC_OK) return rc;
    if ((rc = set_u16(ns, K_HPORT, cfg->hub_port)) != RC_OK) return rc;
    if ((rc = set_str(ns, K_MHOST, cfg->mqtt_host)) != RC_OK) return rc;
    if ((rc = set_u16(ns, K_MPORT, cfg->mqtt_port)) != RC_OK) return rc;
    if ((rc = set_str(ns, K_MUSER, cfg->mqtt_username)) != RC_OK) return rc;
    if ((rc = set_str(ns, K_MPASS, cfg->mqtt_password)) != RC_OK) return rc;
    if ((rc = set_u32(ns, K_GEN, cfg->generation)) != RC_OK) return rc;
    if ((rc = set_u8(ns, K_DONE, cfg->complete ? 1 : 0)) != RC_OK) return rc;
    return RC_OK;
}

/* 从指定 namespace 读完整 config。 */
static int read_config(const ns_ref_t *ns, runtime_config_t *out, int missing_code) {
    if (out == NULL) return RC_ERR_ARGS;
    memset(out, 0, sizeof(*out));
    int rc = get_u8(ns, K_VER, &out->schema_version);
    if (rc == RC_ERR_NO_CONFIG) return missing_code;
    if (rc != RC_OK) return rc;
    if (out->schema_version != RUNTIME_CONFIG_SCHEMA_VERSION) {
        ESP_LOGE(TAG, "%s: unknown schema version %d (supported %d) → refuse to misread",
                 ns->ns, out->schema_version, RUNTIME_CONFIG_SCHEMA_VERSION);
        return RC_ERR_VERSION;
    }
    if ((rc = get_str(ns, K_SSID, out->wifi_ssid, sizeof(out->wifi_ssid))) != RC_OK) return rc == RC_ERR_NO_CONFIG ? RC_ERR_INVALID : rc;
    if ((rc = get_str(ns, K_WPASS, out->wifi_password, sizeof(out->wifi_password))) != RC_OK) return rc == RC_ERR_NO_CONFIG ? RC_ERR_INVALID : rc;
    if ((rc = get_str(ns, K_HHOST, out->hub_host, sizeof(out->hub_host))) != RC_OK) return rc == RC_ERR_NO_CONFIG ? RC_ERR_INVALID : rc;
    if ((rc = get_u16(ns, K_HPORT, &out->hub_port)) != RC_OK) return rc == RC_ERR_NO_CONFIG ? RC_ERR_INVALID : rc;
    /* MQTT 字段允许缺失（未 complete 的 pending） */
    get_str(ns, K_MHOST, out->mqtt_host, sizeof(out->mqtt_host));
    get_u16(ns, K_MPORT, &out->mqtt_port);
    get_str(ns, K_MUSER, out->mqtt_username, sizeof(out->mqtt_username));
    get_str(ns, K_MPASS, out->mqtt_password, sizeof(out->mqtt_password));
    uint8_t done = 0;
    get_u8(ns, K_DONE, &done);
    out->complete = done == 1;
    get_u32(ns, K_GEN, &out->generation);
    return RC_OK;
}

/* erase namespace 全部 key */
static int erase_ns(const ns_ref_t *ns) {
    esp_err_t err = nvs_erase_all(ns->h);
    if (err != ESP_OK && err != ESP_ERR_NVS_NOT_FOUND) return RC_ERR_NVS;
    return nvs_commit(ns->h) == ESP_OK ? RC_OK : RC_ERR_NVS;
}

/* ---- 公开 API ---- */

int runtime_config_load_active(runtime_config_t *out) {
    ns_ref_t ns = ns_active();
    return read_config(&ns, out, RC_ERR_NO_CONFIG);
}

int runtime_config_load_pending(runtime_config_t *out) {
    ns_ref_t ns = ns_pending();
    return read_config(&ns, out, RC_ERR_NO_PENDING);
}

int runtime_config_validate(const runtime_config_t *cfg) {
    if (cfg == NULL) return RC_ERR_ARGS;
    if (cfg->wifi_ssid[0] == '\0' || cfg->hub_host[0] == '\0') return RC_ERR_INVALID;
    if (cfg->hub_port == 0 || cfg->hub_port > 65535) return RC_ERR_INVALID;
    if (cfg->complete) {
        if (cfg->mqtt_host[0] == '\0' || cfg->mqtt_username[0] == '\0' ||
            cfg->mqtt_password[0] == '\0' || cfg->mqtt_port == 0) {
            return RC_ERR_INVALID;
        }
    }
    return RC_OK;
}

int runtime_config_validate_candidate(const runtime_candidate_t *cand) {
    if (cand == NULL) return RC_ERR_ARGS;
    if (cand->wifi_ssid[0] == '\0') return RC_ERR_INVALID;
    if (cand->hub_host[0] == '\0') return RC_ERR_INVALID;
    if (cand->token[0] == '\0') return RC_ERR_INVALID;
    if (cand->hub_port == 0 || cand->hub_port > 65535) return RC_ERR_INVALID;
    return RC_OK;
}

int runtime_config_stage_pending(const runtime_candidate_t *cand) {
    if (cand == NULL) return RC_ERR_ARGS;
    int rc = runtime_config_validate_candidate(cand);
    if (rc != RC_OK) return rc;

    runtime_config_t p = {0};
    p.schema_version = RUNTIME_CONFIG_SCHEMA_VERSION;
    strlcpy(p.wifi_ssid, cand->wifi_ssid, sizeof(p.wifi_ssid));
    strlcpy(p.wifi_password, cand->wifi_password, sizeof(p.wifi_password));
    strlcpy(p.hub_host, cand->hub_host, sizeof(p.hub_host));
    p.hub_port = cand->hub_port;
    p.complete = false;
    /* generation 继承 active（promote 时 +1） */
    runtime_config_t act;
    if (runtime_config_load_active(&act) == RC_OK) {
        p.generation = act.generation;
    }

    ns_ref_t ns = ns_pending();
    if ((rc = write_config(&ns, &p)) != RC_OK) return rc;
    /* token 单独存 pending（重试用；promote 不复制到 active） */
    if ((rc = set_str(&ns, K_TOKEN, cand->token)) != RC_OK) return rc;
    if (nvs_commit(ns.h) != ESP_OK) return RC_ERR_NVS;

    runtime_config_log_simple("staged pending", &p);
    return RC_OK;
}

int runtime_config_pending_set_creds(const char *mqtt_host, uint16_t mqtt_port,
                                     const char *username, const char *password) {
    if (mqtt_host == NULL || username == NULL || password == NULL) return RC_ERR_ARGS;
    if (mqtt_host[0] == '\0' || username[0] == '\0' || password[0] == '\0' || mqtt_port == 0) {
        return RC_ERR_INVALID;
    }
    runtime_config_t p;
    int rc = runtime_config_load_pending(&p);
    if (rc != RC_OK) return rc;
    strlcpy(p.mqtt_host, mqtt_host, sizeof(p.mqtt_host));
    p.mqtt_port = mqtt_port;
    strlcpy(p.mqtt_username, username, sizeof(p.mqtt_username));
    strlcpy(p.mqtt_password, password, sizeof(p.mqtt_password));
    p.complete = true;
    if ((rc = runtime_config_validate(&p)) != RC_OK) return rc;

    ns_ref_t ns = ns_pending();
    if ((rc = write_config(&ns, &p)) != RC_OK) return rc;
    if (nvs_commit(ns.h) != ESP_OK) return RC_ERR_NVS;

    ESP_LOGI(TAG, "pending creds persisted (mqtt_host=%s port=%u user=%s) complete=1",
             p.mqtt_host, p.mqtt_port, p.mqtt_username); /* 密码绝不打日志 */
    return RC_OK;
}

int runtime_config_promote_pending(void) {
    runtime_config_t p;
    int rc = runtime_config_load_pending(&p);
    if (rc == RC_ERR_NO_PENDING) return RC_ERR_NO_PENDING;
    if (rc != RC_OK) return rc;
    if (!p.complete) {
        ESP_LOGE(TAG, "promote refused: pending not complete");
        return RC_ERR_INVALID;
    }
    if ((rc = runtime_config_validate(&p)) != RC_OK) return rc;

    p.generation += 1;
    ns_ref_t act = ns_active();
    if ((rc = write_config(&act, &p)) != RC_OK) return rc;
    if (nvs_commit(act.h) != ESP_OK) return RC_ERR_NVS;

    /* active 已落盘；清 pending（含 token）。两 commit 间掉电 → 幂等重放 */
    ns_ref_t pend = ns_pending();
    erase_ns(&pend);

    ESP_LOGI(TAG, "promoted pending → active (generation=%lu)", (unsigned long)p.generation);
    return RC_OK;
}

int runtime_config_discard_pending(void) {
    ns_ref_t ns = ns_pending();
    return erase_ns(&ns);
}

int runtime_config_boot_reconcile(bool *promoted) {
    if (promoted) *promoted = false;
    runtime_config_t p;
    int rc = runtime_config_load_pending(&p);
    if (rc == RC_ERR_NO_PENDING) return RC_OK;
    if (rc != RC_OK) return rc; /* 版本未知等 → 交给上层进 Recovery */
    if (!p.complete) {
        /* 半截 pending（crash A/B）：无凭据可恢复，直接丢弃，走 active/UNPROVISIONED */
        ESP_LOGW(TAG, "incomplete pending at boot → discard");
        return runtime_config_discard_pending();
    }
    /* crash C/D：凭据已持久化 + token 已消费 → promote 是唯一无死状态路径 */
    ESP_LOGW(TAG, "complete pending at boot → promote (crash recovery)");
    rc = runtime_config_promote_pending();
    if (rc == RC_OK && promoted) *promoted = true;
    return rc;
}

int runtime_config_clear(void) {
    ns_ref_t act = ns_active();
    int rc = erase_ns(&act);
    if (rc != RC_OK) return rc;
    ns_ref_t pend = ns_pending();
    rc = erase_ns(&pend);
    ESP_LOGW(TAG, "runtime config cleared (factory reset capability)");
    return rc;
}

const char *runtime_config_strerror(int err) {
    switch (err) {
        case RC_OK:            return "ok";
        case RC_ERR_NO_CONFIG: return "no active config";
        case RC_ERR_INVALID:   return "invalid config fields";
        case RC_ERR_VERSION:   return "unknown schema version";
        case RC_ERR_NVS:       return "nvs error";
        case RC_ERR_ARGS:      return "bad args";
        case RC_ERR_NO_PENDING:return "no pending config";
        default:               return "unknown";
    }
}

void runtime_config_log_fields(char *buf, size_t buflen, const runtime_config_t *cfg) {
    if (buf == NULL || buflen == 0) return;
    if (cfg == NULL) {
        snprintf(buf, buflen, "cfg=NULL");
        return;
    }
    snprintf(buf, buflen,
             "schema=%u gen=%lu ssid=%s hub=%s:%u mqtt=%s:%u user=%s complete=%d",
             cfg->schema_version, (unsigned long)cfg->generation,
             cfg->wifi_ssid, cfg->hub_host, cfg->hub_port,
             cfg->mqtt_host[0] ? cfg->mqtt_host : "-", cfg->mqtt_port,
             cfg->mqtt_username[0] ? cfg->mqtt_username : "-", cfg->complete ? 1 : 0);
}

/* ESP_LOG 不进 host 测试；单独包一层供 stage 路径复用 */
static void runtime_config_log_simple(const char *tagmsg, const runtime_config_t *cfg) {
    char buf[256];
    runtime_config_log_fields(buf, sizeof(buf), cfg);
    ESP_LOGI(TAG, "%s: %s", tagmsg, buf); /* buf 无密码字段（见 log_fields） */
}
