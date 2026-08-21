# led_manager — 板载状态 LED（WS2812 / 单色 GPIO）：状态轮询 + 闪烁语义 + 命令脉冲
idf_component_register(
    SRCS "led_manager.c"
    INCLUDE_DIRS "include"
    REQUIRES
        wifi_manager
        mqtt_manager
        hid_engine
        esp_timer
        led_strip
        driver
)
