/**
 * main.c — app_main 装配（M1-G3：Provisioning 启动流）
 *
 * 启动顺序：
 *   1. NVS（nvs_flash_init）
 *   2. device_identity（device_id 持久化 + boot_id 生成）
 *   3. led_manager（状态 LED 轮询任务，越早起灯越早可见）
 *   4. network init（netif + event loop）
 *   5. USB HID（TinyUSB）→ hid_engine_init
 *   6. command_engine_init（队列 / worker / lease tick / dedup）
 *   7. 装配 publishers（command_engine ← mqtt_manager 的 publish_*；EXECUTED 触发 LED 脉冲）
 *   8. runtime_config_init + boot_reconcile（crash 恢复：complete pending → promote）
 *   9. provisioning_boot_decide：
 *        NVS active valid ──→ 正常路径（Wi-Fi → MQTT → READY）
 *        DEV_STATIC 开且无 NVS ─→ Kconfig 开发配置（仅内存，绝不写 NVS）
 *        无配置 ──────────→ UNPROVISIONED → BLE Provisioning
 *        版本未知 ────────→ RECOVERY（BLE 开，active 只读）
 *  10. prov_task：状态机 + BLE candidate 队列 + 运行期 RECOVERY 监控
 *
 * 配置优先级（DEVELOPMENT_RULES / PROVISIONING_V1）：
 *   1. valid active runtime config（NVS）
 *   2. DEV_STATIC_CONFIG 显式开发 fallback（默认 OFF）
 *   3. Provision Mode（BLE）
 *   Kconfig 永不覆盖 NVS。
 *
 * Fail-safe：MQTT/Wi-Fi 断开 hook → hid_engine_release_all() 由各 manager 自带。
 */
#include <stdio.h>
#include <string.h>

#include "esp_log.h"
#include "esp_system.h"
#include "nvs_flash.h"
#include "esp_netif.h"
#include "esp_event.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/queue.h"

#include "smart_hid_protocol.h"
#include "device_identity.h"
#include "hid_engine.h"
#include "wifi_manager.h"
#include "mqtt_manager.h"
#include "command_engine.h"
#include "command_engine_publisher.h"
#include "status_manager.h"
#include "runtime_config.h"
#include "provisioning.h"
#include "hub_pairing.h"
#include "ble_provision.h"
#include "led_manager.h"

static const char *TAG = "main";

/* ----------------------------------------------------------------
 * 弱符号覆盖：mqtt_manager → command_engine
 * ---------------------------------------------------------------- */
void app_mqtt_on_command(const char *topic, const char *payload, int len) {
    smart_hid_ack_t immediate;
    bool has_immediate = command_engine_handle_raw(topic, payload, (size_t)len, &immediate);
    if (has_immediate) {
        mqtt_manager_publish_ack(&immediate);
    }
    /* 否则：command_engine 已入队，由 worker 异步 publish executed ack */
}

/* ----------------------------------------------------------------
 * provisioning 适配器（wifi_manager / hub_pairing / mqtt_manager 包装）
 * ---------------------------------------------------------------- */
static int adapter_wifi_connect(const char *ssid, const char *password, uint32_t timeout_ms) {
    return wifi_manager_connect_sta(ssid, password, timeout_ms);
}

static int adapter_hub_pair(const runtime_candidate_t *cand, prov_creds_t *creds_out) {
    hub_pairing_creds_t hc;
    int rc = hub_pairing_perform(cand->hub_host, cand->hub_port, cand->token,
                                 device_identity_get_device_id(),
                                 device_identity_get_boot_id(),
                                 device_identity_get_firmware(),
                                 "esp32s3", &hc);
    if (rc == 0) {
        strlcpy(creds_out->mqtt_host, hc.mqtt_host, sizeof(creds_out->mqtt_host));
        creds_out->mqtt_port = hc.mqtt_port;
        strlcpy(creds_out->mqtt_username, hc.mqtt_username, sizeof(creds_out->mqtt_username));
        strlcpy(creds_out->mqtt_password, hc.mqtt_password, sizeof(creds_out->mqtt_password));
    }
    return rc;
}

static int adapter_mqtt_start(const char *host, uint16_t port,
                              const char *username, const char *password,
                              uint32_t timeout_ms) {
    mqtt_manager_configure(host, port, username, password);
    if (mqtt_manager_init() != 0) return -1;
    return mqtt_manager_wait_connected(timeout_ms);
}

static void adapter_on_progress(prov_state_t st, const char *step, prov_error_t err) {
    ESP_LOGI(TAG, "[prov] state=%s step=%s err=%s",
             prov_state_str(st), step ? step : "-", prov_error_str(err) ? prov_error_str(err) : "none");
    ble_provision_publish(st, step, err);
}

