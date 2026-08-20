/**
 * test_main.c — 固件 host 单测 runner。
 *
 * 编译：见 run.sh（host gcc + freertos/esp/nvs stub + ESP-IDF cJSON，不依赖硬件）。
 * 运行：./smarthid_host_test，退出码 0=全过，1=有失败。
 */
#include "test_framework.h"

void test_hid_keymap_all(void);
void test_dedup_cache_basic(void);
void test_dedup_cache_clear(void);
void test_dedup_cache_ring_wraparound(void);

/* runtime_config（M1-G3） */
void test_rc_no_config_at_fresh_boot(void);
void test_rc_validate_candidate_rejects_missing(void);
void test_rc_stage_pending_keeps_active_untouched(void);
void test_rc_promote_requires_complete(void);
void test_rc_creds_then_promote(void);
void test_rc_boot_reconcile_discards_incomplete(void);
void test_rc_boot_reconcile_promotes_complete(void);
void test_rc_unknown_version_rejected(void);
void test_rc_commit_failure_keeps_recoverable_state(void);
void test_rc_clear(void);
void test_rc_log_fields_redact_secrets(void);

/* provisioning 状态机（M1-G3） */
void test_prov_no_config_enters_provisioning(void);
void test_prov_valid_config_enters_normal_boot(void);
void test_prov_corrupt_config_enters_recovery(void);
void test_prov_run_normal_reaches_ready(void);
void test_prov_run_normal_wifi_fail_enters_recovery(void);
void test_prov_candidate_full_success_commits(void);
void test_prov_wifi_failure_keeps_old_active(void);
void test_prov_pairing_failure_keeps_old_active(void);
void test_prov_expired_token_stable_error(void);
void test_prov_used_token_stable_error(void);
void test_prov_invalid_payload_rejected_before_anything(void);
void test_prov_mqtt_failure_after_promote_enters_recovery(void);
void test_prov_storage_failure_recoverable(void);
void test_prov_reboot_after_success_loads_runtime_config(void);
void test_prov_progress_never_carries_secrets(void);

/* BLE 协议层（M1-G3） */
void test_ble_frame_single_chunk(void);
void test_ble_frame_multi_chunk_ordered(void);
void test_ble_frame_out_of_order_rejected(void);
void test_ble_frame_malformed(void);
void test_ble_parse_candidate(void);
void test_ble_build_info_status(void);

int g_suites_run = 0, g_suites_fail = 0;

int main(void) {
    struct {
        const char *name;
        void (*fn)(void);
    } suites[] = {
        { "hid_keymap: 全键码覆盖 + 别名 + 大小写 + 无效输入", test_hid_keymap_all },
        { "dedup_cache: 基本 miss/hit", test_dedup_cache_basic },
        { "dedup_cache: clear 重置", test_dedup_cache_clear },
        { "dedup_cache: 环形覆盖（256+1 淘汰最旧）", test_dedup_cache_ring_wraparound },

        { "runtime_config: 新机无配置", test_rc_no_config_at_fresh_boot },
        { "runtime_config: candidate 校验拒绝缺失字段", test_rc_validate_candidate_rejects_missing },
        { "runtime_config: stage 只写 pending 不动 active", test_rc_stage_pending_keeps_active_untouched },
        { "runtime_config: 未 complete 禁止 promote", test_rc_promote_requires_complete },
        { "runtime_config: 凭据补全 + promote", test_rc_creds_then_promote },
        { "runtime_config: crash A/B 半截 pending 丢弃", test_rc_boot_reconcile_discards_incomplete },
        { "runtime_config: crash C/D complete pending 提升", test_rc_boot_reconcile_promotes_complete },
        { "runtime_config: 未知 schema 版本拒绝", test_rc_unknown_version_rejected },
        { "runtime_config: commit 失败可恢复", test_rc_commit_failure_keeps_recoverable_state },
        { "runtime_config: factory clear", test_rc_clear },
        { "runtime_config: 日志字段 redact", test_rc_log_fields_redact_secrets },

        { "provisioning: 无配置进 Provisioning", test_prov_no_config_enters_provisioning },
        { "provisioning: 有效配置正常启动", test_prov_valid_config_enters_normal_boot },
        { "provisioning: 损坏配置进 RECOVERY", test_prov_corrupt_config_enters_recovery },
        { "provisioning: 正常路径到 READY", test_prov_run_normal_reaches_ready },
        { "provisioning: Wi-Fi 持续失败有界重试后 RECOVERY", test_prov_run_normal_wifi_fail_enters_recovery },
        { "provisioning: candidate 全流程成功提交", test_prov_candidate_full_success_commits },
        { "provisioning: Wi-Fi 失败保留旧 active", test_prov_wifi_failure_keeps_old_active },
        { "provisioning: pairing 失败保留旧 active", test_prov_pairing_failure_keeps_old_active },
        { "provisioning: 过期 token 稳定错误码", test_prov_expired_token_stable_error },
        { "provisioning: 已用 token 稳定错误码", test_prov_used_token_stable_error },
        { "provisioning: 非法 payload 任何适配器前拒绝", test_prov_invalid_payload_rejected_before_anything },
        { "provisioning: promote 后 MQTT 失败进 RECOVERY", test_prov_mqtt_failure_after_promote_enters_recovery },
        { "provisioning: 存储失败可恢复（crash 边界）", test_prov_storage_failure_recoverable },
        { "provisioning: 重启后加载 runtime config", test_prov_reboot_after_success_loads_runtime_config },
        { "provisioning: 进度无密钥", test_prov_progress_never_carries_secrets },

        { "ble_proto: 单帧", test_ble_frame_single_chunk },
        { "ble_proto: 多帧顺序重组（小 MTU）", test_ble_frame_multi_chunk_ordered },
        { "ble_proto: 乱序拒绝 + seq0 重启", test_ble_frame_out_of_order_rejected },
        { "ble_proto: 非法帧", test_ble_frame_malformed },
        { "ble_proto: candidate 解析（默认端口/缺字段/版本）", test_ble_parse_candidate },
        { "ble_proto: info/status JSON", test_ble_build_info_status },
    };
    int n = (int)(sizeof(suites) / sizeof(suites[0]));

    printf("=== Smart HID Firmware host 单测 ===\n");
    for (int i = 0; i < n; i++) {
        g_suites_run++;
        printf("[RUN] %s\n", suites[i].name);
        suites[i].fn();
    }

    printf("\n=== %d suite, %d passed, %d failed ===\n",
           n, n - g_suites_fail, g_suites_fail);
    return g_suites_fail ? 1 : 0;
}
