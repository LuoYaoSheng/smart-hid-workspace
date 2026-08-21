/**
 * hid_engine.c — USB Composite HID 报告发送 + Lease + release_all
 *
 * 实现依据：docs/archive/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md §3-5
 *
 * 报告结构：
 *   Keyboard report（8 字节）：
 *     [0]    modifier bitmask
 *     [1]    reserved (0)
 *     [2-7]  current pressed key usage ids（最多 6 键并发）
 *   Mouse report（4 字节）：
 *     [0]    button bitmask (bit0=left bit1=right bit2=middle)
 *     [1]    dx (-127..127)
 *     [2]    dy (-127..127)
 *     [3]    wheel (-127..127)
 *
 * Lease：key_down / button_down 必须带 lease_ms；记入 lease 表，
 *        hid_engine_tick_leases 周期清理过期项 → 自动 release。
 *        MQTT/Wi-Fi 断开时 command_engine 调 hid_engine_release_all()。
 *
 * Fail-safe：release_all 幂等，多次调用安全。
 */
#include "hid_engine.h"
#include "hid_keymap.h"

#include <string.h>
#include <stdlib.h>
#include "sdkconfig.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"

#include "tusb.h"
#include "class/hid/hid_device.h"
#include "tinyusb.h"

static const char *TAG = "hid_engine";

/* ----------------------------------------------------------------
 * USB 描述符：Composite Keyboard + Mouse
 *
 * 通过 esp_tinyusb 的 tinyusb_driver_install 注册：
 *   - Device Descriptor（VID/PID/字符串索引）
 *   - Configuration Descriptor（2 个 Interface：Keyboard + Mouse，各占 1 个 IN EP）
 *   - Report Descriptor（Composite，含 report ID）
 *   - String Descriptors
 * ---------------------------------------------------------------- */

#define EPNUM_HID        0x81  /* 单接口单端点：HID IN */

#define REPORT_ID_KEYBOARD  CONFIG_SMART_HID_USB_HID_KEYBOARD_REPORT_ID
#define REPORT_ID_MOUSE     CONFIG_SMART_HID_USB_HID_MOUSE_REPORT_ID

/* 端点大小（HID IN 端点） */
#define CFG_TUD_HID_EP_BUFSIZE 64

/* Configuration Descriptor 总长度：
 *   1 config (9) + 1 interface(9+9+7=25) = 34
 *   使用 TUD_CONFIG_DESC_LEN + TUD_HID_DESC_LEN */
#define CONFIG_TOTAL_LEN  (TUD_CONFIG_DESC_LEN + TUD_HID_DESC_LEN)

/* HID Report Descriptor：单接口复合（键盘 + 鼠标，report ID 区分）
 *
 * 方案沿革（2026-08-20 真机迭代教训）：
 *  1. 双接口共用复合描述符 + 全部报告走 instance 0 → Windows 键盘可用、鼠标集合收不到输入
 *  2. 双接口各自专属描述符 + 按接口发报告 → Windows 枚举正常但 TinyUSB instance 1
 *     永不 ready（tud_hid_n_ready(1)=false，报告发不出）
 *  3. 【当前】单接口复合（report ID）+ 全走 instance 0 —— 键盘通路已真机验证，
 *     复合 report ID 是 TinyUSB/Pico/Arduino 生态最广泛验证的组合键盘鼠标形态
 */
static const uint8_t kHidReportDescriptor[] = {
    /* Keyboard Report (ID = REPORT_ID_KEYBOARD) */
    TUD_HID_REPORT_DESC_KEYBOARD(HID_REPORT_ID(REPORT_ID_KEYBOARD)),

    /* Mouse Report (ID = REPORT_ID_MOUSE) */
    TUD_HID_REPORT_DESC_MOUSE(HID_REPORT_ID(REPORT_ID_MOUSE)),
};

/* ----------------------------------------------------------------
 * USB Device Descriptor（tusb_desc_device_t）
 * VID/PID 用 Espressif 默认值，可在 menuconfig 覆盖。
 * ---------------------------------------------------------------- */