static const prov_adapter_t g_prov_adapter = {
    .wifi_connect = adapter_wifi_connect,
    .hub_pair     = adapter_hub_pair,
    .mqtt_start   = adapter_mqtt_start,
    .on_progress  = adapter_on_progress,
};

/* ----------------------------------------------------------------
 * prov task：candidate 队列消费 + RECOVERY 监控
 * ---------------------------------------------------------------- */
static QueueHandle_t s_cand_queue = NULL;
static bool s_ble_inited = false;

static void on_ble_candidate(const runtime_candidate_t *cand);

static void ensure_ble_started(void) {
    if (s_ble_inited) {
        ble_provision_set_advertising(true);
        return;
    }
    char name[24];
    snprintf(name, sizeof(name), "SHID-%s", device_identity_get_device_id() + 4);
    if (ble_provision_init(name, on_ble_candidate) == 0) {
        s_ble_inited = true;
        ble_provision_set_advertising(true);
    }
}

static void on_ble_candidate(const runtime_candidate_t *cand) {
    /* BLE 栈回调上下文：只入队，配网逻辑在 prov task 串行执行 */
    if (s_cand_queue != NULL) {
        xQueueSend(s_cand_queue, cand, 0);
    }
}

/* DEV 静态配置（CONFIG_SMART_HID_DEV_STATIC_CONFIG=y 且无 NVS 配置时；
 * 仅内存，绝不写 NVS——Kconfig 不覆盖用户 BLE 配置） */
static bool build_dev_static_config(runtime_config_t *cfg) {
#if defined(CONFIG_SMART_HID_DEV_STATIC_CONFIG) && CONFIG_SMART_HID_DEV_STATIC_CONFIG
    if (strlen(CONFIG_SMART_HID_WIFI_SSID) == 0 ||
        strlen(CONFIG_SMART_HID_MQTT_BROKER_HOST) == 0) {
        ESP_LOGE(TAG, "DEV_STATIC_CONFIG enabled but Kconfig wifi/mqtt empty");
        return false;
    }
    memset(cfg, 0, sizeof(*cfg));
    cfg->schema_version = RUNTIME_CONFIG_SCHEMA_VERSION;
    strlcpy(cfg->wifi_ssid, CONFIG_SMART_HID_WIFI_SSID, sizeof(cfg->wifi_ssid));
    strlcpy(cfg->wifi_password, CONFIG_SMART_HID_WIFI_PASSWORD, sizeof(cfg->wifi_password));
    strlcpy(cfg->mqtt_host, CONFIG_SMART_HID_MQTT_BROKER_HOST, sizeof(cfg->mqtt_host));
    cfg->mqtt_port = (uint16_t)CONFIG_SMART_HID_MQTT_BROKER_PORT;
    if (strlen(CONFIG_SMART_HID_MQTT_USERNAME) > 0) {
        strlcpy(cfg->mqtt_username, CONFIG_SMART_HID_MQTT_USERNAME, sizeof(cfg->mqtt_username));
        strlcpy(cfg->mqtt_password, CONFIG_SMART_HID_MQTT_PASSWORD, sizeof(cfg->mqtt_password));
    }
    cfg->complete = true; /* DEV 假定完整（hub 账号直连，无 pairing） */
    ESP_LOGW(TAG, "DEV STATIC config active (NOT production; never written to NVS)");
    return true;
#else
    (void)cfg;
    return false;
#endif
}

