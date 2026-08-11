/**
 * wifi_manager.c — STA + 事件处理 + 断开 release_all hook
 */
#include "wifi_manager.h"

#include <string.h>
#include "esp_log.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "nvs_flash.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
#include "sdkconfig.h"
#include "hid_engine.h"

static const char *TAG = "wifi_manager";

#define WIFI_CONNECTED_BIT BIT0
static EventGroupHandle_t s_evt = NULL;
static volatile bool s_connected = false;

static void on_ip(void *arg, esp_event_base_t base, int32_t id, void *data) {
    (void)arg; (void)base; (void)id;
    ip_event_got_ip_t *e = (ip_event_got_ip_t *)data;
    ESP_LOGI(TAG, "got ip: " IPSTR, IP2STR(&e->ip_info.ip));
    s_connected = true;
    if (s_evt) xEventGroupSetBits(s_evt, WIFI_CONNECTED_BIT);
}

static void on_wifi_event(void *arg, esp_event_base_t base, int32_t id, void *data) {
    (void)arg; (void)base; (void)data;
    if (id == WIFI_EVENT_STA_DISCONNECTED) {
        s_connected = false;
        ESP_LOGW(TAG, "disconnected → release_all + retry");
        hid_engine_release_all();
        esp_wifi_connect();
    }
}

int wifi_manager_init(void) {
    /* NVS 由 device_identity 已 init；这里仅确保 netif/wifi 初始化 */
    esp_netif_create_default_wifi_sta();

    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));

    s_evt = xEventGroupCreate();

    ESP_ERROR_CHECK(esp_event_handler_instance_register(WIFI_EVENT, ESP_EVENT_ANY_ID, on_wifi_event, NULL, NULL));
    ESP_ERROR_CHECK(esp_event_handler_instance_register(IP_EVENT, IP_EVENT_STA_GOT_IP, on_ip, NULL, NULL));

    wifi_config_t wc = {0};
    strncpy((char *)wc.sta.ssid,     CONFIG_SMART_HID_WIFI_SSID,     sizeof(wc.sta.ssid) - 1);
    strncpy((char *)wc.sta.password, CONFIG_SMART_HID_WIFI_PASSWORD, sizeof(wc.sta.password) - 1);

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wc));
    ESP_ERROR_CHECK(esp_wifi_start());

    ESP_LOGI(TAG, "wifi STA started, connecting to %s ...", CONFIG_SMART_HID_WIFI_SSID);
    esp_wifi_connect();

    /* 阻塞等首次拿到 IP（最长 30s） */
    xEventGroupWaitBits(s_evt, WIFI_CONNECTED_BIT, pdFALSE, pdTRUE, pdMS_TO_TICKS(30000));
    return 0;
}

bool wifi_manager_is_connected(void) { return s_connected; }