static const tusb_desc_device_t s_device_descriptor = {
    .bLength            = sizeof(tusb_desc_device_t),
    .bDescriptorType    = TUSB_DESC_DEVICE,
    .bcdUSB             = 0x0200,
    .bDeviceClass       = 0x00,
    .bDeviceSubClass    = 0x00,
    .bDeviceProtocol    = 0x00,
    .bMaxPacketSize0    = CFG_TUD_ENDPOINT0_SIZE,
    .idVendor           = 0x303A,   /* Espressif VID */
    .idProduct          = 0x4001,   /* 自定义 PID */
    .bcdDevice          = 0x0100,
    .iManufacturer      = 0x01,
    .iProduct           = 0x02,
    .iSerialNumber      = 0x03,
    .bNumConfigurations = 0x01
};

/* ----------------------------------------------------------------
 * String Descriptors
 * ---------------------------------------------------------------- */
static const char *s_string_descriptors[5] = {
    "",                     /* 0: 支持的语言（由 lang_id 描述符提供） */
    "Espressif",            /* 1: Manufacturer */
    "Smart HID Device",     /* 2: Product */
    "SMARTHID0001",         /* 3: Serial Number */
    "Smart HID Config",     /* 4: Configuration */
};

/* ----------------------------------------------------------------
 * Configuration Descriptor：2 Interface（Keyboard + Mouse）
 *   Interface 0: Keyboard (EP1 IN)
 *   Interface 1: Mouse (EP2 IN)
 * ---------------------------------------------------------------- */
static const uint8_t s_configuration_descriptor[] = {
    /* Config number, interface count, string index, total length, attribute, power in mA */
    TUD_CONFIG_DESCRIPTOR(1, /* interfaces */ 1, /* str idx */ 0,
                          CONFIG_TOTAL_LEN, TUSB_DESC_CONFIG_ATT_REMOTE_WAKEUP, 100),

    /* Interface 0: Composite HID（keyboard ID=2 / mouse ID=1，report ID 区分） */
    TUD_HID_DESCRIPTOR(/* itf */ 0, /* str */ 0, HID_ITF_PROTOCOL_NONE,
                       /* desc len */ sizeof(kHidReportDescriptor),
                       /* EP IN */ EPNUM_HID, /* EP size */ CFG_TUD_HID_EP_BUFSIZE,
                       /* poll interval */ 10),
};

/* tinyusb_driver 配置（esp_tinyusb 2.x：tinyusb_config_t）*/
static const tinyusb_config_t s_tusb_drv_cfg = {
    .port                      = TINYUSB_PORT_FULL_SPEED_0,
    .phy.skip_setup           = false,        /* 让 esp_tinyusb 自动配置内部 PHY */
    .phy.self_powered         = false,
    .task.size                = 4096,
    .task.priority            = 5,
    .task.xCoreID             = 0,            /* 必须显式核号：esp_tinyusb 直传 xTaskCreatePinnedToCore，-1 会触发核号断言（真机 2026-08-20 验证） */
    .descriptor.device        = &s_device_descriptor,
    .descriptor.string        = (const char **)s_string_descriptors,
    .descriptor.string_count  = sizeof(s_string_descriptors) / sizeof(s_string_descriptors[0]),
    .descriptor.full_speed_config = s_configuration_descriptor,
};


uint8_t const *tud_hid_descriptor_report_cb(uint8_t instance) {
    (void)instance;    /* 单接口：只有一个描述符 */
    return kHidReportDescriptor;
}

uint16_t tud_hid_get_report_cb(uint8_t instance, uint8_t report_id,
                               hid_report_type_t report_type,
                               uint8_t *buffer, uint16_t reqlen) {
    (void)instance; (void)report_id; (void)report_type; (void)buffer; (void)reqlen;
    return 0;
}

void tud_hid_set_report_cb(uint8_t instance, uint8_t report_id,
                           hid_report_type_t report_type,
                           uint8_t const *buffer, uint16_t bufsize) {
    (void)instance; (void)report_id; (void)report_type; (void)buffer; (void)bufsize;
}

/* ----------------------------------------------------------------
 * 当前 HID 状态（pressed keys / modifiers / mouse buttons + leases）
 * ---------------------------------------------------------------- */
typedef struct {
    uint8_t  usage;          /* 0 = 空槽 */
    uint32_t expire_ms;      /* 0 = 无 lease（key_up 已显式释放前持续按住） */
} pressed_key_slot_t;

typedef struct {
    uint8_t  button_mask;    /* bit0 left / bit1 right / bit2 middle */
    uint32_t expire_ms;      /* 0 = 无 lease */
} pressed_button_slot_t;

