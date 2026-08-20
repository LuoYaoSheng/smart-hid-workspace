/**
 * test_provisioning.c — 配网状态机 host 单测（fake adapters；spec M1-G3 §39）
 */
#include "test_framework.h"
#include "provisioning.h"
#include "runtime_config.h"
#include "nvs.h"

#include <string.h>

/* ---- fake adapter ---- */
typedef struct {
    int wifi_rc;
    int pair_rc;          /* 0 ok；-404/-409/-410/-1 */
    int mqtt_rc;
    prov_creds_t creds;   /* pair 成功时返回 */
    int wifi_calls, pair_calls, mqtt_calls;
    prov_state_t last_state;
    prov_error_t last_err;
    char last_step[32];
} fake_t;

static fake_t g_fake;

static int f_wifi(const char *ssid, const char *password, uint32_t t) {
    (void)ssid; (void)password; (void)t;
    g_fake.wifi_calls++;
    return g_fake.wifi_rc;
}
static int f_pair(const runtime_candidate_t *c, prov_creds_t *out) {
    (void)c;
    g_fake.pair_calls++;
    if (g_fake.pair_rc == 0) *out = g_fake.creds;
    return g_fake.pair_rc;
}
static int f_mqtt(const char *h, uint16_t p, const char *u, const char *pw, uint32_t t) {
    (void)h; (void)p; (void)u; (void)pw; (void)t;
    g_fake.mqtt_calls++;
    return g_fake.mqtt_rc;
}
static void f_progress(prov_state_t st, const char *step, prov_error_t err) {
    g_fake.last_state = st;
    g_fake.last_err = err;
    snprintf(g_fake.last_step, sizeof(g_fake.last_step), "%s", step ? step : "-");
}
static const prov_adapter_t g_fake_adapter = {
    .wifi_connect = f_wifi, .hub_pair = f_pair, .mqtt_start = f_mqtt, .on_progress = f_progress,
};

static void reset_all(void) {
    nvs_stub_reset();
    memset(&g_fake, 0, sizeof(g_fake));
    runtime_config_init();
    provisioning_init(&g_fake_adapter);
    snprintf(g_fake.creds.mqtt_host, sizeof(g_fake.creds.mqtt_host), "%s", "192.168.1.8");
    g_fake.creds.mqtt_port = 17891;
    snprintf(g_fake.creds.mqtt_username, sizeof(g_fake.creds.mqtt_username), "%s", "dev_HID-AAAA0001");
    snprintf(g_fake.creds.mqtt_password, sizeof(g_fake.creds.mqtt_password), "%s", "cred-hex");
}

static runtime_candidate_t cand(const char *ssid) {
    runtime_candidate_t c = {0};
    snprintf(c.wifi_ssid, sizeof(c.wifi_ssid), "%s", ssid);
    snprintf(c.wifi_password, sizeof(c.wifi_password), "%s", "wifi-pass");
    snprintf(c.hub_host, sizeof(c.hub_host), "%s", "192.168.1.8");
    c.hub_port = 17892;
    snprintf(c.token, sizeof(c.token), "%s", "tok-32-hex-0000000000000000");
    return c;
}



static void stage_cand(const char *ssid) {
    runtime_candidate_t c = cand(ssid);
    (void)runtime_config_stage_pending(&c);
}

static prov_state_t process_cand(const char *ssid) {
    runtime_candidate_t c = cand(ssid);
    return provisioning_process_candidate(&c);
}

/* 预置一份完整 active（gen=1, ssid=oldnet） */
static void seed_active(void) {
    stage_cand("oldnet");
    runtime_config_pending_set_creds("192.168.1.8", 17891, "dev_HID-OLD00001", "old-cred");
    runtime_config_promote_pending();
}

/* ---- boot 决策 ---- */

void test_prov_no_config_enters_provisioning(void) {
    reset_all();
    runtime_config_t cfg;
    prov_config_src_t src = provisioning_boot_decide(&cfg, false);
    CHECK(src == PROV_SRC_NONE, "no config → SRC_NONE");
    CHECK(provisioning_state() == PROV_UNPROVISIONED, "state UNPROVISIONED");
}