static void prov_task(void *arg) {
    (void)arg;
    runtime_config_t cfg;
    prov_config_src_t src = provisioning_boot_decide(&cfg, false);

    if (src == PROV_SRC_NONE) {
        /* 无 NVS：DEV 静态模式或 Provision Mode */
        if (build_dev_static_config(&cfg)) {
            src = PROV_SRC_DEV_STATIC;
        }
    }

    if (src != PROV_SRC_NONE) {
        ESP_LOGI(TAG, "boot: normal path (src=%d)", src);
        provisioning_run_normal(&cfg);
    } else {
        ESP_LOGI(TAG, "boot: no config → provisioning mode");
    }

    /* BLE：UNPROVISIONED / RECOVERY / PROVISIONING 态开；READY 关 */
    prov_state_t st = provisioning_state();
    if (st != PROV_READY) {
        ensure_ble_started();
    }

    /* 主循环：消费 BLE candidate；READY 后监控长期失联 → RECOVERY */
    runtime_candidate_t cand;
    uint32_t disconnected_ms = 0;
    while (true) {
        if (provisioning_state() == PROV_READY) {
            /* 运行期监控：Wi-Fi/MQTT 持续断开 5 分钟 → RECOVERY（BLE 可见） */
            vTaskDelay(pdMS_TO_TICKS(5000));
            if (wifi_manager_is_connected() && mqtt_manager_is_connected()) {
                disconnected_ms = 0;
                continue;
            }
            disconnected_ms += 5000;
            if (disconnected_ms >= 5 * 60 * 1000U) {
                ESP_LOGW(TAG, "persistent disconnect → RECOVERY (BLE provisioning on)");
                disconnected_ms = 0;
                ensure_ble_started();
                adapter_on_progress(PROV_RECOVERY, "persistent_disconnect", PROV_ERR_WIFI_FAILED);
            }
            continue;
        }

        /* Provisioning/Recovery：等 candidate（阻塞 1s 轮询，避免忙等） */
        if (s_cand_queue != NULL &&
            xQueueReceive(s_cand_queue, &cand, pdMS_TO_TICKS(1000)) == pdTRUE) {
            prov_state_t after = provisioning_process_candidate(&cand);
            if (after == PROV_READY) {
                ble_provision_set_advertising(false);
            } else if (after == PROV_RECOVERY) {
                ble_provision_set_advertising(true);
            }
        } else {
            vTaskDelay(pdMS_TO_TICKS(100)); /* 无队列（不应发生）防忙等 */
        }
    }
}

/* ----------------------------------------------------------------
 * publisher 包装：EXECUTED ack 顺带给 LED 一个脉冲（命令到达肉眼可见）
 * ---------------------------------------------------------------- */
static void publish_ack_with_led_pulse(const smart_hid_ack_t *ack) {
    mqtt_manager_publish_ack(ack);
    if (ack->status == SMART_HID_ACK_EXECUTED) {
        led_manager_pulse();
    }
}

/* ----------------------------------------------------------------
 * app_main
 * ---------------------------------------------------------------- */
void app_main(void) {
    ESP_LOGI(TAG, "=== Smart HID Firmware boot ===");

    /* 1. NVS */
    esp_err_t err = nvs_flash_init();
    if (err == ESP_ERR_NVS_NO_FREE_PAGES || err == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        err = nvs_flash_init();
    }
    ESP_ERROR_CHECK(err);

    /* 2. device_identity（NVS + boot_id） */
    ESP_ERROR_CHECK(device_identity_init(CONFIG_SMART_HID_DEVICE_ID) == 0 ? ESP_OK : ESP_FAIL);
    ESP_LOGI(TAG, "device_id=%s boot_id=%s firmware=%s",
             device_identity_get_device_id(),
             device_identity_get_boot_id(),
             device_identity_get_firmware());

    /* 3. led_manager（状态 LED；轮询式，不依赖后续初始化） */
    ESP_ERROR_CHECK(led_manager_init() == 0 ? ESP_OK : ESP_FAIL);

    /* 4. netif / event loop */
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    /* 5. USB + hid_engine（hid_engine_init 内部调 tinyusb_driver_install 完成 USB 栈注册）*/
    hid_engine_init();

    /* 6. command_engine（queue + worker + lease tick + dedup） */
    ESP_ERROR_CHECK(command_engine_init() == 0 ? ESP_OK : ESP_FAIL);

    /* 7. 装配 publishers（EXECUTED ack 触发 LED 脉冲） */
    command_engine_set_publishers(publish_ack_with_led_pulse, mqtt_manager_publish_event);

    /* 8. runtime config + crash 恢复（complete pending → promote） */
    ESP_ERROR_CHECK(runtime_config_init() == RC_OK ? ESP_OK : ESP_FAIL);
    bool promoted = false;
    int rc = runtime_config_boot_reconcile(&promoted);
    if (rc == RC_OK && promoted) {
        ESP_LOGW(TAG, "boot reconcile promoted complete pending (crash recovery)");
    } else if (rc != RC_OK) {
        ESP_LOGE(TAG, "boot reconcile: %s", runtime_config_strerror(rc));
    }

    /* 9. Wi-Fi 驱动（不连接——参数由 prov_task 决定） */
    ESP_ERROR_CHECK(wifi_manager_init() == 0 ? ESP_OK : ESP_FAIL);

    /* 10. provisioning（适配器 + BLE 队列 + 状态机 task） */
    ESP_ERROR_CHECK(provisioning_init(&g_prov_adapter) == 0 ? ESP_OK : ESP_FAIL);
    s_cand_queue = xQueueCreate(2, sizeof(runtime_candidate_t));
    xTaskCreate(prov_task, "prov_task", 6144, NULL, 5, NULL);

    /* 11. status 心跳 */
    status_manager_init();

    ESP_LOGI(TAG, "=== Smart HID Firmware boot complete (state machine running) ===");
}
