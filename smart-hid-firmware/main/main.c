/**
 * main.c — app_main 装配
 *
 * 启动顺序：
 *   1. NVS（nvs_flash_init）
 *   2. device_identity（device_id 持久化 + boot_id 生成）
 *   3. network init（netif + event loop）
 *   4. USB HID（TinyUSB tusb_init）→ hid_engine_init
 *   5. command_engine_init（内部装配 worker task / lease tick / queue / dedup）
 *   6. 装配 publishers（command_engine ← mqtt_manager 的 publish_*）
 *   7. wifi_manager_init
 *   8. mqtt_manager_init（连上后订阅 command topic，自动 publish online status retained）
 *   9. status_manager_init（心跳）
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

#include "smart_hid_protocol.h"
#include "device_identity.h"
#include "hid_engine.h"
#include "wifi_manager.h"
#include "mqtt_manager.h"
#include "command_engine.h"
#include "command_engine_publisher.h"
#include "status_manager.h"

static const char *TAG = "main";

/* ----------------------------------------------------------------
 * 弱符号覆盖：mqtt_manager → command_engine
 *
 * mqtt_manager 在 command topic 收到 payload 时调 app_mqtt_on_command；
 * 我们在这里转发到 command_engine_handle_raw，并把即时 ack publish 出去。
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
 * USB 初始化由 hid_engine_init 内部完成（tinyusb_driver_install）。
 * main.c 不直接接触 TinyUSB API。
 * ---------------------------------------------------------------- */

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

    /* 3. netif / event loop */
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    /* 4. USB + hid_engine（hid_engine_init 内部调 tinyusb_driver_install 完成 USB 栈注册）*/
    hid_engine_init();

    /* 5. command_engine（queue + worker + lease tick + dedup） */
    ESP_ERROR_CHECK(command_engine_init() == 0 ? ESP_OK : ESP_FAIL);

    /* 6. 装配 publishers */
    command_engine_set_publishers(mqtt_manager_publish_ack, mqtt_manager_publish_event);

    /* 7. Wi-Fi */
    wifi_manager_init();

    /* 8. MQTT（连上后自动 subscribe + publish online status retained） */
    ESP_ERROR_CHECK(mqtt_manager_init() == 0 ? ESP_OK : ESP_FAIL);

    /* 9. status 心跳 */
    status_manager_init();

    ESP_LOGI(TAG, "=== Smart HID Firmware ready ===");
}
