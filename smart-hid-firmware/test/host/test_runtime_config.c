/**
 * test_runtime_config.c — NVS 运行时配置 host 单测（spec M1-G3 §39 / §40 crash 界）
 */
#include "test_framework.h"
#include "runtime_config.h"
#include "nvs.h"

#include <string.h>

static runtime_candidate_t cand(const char *ssid, const char *host, const char *token) {
    runtime_candidate_t c = {0};
    snprintf(c.wifi_ssid, sizeof(c.wifi_ssid), "%s", ssid);
    snprintf(c.wifi_password, sizeof(c.wifi_password), "%s", "wifi-secret-1");
    snprintf(c.hub_host, sizeof(c.hub_host), "%s", host);
    c.hub_port = 17892;
    snprintf(c.token, sizeof(c.token), "%s", token);
    return c;
}


static int stage(const char *ssid, const char *host, const char *token) {
    runtime_candidate_t c = cand(ssid, host, token);
    return runtime_config_stage_pending(&c);
}

static void reset_all(void) {
    nvs_stub_reset();
    runtime_config_init();
}

/* 新机：无 active 配置（正常状态，非错误） */
void test_rc_no_config_at_fresh_boot(void) {
    reset_all();
    runtime_config_t cfg;
    CHECK(runtime_config_load_active(&cfg) == RC_ERR_NO_CONFIG, "fresh boot should report NO_CONFIG");
}

/* candidate 校验：缺 SSID/host/token 拒绝 */
void test_rc_validate_candidate_rejects_missing(void) {
    reset_all();
    runtime_candidate_t c = cand("net", "192.168.1.8", "tok-123");
    CHECK(runtime_config_validate_candidate(&c) == RC_OK, "full candidate valid");

    runtime_candidate_t no_ssid = cand("", "192.168.1.8", "tok");
    CHECK(runtime_config_validate_candidate(&no_ssid) == RC_ERR_INVALID, "empty ssid rejected");
    runtime_candidate_t no_host = cand("net", "", "tok");
    CHECK(runtime_config_validate_candidate(&no_host) == RC_ERR_INVALID, "empty hub_host rejected");
    runtime_candidate_t no_tok = cand("net", "192.168.1.8", "");
    CHECK(runtime_config_validate_candidate(&no_tok) == RC_ERR_INVALID, "empty token rejected");
}

/* stage 只写 pending：active 仍无（candidate 不提前破坏 active） */
void test_rc_stage_pending_keeps_active_untouched(void) {
    reset_all();
    CHECK(stage("net", "192.168.1.8", "tok-abc") == RC_OK, "stage ok");

    runtime_config_t act;
    CHECK(runtime_config_load_active(&act) == RC_ERR_NO_CONFIG, "active must stay empty");

    runtime_config_t p;
    CHECK(runtime_config_load_pending(&p) == RC_OK, "pending loadable");
    CHECK(strcmp(p.wifi_ssid, "net") == 0, "pending ssid");
    CHECK(p.complete == false, "pending not complete before creds");
}

/* 未 complete 的 pending 禁止 promote */
void test_rc_promote_requires_complete(void) {
    reset_all();
    stage("net", "192.168.1.8", "tok");
    CHECK(runtime_config_promote_pending() == RC_ERR_INVALID, "promote incomplete pending must fail");
    CHECK(runtime_config_load_active(&(runtime_config_t){0}) == RC_ERR_NO_CONFIG, "active still empty");
}

/* 凭据补全 + promote：active 生效、generation+1、pending 清空 */
void test_rc_creds_then_promote(void) {
    reset_all();
    stage("newnet", "192.168.1.8", "tok");
    CHECK(runtime_config_pending_set_creds("192.168.1.8", 17891, "dev_HID-AAAA0001", "cred-hex-64") == RC_OK,
          "set creds");

    runtime_config_t p;
    runtime_config_load_pending(&p);
    CHECK(p.complete == true, "pending complete after creds");

    CHECK(runtime_config_promote_pending() == RC_OK, "promote ok");
    runtime_config_t act;
    CHECK(runtime_config_load_active(&act) == RC_OK, "active loads");
    CHECK(strcmp(act.wifi_ssid, "newnet") == 0, "active ssid from candidate");
    CHECK(strcmp(act.mqtt_username, "dev_HID-AAAA0001") == 0, "active mqtt user from creds");
    CHECK(act.complete == true && act.generation == 1, "active complete + generation 1");
    CHECK(runtime_config_load_pending(&p) == RC_ERR_NO_PENDING, "pending cleared after promote");
}

