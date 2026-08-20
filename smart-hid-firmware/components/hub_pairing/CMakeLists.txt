# hub_pairing — ControlHub 设备侧配对 HTTP 客户端（仅目标机）
idf_component_register(
    SRCS "hub_pairing.c"
    INCLUDE_DIRS "include"
    REQUIRES json esp_http_client log
)