void test_prov_valid_config_enters_normal_boot(void) {
    reset_all();
    seed_active();
    runtime_config_t cfg;
    prov_config_src_t src = provisioning_boot_decide(&cfg, false);
    CHECK(src == PROV_SRC_NVS, "valid active → SRC_NVS");
    CHECK(strcmp(cfg.wifi_ssid, "oldnet") == 0, "cfg from NVS");
}

void test_prov_corrupt_config_enters_recovery(void) {
    reset_all();
    seed_active();
    nvs_stub_raw_set_u8("rt_active", "ver", 42); /* 未知版本 */
    runtime_config_t cfg;
    prov_config_src_t src = provisioning_boot_decide(&cfg, false);
    CHECK(src == PROV_SRC_NONE, "corrupt → SRC_NONE");
    CHECK(provisioning_state() == PROV_RECOVERY, "state RECOVERY (BLE provisioning visible)");
}

void test_prov_run_normal_reaches_ready(void) {
    reset_all();
    seed_active();
    runtime_config_t cfg;
    provisioning_boot_decide(&cfg, false);
    g_fake.wifi_rc = 0;
    g_fake.mqtt_rc = 0;
    prov_state_t st = provisioning_run_normal(&cfg);
    CHECK(st == PROV_READY, "happy path READY");
    CHECK(g_fake.wifi_calls == 1 && g_fake.mqtt_calls == 1, "adapters each called once");
    CHECK(provisioning_is_provisioned(), "provisioned flag set");
}

void test_prov_run_normal_wifi_fail_enters_recovery(void) {
    reset_all();
    seed_active();
    runtime_config_t cfg;
    provisioning_boot_decide(&cfg, false);
    g_fake.wifi_rc = -1;
    prov_state_t st = provisioning_run_normal(&cfg);
    CHECK(st == PROV_RECOVERY, "wifi persistently failing → RECOVERY");
    CHECK(g_fake.wifi_calls == PROV_WIFI_FAIL_THRESHOLD, "bounded retries (no infinite silent loop)");
    CHECK(g_fake.last_err == PROV_ERR_WIFI_FAILED, "error surfaced");
}

/* ---- candidate 流程 ---- */

void test_prov_candidate_full_success_commits(void) {
    reset_all();
    seed_active();
    prov_state_t st = process_cand("newnet");
    CHECK(st == PROV_READY, "candidate → READY");
    runtime_config_t act;
    runtime_config_load_active(&act);
    CHECK(strcmp(act.wifi_ssid, "newnet") == 0, "active = new config");
    CHECK(strcmp(act.mqtt_username, "dev_HID-AAAA0001") == 0, "active = new creds");
    CHECK(act.generation == 2, "generation incremented");
    CHECK(strcmp(g_fake.last_step, "ready") == 0, "final step ready");
}

void test_prov_wifi_failure_keeps_old_active(void) {
    reset_all();
    seed_active();
    g_fake.wifi_rc = -1;
    prov_state_t st = process_cand("badnet");
    CHECK(st == PROV_PROVISIONING, "back to provisioning for retry");
    runtime_config_t act;
    runtime_config_load_active(&act);
    CHECK(strcmp(act.wifi_ssid, "oldnet") == 0, "old active kept");
    CHECK(act.generation == 1, "generation unchanged");
    runtime_config_t p;
    CHECK(runtime_config_load_pending(&p) == RC_ERR_NO_PENDING, "failed pending discarded");
    CHECK(g_fake.last_err == PROV_ERR_WIFI_FAILED, "wifi_failed error code");
}

void test_prov_pairing_failure_keeps_old_active(void) {
    reset_all();
    seed_active();
    g_fake.pair_rc = -404;
    prov_state_t st = process_cand("newnet");
    CHECK(st == PROV_PROVISIONING, "pairing invalid → back to provisioning");
    runtime_config_t act;
    runtime_config_load_active(&act);
    CHECK(strcmp(act.wifi_ssid, "oldnet") == 0, "old active kept");
    CHECK(g_fake.last_err == PROV_ERR_PAIRING_INVALID, "pairing_invalid error");
    CHECK(strcmp(g_fake.last_step, "pairing_invalid") == 0, "stable step name");
}

void test_prov_expired_token_stable_error(void) {
    reset_all();
    g_fake.pair_rc = -410;
    prov_state_t st = process_cand("newnet");
    CHECK(st == PROV_PROVISIONING, "expired → provisioning (user rescan QR)");
    CHECK(g_fake.last_err == PROV_ERR_PAIRING_EXPIRED, "pairing_expired error");
    CHECK(strcmp(g_fake.last_step, "pairing_expired") == 0, "stable step");
}

