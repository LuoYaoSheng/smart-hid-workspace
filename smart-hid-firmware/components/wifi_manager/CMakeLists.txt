# wifi_manager — Wi-Fi STA（F1/F2 固定 SSID；F3 后由 BLE 配网写 NVS）
idf_component_register(
    SRCS "wifi_manager.c"
    INCLUDE_DIRS "."
    REQUIRES
        esp_wifi
        esp_event
        esp_netif
        nvs_flash
)
