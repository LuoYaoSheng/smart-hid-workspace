# hid_engine — USB Composite HID（Keyboard + Mouse）报告发送 + Lease + release_all
idf_component_register(
    SRCS "hid_engine.c" "hid_keymap.c"
    INCLUDE_DIRS "include"
    REQUIRES
        smart_hid_protocol
        esp_tinyusb
        esp_timer
)
