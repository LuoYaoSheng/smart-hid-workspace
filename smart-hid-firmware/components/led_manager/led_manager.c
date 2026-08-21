/**
 * led_manager.c — 板载状态 LED：状态轮询 + 闪烁语义 + 命令脉冲
 *
 * 实现方式：
 *   - 50ms tick 任务轮询 wifi_manager_is_connected / mqtt_manager_is_connected /
 *     hid_engine_is_ready 三个现成查询函数，不侵入任何现有组件；
 *     Wi-Fi/MQTT 初始化前轮询安全（读到未连接）。
 *   - 硬件后端由 Kconfig 选择：
 *       WS2812：led_strip（RMT），颜色 + 节奏双语义
 *       SIMPLE：gpio 直驱，仅节奏语义；脉冲 = 短暂反相
 *       NONE  ：空实现，不建任务
 *   - 命令脉冲：led_manager_pulse() 记录截止时间，tick 任务在该窗口内用
 *     白色（单色 LED 为反相）覆盖当前状态显示。
 */
#include "led_manager.h"

#include <stdbool.h>
#include <stdint.h>

#include "sdkconfig.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

#include "wifi_manager.h"
#include "mqtt_manager.h"
#include "hid_engine.h"

#if defined(CONFIG_SMART_HID_LED_TYPE_WS2812)
#include "led_strip.h"
#elif defined(CONFIG_SMART_HID_LED_TYPE_SIMPLE)
#include "driver/gpio.h"
#endif

static const char *TAG = "led_manager";

#define LED_TICK_MS 50

/* ----------------------------------------------------------------
 * 状态定义与闪烁节拍（mask 数组逐 tick 定义亮灭，长度即周期）
 * ---------------------------------------------------------------- */
typedef enum {
    LED_STATE_WIFI_CONNECTING = 0,  /* Wi-Fi 未连（含上电初期） */
    LED_STATE_LINK_LOST,            /* 曾就绪后 Wi-Fi 断开 */
    LED_STATE_MQTT_CONNECTING,      /* Wi-Fi 通、MQTT 未通 */
    LED_STATE_HOST_MISSING,         /* 网络全通、USB 未挂载到宿主机 */
    LED_STATE_READY,                /* 全链路就绪 */
    LED_STATE_COUNT
} led_state_t;

static const uint8_t k_pat_solid[]  = {1};
static const uint8_t k_pat_fast[]   = {1,1,1,1,1, 0,0,0,0,0};                      /* 2Hz */
static const uint8_t k_pat_slow[]   = {1,1,1,1,1,1,1,1,1,1, 0,0,0,0,0,0,0,0,0,0};  /* 0.5Hz */
static const uint8_t k_pat_double[] = {1,1,1, 0,0,0, 1,1,1, 0,0,0,0,0,0,0,0,0,0,0};/* 双闪 */

typedef struct {
    const uint8_t *pat;
    uint32_t       len;   /* 周期（tick 数） */
    uint8_t r, g, b;      /* WS2812 颜色（已按亮度缩放）；单色 LED 忽略 */
} led_pattern_t;

#if defined(CONFIG_SMART_HID_LED_TYPE_WS2812)
#define WS_BRIGHTNESS ((uint8_t)CONFIG_SMART_HID_LED_WS2812_BRIGHTNESS)
#define PAT_WS2812(pat, r_, g_, b_) {(pat), sizeof(pat), (r_), (g_), (b_)}
#else
#define PAT_MONO(pat)               {(pat), sizeof(pat), 0, 0, 0}
#endif

