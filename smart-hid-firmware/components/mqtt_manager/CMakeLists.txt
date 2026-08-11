# mqtt_manager — esp-mqtt 封装：connect / subscribe / publish ack/status/event
idf_component_register(
    SRCS "mqtt_manager.c"
    INCLUDE_DIRS "."
    REQUIRES
        smart_hid_protocol
        device_identity
        mqtt
)
