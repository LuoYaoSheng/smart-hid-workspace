/**
 * status_manager.c — 心跳 task
 */
#include "status_manager.h"

#include <string.h>
#include <time.h>
#include "esp_log.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "sdkconfig.h"

#include "device_identity.h"
#include "hid_engine.h"
#include "mqtt_manager.h"
#include "smart_hid_protocol.h"

static const char *TAG = "status_manager";

static void heartbeat_task(void *arg) {
    (void)arg;
    /* 心跳周期 */
    int period_sec = CONFIG_SMART_HID_STATUS_HEARTBEAT_SEC > 0 ?
                     CONFIG_SMART_HID_STATUS_HEARTBEAT_SEC : 10;
    while (true) {
        vTaskDelay(pdMS_TO_TICKS(period_sec * 1000));
        status_manager_publish_now(true);
    }
}

void status_manager_publish_now(bool online) {
    smart_hid_status_t st = {0};
    strncpy(st.protocol,  SMART_HID_PROTOCOL_VERSION, sizeof(st.protocol) - 1);
    strncpy(st.device_id, device_identity_get_device_id(), sizeof(st.device_id) - 1);
    st.online        = online;
    strncpy(st.boot_id, device_identity_get_boot_id(), sizeof(st.boot_id) - 1);
    st.usb_hid_ready = hid_engine_is_ready();
    strncpy(st.firmware, device_identity_get_firmware(), sizeof(st.firmware) - 1);
    st.timestamp = (int64_t)time(NULL);
    mqtt_manager_publish_status(&st);
    ESP_LOGI(TAG, "status published online=%d usb_hid_ready=%d",
             st.online, st.usb_hid_ready);
}

int status_manager_init(void) {
    BaseType_t ok = xTaskCreate(heartbeat_task, "status_hb", 3072, NULL, 3, NULL);
    if (ok != pdPASS) {
        ESP_LOGE(TAG, "create heartbeat task failed");
        return -1;
    }
    ESP_LOGI(TAG, "status_manager started");
    return 0;
}