#define MAX_PRESSED_KEYS 6
static SemaphoreHandle_t s_lock = NULL;
static uint8_t s_keyboard_modifier = 0;                       /* modifier bitmask */
static pressed_key_slot_t s_pressed_keys[MAX_PRESSED_KEYS];
static pressed_button_slot_t s_pressed_buttons;               /* 单 slot 三 button */
static bool s_inited = false;

/* ----------------------------------------------------------------
 * 工具：当前 ms
 * ---------------------------------------------------------------- */
static uint32_t now_ms(void) {
    return (uint32_t)(esp_timer_get_time() / 1000);
}

static void ms_sleep(uint32_t ms) {
    vTaskDelay(pdMS_TO_TICKS(ms));
}

/* ----------------------------------------------------------------
 * 发送 Keyboard Report
 * ---------------------------------------------------------------- */
static void send_keyboard_report(uint8_t modifier, const uint8_t keys[6]) {
    if (!tud_mounted() || !tud_hid_n_ready(0)) return;
    uint8_t report[8] = {0};
    report[0] = modifier;
    if (keys) memcpy(&report[2], keys, 6);
    if (!tud_hid_n_report(0, REPORT_ID_KEYBOARD, report, sizeof(report))) {
        ESP_LOGW(TAG, "keyboard report send failed");
    }
}

static void send_mouse_report(uint8_t buttons, int8_t dx, int8_t dy, int8_t wheel) {
    if (!tud_mounted() || !tud_hid_n_ready(0)) {
        ESP_LOGW(TAG, "mouse send skipped: mounted=%d ready=%d",
                 (int)tud_mounted(), (int)tud_hid_n_ready(0));
        return;
    }
    /* 报告长度必须与 TUD_HID_REPORT_DESC_MOUSE 模板一致：该模板声明
     * [buttons 1B][X 1B][Y 1B][wheel 1B][AC_PAN 水平轮 1B] 共 5 字节。
     * 少发 1 字节会被 Windows 视为短报告静默丢弃（2026-08-20 真机教训）。 */
    uint8_t report[5] = {0};
    report[0] = buttons;
    report[1] = (uint8_t)dx;
    report[2] = (uint8_t)dy;
    report[3] = (uint8_t)wheel;
    /* report[4] = AC_PAN（水平滚轮），保持 0 */
    if (!tud_hid_n_report(0, REPORT_ID_MOUSE, report, sizeof(report))) {
        ESP_LOGW(TAG, "mouse report send failed");
    }
}

/* ----------------------------------------------------------------
 * pressed keys 管理
 * ---------------------------------------------------------------- */
static int pressed_keys_add(uint8_t usage, uint32_t lease_ms) {
    /* 找空槽或已存在同 usage 的槽 */
    for (int i = 0; i < MAX_PRESSED_KEYS; i++) {
        if (s_pressed_keys[i].usage == 0) {
            s_pressed_keys[i].usage      = usage;
            s_pressed_keys[i].expire_ms  = (lease_ms > 0) ? (now_ms() + lease_ms) : 0;
            return 0;
        }
    }
    return -1;  /* 满 */
}

static void pressed_keys_remove(uint8_t usage) {
    for (int i = 0; i < MAX_PRESSED_KEYS; i++) {
        if (s_pressed_keys[i].usage == usage) {
            s_pressed_keys[i].usage     = 0;
            s_pressed_keys[i].expire_ms = 0;
            return;
        }
    }
}

static void pressed_keys_flush_report(void) {
    uint8_t keys[6] = {0};
    int idx = 0;
    for (int i = 0; i < MAX_PRESSED_KEYS && idx < 6; i++) {
        if (s_pressed_keys[i].usage != 0) {
            keys[idx++] = s_pressed_keys[i].usage;
        }
    }
    send_keyboard_report(s_keyboard_modifier, keys);
}

/* ----------------------------------------------------------------
 * init / is_ready / release_all
 * ---------------------------------------------------------------- */