void test_prov_used_token_stable_error(void) {
    reset_all();
    g_fake.pair_rc = -409;
    process_cand("newnet");
    CHECK(g_fake.last_err == PROV_ERR_PAIRING_USED, "pairing_used error");
}

void test_prov_invalid_payload_rejected_before_anything(void) {
    reset_all();
    seed_active();
    runtime_candidate_t bad = {0}; /* 全空 */
    provisioning_process_candidate(&bad);
    CHECK(g_fake.wifi_calls == 0 && g_fake.pair_calls == 0, "no adapters called");
    CHECK(g_fake.last_err == PROV_ERR_INVALID_PAYLOAD, "invalid_payload");
    runtime_config_t act;
    runtime_config_load_active(&act);
    CHECK(strcmp(act.wifi_ssid, "oldnet") == 0, "active untouched");
}

/* MQTT 失败发生在 promote 之后：active 保留新凭据（服务端已轮换，旧凭据已死），
 * 进 RECOVERY 让 BLE 可见、可重配 */
void test_prov_mqtt_failure_after_promote_enters_recovery(void) {
    reset_all();
    seed_active();
    g_fake.mqtt_rc = -1;
    prov_state_t st = process_cand("newnet");
    CHECK(st == PROV_RECOVERY, "mqtt fail → RECOVERY (not brick)");
    runtime_config_t act;
    runtime_config_load_active(&act);
    CHECK(strcmp(act.wifi_ssid, "newnet") == 0, "new active kept (server rotated creds)");
    CHECK(g_fake.last_err == PROV_ERR_MQTT_INVALID, "mqtt_invalid error");
}

/* Crash C 等价路径：pairing 成功但存储失败 → pending 保持 complete 可恢复 */
void test_prov_storage_failure_recoverable(void) {
    reset_all();
    seed_active();
    nvs_stub_fail_ns("rt_active"); /* promote 写 active 失败（pending ns 正常） */
    prov_state_t st = process_cand("newnet");
    CHECK(st == PROV_PROVISIONING, "storage failure → back to provisioning");
    CHECK(g_fake.last_err == PROV_ERR_STORAGE_FAILED, "storage_failed error");
    runtime_config_t act;
    runtime_config_load_active(&act);
    CHECK(strcmp(act.wifi_ssid, "oldnet") == 0, "old active kept on storage failure");
    /* 完整 pending 幸存 → 下次 boot_reconcile 恢复（解除定向失败） */
    nvs_stub_reset_fail_ns();
    bool promoted = false;
    runtime_config_boot_reconcile(&promoted);
    CHECK(promoted, "crash-boundary: complete pending recovered at next boot");
}

/* 重启后加载 runtime config（provision 成功 → 正常启动路径） */
void test_prov_reboot_after_success_loads_runtime_config(void) {
    reset_all();
    g_fake.wifi_rc = 0;
    process_cand("newnet");
    CHECK(provisioning_state() == PROV_READY, "provisioned");

    /* 模拟重启：状态机重建，NVS 保留 */
    memset(&g_fake, 0, sizeof(g_fake));
    provisioning_init(&g_fake_adapter);
    runtime_config_t cfg;
    prov_config_src_t src = provisioning_boot_decide(&cfg, false);
    CHECK(src == PROV_SRC_NVS, "reboot → SRC_NVS");
    CHECK(strcmp(cfg.wifi_ssid, "newnet") == 0, "reboot loads new config");
    CHECK(strcmp(cfg.mqtt_username, "dev_HID-AAAA0001") == 0, "reboot loads creds");
}

/* 状态推送从不包含密钥（fake 捕获 step 字符串 + 日志字段独立测试覆盖） */
void test_prov_progress_never_carries_secrets(void) {
    reset_all();
    process_cand("newnet");
    CHECK(strstr(g_fake.last_step, "wifi-pass") == NULL, "step has no wifi password");
    CHECK(strstr(g_fake.last_step, "tok-32-hex") == NULL, "step has no token");
    CHECK(prov_state_str(PROV_READY) && strcmp(prov_state_str(PROV_READY), "ready") == 0,
          "state strings stable");
}