static led_pattern_t state_pattern(led_state_t st) {
#if defined(CONFIG_SMART_HID_LED_TYPE_WS2812)
    const uint8_t B = WS_BRIGHTNESS;
    switch (st) {
        case LED_STATE_WIFI_CONNECTING:  return (led_pattern_t)PAT_WS2812(k_pat_fast,   B, B, 0); /* 黄 */
        case LED_STATE_LINK_LOST:        return (led_pattern_t)PAT_WS2812(k_pat_fast,   B, 0, 0); /* 红 */
        case LED_STATE_MQTT_CONNECTING:  return (led_pattern_t)PAT_WS2812(k_pat_slow,   0, B, B); /* 青 */
        case LED_STATE_HOST_MISSING:     return (led_pattern_t)PAT_WS2812(k_pat_double, B, 0, B); /* 紫 */
        case LED_STATE_READY:            return (led_pattern_t)PAT_WS2812(k_pat_solid,  0, B, 0); /* 绿 */
        default:                         return (led_pattern_t)PAT_WS2812(k_pat_slow,   0, 0, B);
    }
#else
    /* 单色 LED：无颜色语义，仅节奏 */
    switch (st) {
        case LED_STATE_WIFI_CONNECTING:
        case LED_STATE_LINK_LOST:        return (led_pattern_t)PAT_MONO(k_pat_fast);
        case LED_STATE_MQTT_CONNECTING:  return (led_pattern_t)PAT_MONO(k_pat_slow);
        case LED_STATE_HOST_MISSING:     return (led_pattern_t)PAT_MONO(k_pat_double);
        case LED_STATE_READY:            return (led_pattern_t)PAT_MONO(k_pat_solid);
        default:                         return (led_pattern_t)PAT_MONO(k_pat_slow);
    }
#endif
}

static bool pattern_on_at(const led_pattern_t *p, uint32_t tick) {
    return p->pat[tick % p->len] != 0;
}

/* ----------------------------------------------------------------
 * 硬件后端
 * ---------------------------------------------------------------- */
#if defined(CONFIG_SMART_HID_LED_TYPE_WS2812)
static led_strip_handle_t s_strip = NULL;

static int led_hw_init(void) {
    led_strip_config_t strip_cfg = {
        .strip_gpio_num   = CONFIG_SMART_HID_LED_GPIO,
        .max_leds         = 1,
        .led_model        = LED_MODEL_WS2812,
        .flags.invert_out = false,
    };
    led_strip_rmt_config_t rmt_cfg = {
        .clk_src        = RMT_CLK_SRC_DEFAULT,
        .resolution_hz  = 10 * 1000 * 1000,   /* 10MHz：WS2812 位宽 1.25us */
        .flags.with_dma = false,
    };
    esp_err_t rc = led_strip_new_rmt_device(&strip_cfg, &rmt_cfg, &s_strip);
    if (rc != ESP_OK) {
        ESP_LOGE(TAG, "led_strip_new_rmt_device failed: %s", esp_err_to_name(rc));
        return -1;
    }
    return 0;
}

static void led_hw_set(bool on, uint8_t r, uint8_t g, uint8_t b) {
    if (s_strip == NULL) return;
    if (led_strip_set_pixel(s_strip, 0, on ? r : 0, on ? g : 0, on ? b : 0) == ESP_OK) {
        led_strip_refresh(s_strip);
    }
}

#elif defined(CONFIG_SMART_HID_LED_TYPE_SIMPLE)
#define LED_GPIO        CONFIG_SMART_HID_LED_GPIO
#if defined(CONFIG_SMART_HID_LED_SIMPLE_ACTIVE_LOW)
#define LED_ON_LEVEL    0
#else
#define LED_ON_LEVEL    1
#endif

static int led_hw_init(void) {
    gpio_config_t io = {
        .pin_bit_mask = 1ULL << LED_GPIO,
        .mode         = GPIO_MODE_OUTPUT,
        .pull_up_en   = GPIO_PULLUP_DISABLE,
        .pull_down_en = GPIO_PULLDOWN_DISABLE,
        .intr_type    = GPIO_INTR_DISABLE,
    };
    esp_err_t rc = gpio_config(&io);
    if (rc != ESP_OK) {
        ESP_LOGE(TAG, "gpio_config(%d) failed: %s", LED_GPIO, esp_err_to_name(rc));
        return -1;
    }
    return 0;
}

static void led_hw_set(bool on, uint8_t r, uint8_t g, uint8_t b) {
    (void)r; (void)g; (void)b;
    gpio_set_level(LED_GPIO, on ? LED_ON_LEVEL : !LED_ON_LEVEL);
}

#else /* NONE */

