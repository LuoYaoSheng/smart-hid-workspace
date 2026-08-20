# Smart HID Firmware — main app
#
# app_main 装配全部组件，启动顺序见 main/main.c。
idf_component_register(
    SRCS "main.c"
    INCLUDE_DIRS "."
    REQUIRES
        smart_hid_protocol
        device_identity
        wifi_manager
        mqtt_manager
        command_engine
        hid_engine
        status_manager
        runtime_config
        provisioning
        hub_pairing
        ble_provision
)