int hid_engine_init(void) {
    if (s_inited) return 0;

    /* 注册 USB Device 栈（esp_tinyusb）：自动完成 USB PHY / tusb_init /
     * Device/Config/String Descriptor 安装 / USB task 启动。*/
    if (!tusb_inited()) {
        esp_err_t rc = tinyusb_driver_install(&s_tusb_drv_cfg);
        if (rc != ESP_OK) {
            ESP_LOGE(TAG, "tinyusb_driver_install failed: %s", esp_err_to_name(rc));
            return -1;
        }
        ESP_LOGI(TAG, "tinyusb_driver_install ok (composite HID: keyboard+mouse)");
    }

    s_lock = xSemaphoreCreateMutex();
    if (s_lock == NULL) {
        ESP_LOGE(TAG, "create mutex failed");
        return -1;
    }
    memset(s_pressed_keys, 0, sizeof(s_pressed_keys));
    memset(&s_pressed_buttons, 0, sizeof(s_pressed_buttons));
    s_keyboard_modifier = 0;
    s_inited = true;
    ESP_LOGI(TAG, "hid_engine initialized");
    return 0;
}

bool hid_engine_is_ready(void) {
    return tud_mounted() && tud_hid_n_ready(0);
}

void hid_engine_release_all(void) {
    if (!s_inited || s_lock == NULL) return;
    xSemaphoreTake(s_lock, portMAX_DELAY);
    /* 立即发"全 0" 报告，先释放键再释放 modifier 再释放 button */
    if (s_keyboard_modifier != 0 ||
        s_pressed_keys[0].usage != 0 || s_pressed_keys[1].usage != 0 ||
        s_pressed_keys[2].usage != 0 || s_pressed_keys[3].usage != 0 ||
        s_pressed_keys[4].usage != 0 || s_pressed_keys[5].usage != 0) {
        uint8_t empty[6] = {0};
        send_keyboard_report(0, empty);
        memset(s_pressed_keys, 0, sizeof(s_pressed_keys));
        s_keyboard_modifier = 0;
    }
    if (s_pressed_buttons.button_mask != 0) {
        send_mouse_report(0, 0, 0, 0);
        s_pressed_buttons.button_mask = 0;
        s_pressed_buttons.expire_ms   = 0;
    }
    xSemaphoreGive(s_lock);
    ESP_LOGI(TAG, "release_all done");
}

/* ----------------------------------------------------------------
 * lease tick
 * ---------------------------------------------------------------- */
void hid_engine_tick_leases(uint32_t now) {
    if (!s_inited || s_lock == NULL) return;
    if (xSemaphoreTake(s_lock, 0) == pdFALSE) return;

    bool kbd_dirty = false;
    for (int i = 0; i < MAX_PRESSED_KEYS; i++) {
        if (s_pressed_keys[i].usage != 0 &&
            s_pressed_keys[i].expire_ms != 0 &&
            now >= s_pressed_keys[i].expire_ms) {
            ESP_LOGI(TAG, "key lease expired: usage=0x%02X", s_pressed_keys[i].usage);
            s_pressed_keys[i].usage     = 0;
            s_pressed_keys[i].expire_ms = 0;
            kbd_dirty = true;
        }
    }
    /* modifier 没 lease（modifier 总是伴随 key_down，key lease 过期即一起清） */
    if (kbd_dirty) {
        /* 若所有 key 都释放了，modifier 也清零 */
        bool any = false;
        for (int i = 0; i < MAX_PRESSED_KEYS; i++) {
            if (s_pressed_keys[i].usage != 0) { any = true; break; }
        }
        if (!any && s_keyboard_modifier != 0) {
            s_keyboard_modifier = 0;
        }
        pressed_keys_flush_report();
    }

    if (s_pressed_buttons.button_mask != 0 &&
        s_pressed_buttons.expire_ms != 0 &&
        now >= s_pressed_buttons.expire_ms) {
        ESP_LOGI(TAG, "mouse button lease expired: mask=0x%02X", s_pressed_buttons.button_mask);
        send_mouse_report(0, 0, 0, 0);
        s_pressed_buttons.button_mask = 0;
        s_pressed_buttons.expire_ms   = 0;
    }

    xSemaphoreGive(s_lock);
}

/* ----------------------------------------------------------------
 * 内部：执行 keyboard
 * ---------------------------------------------------------------- */
