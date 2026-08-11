# command_engine — Command Queue + Dedup + Boot ID + TTL + Worker
idf_component_register(
    SRCS "command_engine.c" "dedup_cache.c"
    INCLUDE_DIRS "."
    REQUIRES
        smart_hid_protocol
        device_identity
        hid_engine
        mqtt_manager
        status_manager
)
