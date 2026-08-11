# device_identity — device_id (NVS) + boot_id (每次启动生成)
idf_component_register(
    SRCS "device_identity.c"
    INCLUDE_DIRS "include"
    REQUIRES nvs_flash
        smart_hid_protocol
        esp_timer
)