static int execute_keyboard(const smart_hid_command_t *cmd, uint32_t *exec_ms_out) {
    int rc = SMART_HID_CODE_OK;
    uint8_t modifiers = 0;
    uint8_t normal_usages[6];
    int     n_normal = 0;

    /* 把 keys（hotkey）或 key（tap）转成 usage 数组 */
    const char *sources[8];
    int n_sources = 0;
    if (cmd->keyboard.keys_count > 0) {
        for (int i = 0; i < cmd->keyboard.keys_count && n_sources < 8; i++) {
            if (cmd->keyboard.keys[i][0]) sources[n_sources++] = cmd->keyboard.keys[i];
        }
    } else if (cmd->keyboard.key[0]) {
        sources[n_sources++] = cmd->keyboard.key;
    }

    for (int i = 0; i < n_sources; i++) {
        uint8_t usage; bool is_mod;
        if (!hid_keymap_lookup(sources[i], &usage, &is_mod)) {
            ESP_LOGW(TAG, "unknown key name: %s", sources[i]);
            return SMART_HID_CODE_REJECTED_BAD_REQUEST;
        }
        if (is_mod) {
            modifiers |= usage;
        } else {
            if (n_normal >= 6) return SMART_HID_CODE_REJECTED_BAD_REQUEST;
            normal_usages[n_normal++] = usage;
        }
    }

    uint32_t hold_ms = cmd->keyboard.hold_ms > 0 ? cmd->keyboard.hold_ms : 40;
    uint32_t t0 = now_ms();

    xSemaphoreTake(s_lock, portMAX_DELAY);
    switch (cmd->action) {
        case SMART_HID_ACTION_TAP:
        case SMART_HID_ACTION_HOTKEY: {
            /* 报告 modifier+keys → 等 hold_ms → 全 0 */
            uint8_t keys[6] = {0};
            memcpy(keys, normal_usages, n_normal);
            send_keyboard_report(modifiers | s_keyboard_modifier, keys);
            ms_sleep(hold_ms);
            /* 仅释放本次按下的 normal keys + 本次 modifiers（保留其它已 pressed） */
            memset(keys, 0, sizeof(keys));
            int idx = 0;
            for (int i = 0; i < MAX_PRESSED_KEYS && idx < 6; i++) {
                if (s_pressed_keys[i].usage != 0) keys[idx++] = s_pressed_keys[i].usage;
            }
            send_keyboard_report(s_keyboard_modifier, keys);
            break;
        }
        case SMART_HID_ACTION_KEY_DOWN:
            /* 加入 pressed 表 + 更新 modifier */
            s_keyboard_modifier |= modifiers;
            for (int i = 0; i < n_normal; i++) {
                pressed_keys_add(normal_usages[i], cmd->keyboard.lease_ms);
            }
            pressed_keys_flush_report();
            if (cmd->keyboard.lease_ms == 0) {
                /* 文档要求 key_down 必须带 lease_ms；防呆：若未带，按 5s 默认 */
                ESP_LOGW(TAG, "key_down without lease_ms, applying default 5000ms");
                for (int i = 0; i < n_normal; i++) {
                    pressed_keys_add(normal_usages[i], 5000);
                }
            }
            break;
        case SMART_HID_ACTION_KEY_UP:
            for (int i = 0; i < n_normal; i++) {
                pressed_keys_remove(normal_usages[i]);
            }
            s_keyboard_modifier &= (uint8_t)~modifiers;
            pressed_keys_flush_report();
            break;
        default:
            rc = SMART_HID_CODE_REJECTED_BAD_REQUEST;
            break;
    }
    xSemaphoreGive(s_lock);

    if (exec_ms_out) *exec_ms_out = now_ms() - t0;
    return rc;
}

/* ----------------------------------------------------------------
 * 内部：执行 mouse
 * ---------------------------------------------------------------- */
static int mouse_button_mask(const char *name) {
    if (strcasecmp(name, "LEFT")   == 0) return 0x01;
    if (strcasecmp(name, "RIGHT")  == 0) return 0x02;
    if (strcasecmp(name, "MIDDLE") == 0) return 0x04;
    return 0;
}

