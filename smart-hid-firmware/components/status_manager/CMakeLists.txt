# status_manager — 周期 status 心跳 + 上线/离线事件
idf_component_register(
    SRCS "status_manager.c"
    INCLUDE_DIRS "."
    REQUIRES
        smart_hid_protocol
        device_identity
        mqtt_manager
        hid_engine
)
