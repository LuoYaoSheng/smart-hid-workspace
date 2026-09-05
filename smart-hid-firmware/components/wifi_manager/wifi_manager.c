/**
 * wifi_manager.c — STA + 事件处理 + 断开 release_all hook + 运行时配置连接
 */
#include "wifi_manager.h"

#include <string.h>
#include "esp_err.h"
#include "esp_log.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
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

    /* 关闭 Wi-Fi 省电：默认 WIFI_PS_MIN_MODEM 让射频在信标间隔休眠，
     * 收包延迟可达 300ms+，弱信号下导致 MQTT keep-alive 超时→周期性断连重连
     * （真机 2026-08-20：RSSI≈-77 时每 ~10s 掉线一次的根因，修后 8/8 连发全通）。
     * 本设备 USB 总线供电，延迟与稳定性优先于功耗。
     * 按 IDF 文档须在 esp_wifi_start() 之后调用。 */
    ESP_ERROR_CHECK(esp_wifi_set_ps(WIFI_PS_NONE));

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

    esp_wifi_disconnect(); /* 旧连接（若有）——异步，旧尝试可能仍在飞行 */
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wc));
    ESP_LOGW(TAG, "connecting to ssid=%s ...", ssid); /* 密码绝不打日志 */
    /* esp_wifi_disconnect() 不打断已 in-flight 的连接尝试：此时
     * esp_wifi_connect() 返回 ESP_ERR_WIFI_CONN（E14-T3 真机：上次失败的
     * 自动重连未落地即来新候选 → 0x3007 → ESP_ERROR_CHECK abort 整机重启，
     * BLE 断链，App 只见 connection_lost）。重配路径绝不允许 abort：
     * 有界等待旧尝试自行落地后再发起；超限则按连接失败上报 wifi_failed。 */
    esp_err_t err = esp_wifi_connect();
    for (int i = 0; err == ESP_ERR_WIFI_CONN && i < 30; i++) {
        vTaskDelay(pdMS_TO_TICKS(100));
        err = esp_wifi_connect();
    }
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "esp_wifi_connect failed: %s（旧尝试未落地，按 wifi_failed 上报）",
                 esp_err_to_name(err));
        return -1;
    }

    EventBits_t bits = xEventGroupWaitBits(s_evt, WIFI_CONNECTED_BIT,
                                           pdFALSE, pdTRUE,
                                           pdMS_TO_TICKS(timeout_ms));
    if (bits & WIFI_CONNECTED_BIT) return 0;
    ESP_LOGW(TAG, "wifi connect timeout (%u ms)", (unsigned)timeout_ms);
    return -1;
}

bool wifi_manager_is_connected(void) { return s_connected; }
