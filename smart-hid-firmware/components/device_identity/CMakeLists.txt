# device_identity — device_id (NVS) + boot_id (每次启动生成) + 固件版本（esp_app_desc）
idf_component_register(
    SRCS "device_identity.c"
    INCLUDE_DIRS "include"
    REQUIRES nvs_flash
        smart_hid_protocol
        esp_timer
    PRIV_REQUIRES esp_app_format   # esp_app_get_description()（版本来自 PROJECT_VER ← VERSION）
)