/* Crash A/B：半截 pending 在 boot 被丢弃，active 不动 */
void test_rc_boot_reconcile_discards_incomplete(void) {
    reset_all();
    /* 先有一份完整 active */
    stage("oldnet", "192.168.1.8", "tok1");
    runtime_config_pending_set_creds("192.168.1.8", 17891, "dev_HID-AAAA0001", "cred1");
    runtime_config_promote_pending();
    /* 模拟重配网中途中掉电：只有新 candidate，没到 creds */
    stage("badnew", "192.168.1.9", "tok2");

    bool promoted = true;
    CHECK(runtime_config_boot_reconcile(&promoted) == RC_OK, "reconcile ok");
    CHECK(promoted == false, "incomplete pending must NOT promote");
    runtime_config_t act, p;
    runtime_config_load_active(&act);
    CHECK(strcmp(act.wifi_ssid, "oldnet") == 0, "old active kept");
    CHECK(act.generation == 1, "generation unchanged");
    CHECK(runtime_config_load_pending(&p) == RC_ERR_NO_PENDING, "stale pending discarded");
}

/* Crash C/D：complete pending 在 boot 被提升（token 已消费，凭据已持久化） */
void test_rc_boot_reconcile_promotes_complete(void) {
    reset_all();
    stage("oldnet", "192.168.1.8", "tok1");
    runtime_config_pending_set_creds("192.168.1.8", 17891, "dev_HID-AAAA0001", "cred1");
    runtime_config_promote_pending();

    /* 模拟：pairing 成功（token 已被 ControlHub 消费）、凭据已写 pending，
     * 但 promote 前掉电 */
    stage("goodnew", "192.168.1.8", "tok2");
    runtime_config_pending_set_creds("192.168.1.8", 17891, "dev_HID-AAAA0002", "cred2");

    bool promoted = false;
    CHECK(runtime_config_boot_reconcile(&promoted) == RC_OK, "reconcile ok");
    CHECK(promoted == true, "complete pending must promote at boot");
    runtime_config_t act;
    runtime_config_load_active(&act);
    CHECK(strcmp(act.wifi_ssid, "goodnew") == 0, "active = crashed pending values");
    CHECK(strcmp(act.mqtt_username, "dev_HID-AAAA0002") == 0, "active creds from pending");
    CHECK(act.generation == 2, "generation increments");
}

/* 未知未来 schema 版本：拒绝按旧结构读 */
void test_rc_unknown_version_rejected(void) {
    reset_all();
    stage("net", "192.168.1.8", "tok");
    runtime_config_pending_set_creds("192.168.1.8", 17891, "dev_HID-AAAA0001", "cred");
    runtime_config_promote_pending();
    /* 模拟未来固件把版本写成 99 */
    nvs_stub_raw_set_u8("rt_active", "ver", 99);
    runtime_config_t act;
    CHECK(runtime_config_load_active(&act) == RC_ERR_VERSION, "future schema refused");
}

/* NVS commit 失败：promote 报错且状态可恢复（complete pending 仍在） */
void test_rc_commit_failure_keeps_recoverable_state(void) {
    reset_all();
    stage("net", "192.168.1.8", "tok");
    runtime_config_pending_set_creds("192.168.1.8", 17891, "dev_HID-AAAA0001", "cred");
    nvs_stub_fail_commit(true);
    CHECK(runtime_config_promote_pending() == RC_ERR_NVS, "commit failure surfaces");
    nvs_stub_fail_commit(false);
    runtime_config_t p;
    CHECK(runtime_config_load_pending(&p) == RC_OK && p.complete, "complete pending survived → boot_reconcile can recover");
    bool promoted = false;
    CHECK(runtime_config_boot_reconcile(&promoted) == RC_OK && promoted, "recovered on next boot");
}

/* factory reset 底层能力 */
void test_rc_clear(void) {
    reset_all();
    stage("net", "192.168.1.8", "tok");
    runtime_config_pending_set_creds("192.168.1.8", 17891, "dev_HID-AAAA0001", "cred");
    runtime_config_promote_pending();
    CHECK(runtime_config_clear() == RC_OK, "clear ok");
    CHECK(runtime_config_load_active(&(runtime_config_t){0}) == RC_ERR_NO_CONFIG, "active gone");
}

/* 日志字段永不包含密码/token */
void test_rc_log_fields_redact_secrets(void) {
    reset_all();
    runtime_config_t cfg = {0};
    cfg.schema_version = 1;
    snprintf(cfg.wifi_ssid, sizeof(cfg.wifi_ssid), "%s", "ssid-value");
    snprintf(cfg.wifi_password, sizeof(cfg.wifi_password), "%s", "SECRET-WIFI-PASS");
    snprintf(cfg.hub_host, sizeof(cfg.hub_host), "%s", "192.168.1.8");
    snprintf(cfg.mqtt_username, sizeof(cfg.mqtt_username), "%s", "dev_HID-AAAA0001");
    snprintf(cfg.mqtt_password, sizeof(cfg.mqtt_password), "%s", "SECRET-MQTT-PASS");

    char buf[256];
    runtime_config_log_fields(buf, sizeof(buf), &cfg);
    CHECK(strstr(buf, "SECRET-WIFI-PASS") == NULL, "wifi password must not appear");
    CHECK(strstr(buf, "SECRET-MQTT-PASS") == NULL, "mqtt password must not appear");
    CHECK(strstr(buf, "ssid-value") != NULL, "ssid allowed");
    CHECK(strstr(buf, "192.168.1.8") != NULL, "host allowed");
}