static int execute_mouse(const smart_hid_command_t *cmd, uint32_t *exec_ms_out) {
    int rc = SMART_HID_CODE_OK;
    uint32_t t0 = now_ms();

    xSemaphoreTake(s_lock, portMAX_DELAY);
    switch (cmd->action) {
        case SMART_HID_ACTION_MOVE: {
            int32_t remain_x = cmd->mouse.dx;
            int32_t remain_y = cmd->mouse.dy;
            /* 报告间隔必须大于端点 bInterval(10ms)：恰好相等时存在边界竞态
             * （host 轮询与下一次提交同相位时前段报告可能丢失，真机 2026-08-20
             * 观测：4 段仅后 2 段生效）。取 15ms 错开相位。 */
            while (remain_x != 0 || remain_y != 0) {
                int8_t step_x = (int8_t)((remain_x > 127) ? 127 :
                                         (remain_x < -127) ? -127 : remain_x);
                int8_t step_y = (int8_t)((remain_y > 127) ? 127 :
                                         (remain_y < -127) ? -127 : remain_y);
                send_mouse_report(s_pressed_buttons.button_mask, step_x, step_y, 0);
                remain_x -= step_x;
                remain_y -= step_y;
                if (remain_x != 0 || remain_y != 0) ms_sleep(15);
            }
            break;
        }
        case SMART_HID_ACTION_CLICK: {
            int count = cmd->mouse.count > 0 ? cmd->mouse.count : 1;
            int mask = cmd->mouse.button[0] ? mouse_button_mask(cmd->mouse.button) : 0x01;
            if (mask == 0) { rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break; }
            for (int i = 0; i < count; i++) {
                send_mouse_report(s_pressed_buttons.button_mask | mask, 0, 0, 0);
                ms_sleep(15);
                send_mouse_report(s_pressed_buttons.button_mask, 0, 0, 0);
                if (i + 1 < count) ms_sleep(15);
            }
            break;
        }
        case SMART_HID_ACTION_BUTTON_DOWN: {
            int mask = cmd->mouse.button[0] ? mouse_button_mask(cmd->mouse.button) : 0x01;
            if (mask == 0) { rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break; }
            s_pressed_buttons.button_mask |= (uint8_t)mask;
            s_pressed_buttons.expire_ms =
                (cmd->mouse.lease_ms > 0) ? (now_ms() + cmd->mouse.lease_ms) :
                (now_ms() + 5000);  /* lease_ms 未带 → 默认 5s */
            if (cmd->mouse.lease_ms == 0) {
                ESP_LOGW(TAG, "button_down without lease_ms, default 5000ms");
            }
            send_mouse_report(s_pressed_buttons.button_mask, 0, 0, 0);
            break;
        }
        case SMART_HID_ACTION_BUTTON_UP: {
            int mask = cmd->mouse.button[0] ? mouse_button_mask(cmd->mouse.button) : 0x01;
            s_pressed_buttons.button_mask &= (uint8_t)~mask;
            s_pressed_buttons.expire_ms = 0;
            send_mouse_report(s_pressed_buttons.button_mask, 0, 0, 0);
            break;
        }
        case SMART_HID_ACTION_WHEEL:
            send_mouse_report(s_pressed_buttons.button_mask, 0, 0, (int8_t)cmd->mouse.delta);
            break;
        default:
            rc = SMART_HID_CODE_REJECTED_BAD_REQUEST;
            break;
    }
    xSemaphoreGive(s_lock);

    if (exec_ms_out) *exec_ms_out = now_ms() - t0;
    return rc;
}

/* ----------------------------------------------------------------
 * hid_engine_execute 入口
 * ---------------------------------------------------------------- */
int hid_engine_execute(const smart_hid_command_t *cmd, uint32_t *exec_ms_out) {
    if (cmd == NULL) return SMART_HID_CODE_REJECTED_BAD_REQUEST;
    if (!s_inited) {
        ESP_LOGE(TAG, "hid_engine not inited");
        return SMART_HID_CODE_REJECTED_HID_BUSY;
    }

    /* system / release_all */
    if (cmd->type == SMART_HID_TYPE_SYSTEM) {
        if (cmd->action == SMART_HID_ACTION_RELEASE_ALL) {
            uint32_t t0 = now_ms();
            hid_engine_release_all();
            if (exec_ms_out) *exec_ms_out = now_ms() - t0;
            return SMART_HID_CODE_OK;
        }
        return SMART_HID_CODE_REJECTED_BAD_REQUEST;
    }

    if (!hid_engine_is_ready()) {
        ESP_LOGW(TAG, "USB HID not ready, reject");
        return SMART_HID_CODE_REJECTED_HID_BUSY;
    }

    if (cmd->type == SMART_HID_TYPE_KEYBOARD) return execute_keyboard(cmd, exec_ms_out);
    if (cmd->type == SMART_HID_TYPE_MOUSE)    return execute_mouse(cmd, exec_ms_out);
    return SMART_HID_CODE_REJECTED_BAD_REQUEST;
}