static int led_hw_init(void) { return 0; }
static void led_hw_set(bool on, uint8_t r, uint8_t g, uint8_t b) {
    (void)on; (void)r; (void)g; (void)b;
}

#endif

/* ----------------------------------------------------------------
 * 状态轮询 + tick 任务
 * ---------------------------------------------------------------- */
static bool s_inited    = false;
static bool s_was_ready = false;                    /* 曾进入过 READY（区分首连 / 掉线） */
static volatile uint32_t s_pulse_until_ms = 0;      /* 命令脉冲截止时间 */

static uint32_t now_ms(void) {
    return (uint32_t)(esp_timer_get_time() / 1000);
}

static led_state_t poll_state(void) {
    bool wifi = wifi_manager_is_connected();
    bool mqtt = mqtt_manager_is_connected();
    bool usb  = hid_engine_is_ready();

    if (!wifi) return s_was_ready ? LED_STATE_LINK_LOST : LED_STATE_WIFI_CONNECTING;
    if (!mqtt) return LED_STATE_MQTT_CONNECTING;
    if (!usb)  return LED_STATE_HOST_MISSING;

    s_was_ready = true;
    return LED_STATE_READY;
}

static void led_task(void *arg) {
    (void)arg;
    uint32_t tick = 0;
    led_state_t last = LED_STATE_COUNT;
    static const char *k_state_names[LED_STATE_COUNT] = {
        "WIFI_CONNECTING", "LINK_LOST", "MQTT_CONNECTING", "HOST_MISSING", "READY",
    };

    while (true) {
        led_state_t st = poll_state();
        led_pattern_t p = state_pattern(st);

        if (st != last) {
            ESP_LOGI(TAG, "led state -> %s", k_state_names[st]);
            last = st;
            tick = 0;   /* 切状态从节拍头开始，避免错位 */
        }

        bool on = pattern_on_at(&p, tick);
        uint8_t r = p.r, g = p.g, b = p.b;

        /* 命令脉冲窗口：白色（单色 LED 反相）覆盖当前状态显示 */
        uint32_t now = now_ms();
        if (s_pulse_until_ms != 0) {
            if (now < s_pulse_until_ms) {
#if defined(CONFIG_SMART_HID_LED_TYPE_WS2812)
                uint16_t w = (uint16_t)WS_BRIGHTNESS * 2;   /* 脉冲用双倍亮度白光 */
                on = true;
                r = g = b = (w > 255) ? 255 : (uint8_t)w;
#else
                on = false;   /* 单色 LED：常亮状态下用短灭表现脉冲 */
#endif
            } else {
                s_pulse_until_ms = 0;
            }
        }

        led_hw_set(on, r, g, b);
        tick++;
        vTaskDelay(pdMS_TO_TICKS(LED_TICK_MS));
    }
}

/* ----------------------------------------------------------------
 * 公开 API
 * ---------------------------------------------------------------- */
int led_manager_init(void) {
    if (s_inited) return 0;

#if defined(CONFIG_SMART_HID_LED_TYPE_NONE)
    ESP_LOGI(TAG, "led disabled (SMART_HID_LED_TYPE=none)");
    s_inited = true;
    return 0;
#else
    if (led_hw_init() != 0) return -1;

    BaseType_t ok = xTaskCreate(led_task, "led", 3072, NULL, 3, NULL);
    if (ok != pdPASS) {
        ESP_LOGE(TAG, "create led task failed");
        return -1;
    }
    s_inited = true;
#if defined(CONFIG_SMART_HID_LED_TYPE_WS2812)
    ESP_LOGI(TAG, "led_manager started (type=ws2812 gpio=%d brightness=%d)",
             CONFIG_SMART_HID_LED_GPIO, WS_BRIGHTNESS);
#else
    ESP_LOGI(TAG, "led_manager started (type=simple gpio=%d)", CONFIG_SMART_HID_LED_GPIO);
#endif
    return 0;
#endif
}

void led_manager_pulse(void) {
    if (!s_inited) return;
    s_pulse_until_ms = now_ms() + CONFIG_SMART_HID_LED_PULSE_MS;
}
