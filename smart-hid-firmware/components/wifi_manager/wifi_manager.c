/**
 * wifi_manager.c — STA + 事件处理 + 断开 release_all hook + 运行时配置连接
 */
#include "wifi_manager.h"

#include <string.h>
#include "esp_log.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
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
        esp_wifi_connect(); /* 自动重连保留（bounded 场景由上层状态机决策 RECOVERY） */
    }
}

int wifi_manager_init(void) {
    /* NVS 由 main 已 init；这里仅 netif/wifi 初始化（不连接——参数由
     * runtime config 或 DEV Kconfig 提供，见 wifi_manager_connect_sta） */
    esp_netif_create_default_wifi_sta();

    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));

    s_evt = xEventGroupCreate();

    ESP_ERROR_CHECK(esp_event_handler_instance_register(WIFI_EVENT, ESP_EVENT_ANY_ID, on_wifi_event, NULL, NULL));
    ESP_ERROR_CHECK(esp_event_handler_instance_register(IP_EVENT, IP_EVENT_STA_GOT_IP, on_ip, NULL, NULL));

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_start());
    ESP_LOGI(TAG, "wifi STA started (runtime config mode)");
    return 0;
}

int wifi_manager_connect_sta(const char *ssid, const char *password, uint32_t timeout_ms) {
    if (ssid == NULL || ssid[0] == '\0') return -1;

    /* 清掉上一次连接的 CONNECTED 位（重配场景） */
    if (s_evt) xEventGroupClearBits(s_evt, WIFI_CONNECTED_BIT);
    s_connected = false;

    wifi_config_t wc = {0};
    strlcpy((char *)wc.sta.ssid, ssid, sizeof(wc.sta.ssid));
    if (password != NULL) {
        strlcpy((char *)wc.sta.password, password, sizeof(wc.sta.password));
    }

    esp_wifi_disconnect(); /* 旧连接（若有） */
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wc));
    ESP_LOGW(TAG, "connecting to ssid=%s ...", ssid); /* 密码绝不打日志 */
    ESP_ERROR_CHECK(esp_wifi_connect());

    EventBits_t bits = xEventGroupWaitBits(s_evt, WIFI_CONNECTED_BIT,
                                           pdFALSE, pdTRUE,
                                           pdMS_TO_TICKS(timeout_ms));
    if (bits & WIFI_CONNECTED_BIT) return 0;
    ESP_LOGW(TAG, "wifi connect timeout (%u ms)", (unsigned)timeout_ms);
    return -1;
}

bool wifi_manager_is_connected(void) { return s_connected; }
