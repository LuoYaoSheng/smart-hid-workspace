/**
 * hid_engine.c — USB Composite HID 报告发送 + Lease + release_all
 *
 * 实现依据：docs/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md §3-5
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
#include "esp_log.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"

#include "tusb.h"
#include "class/hid/hid_device.h"
#include "device/usbd.h"

static const char *TAG = "hid_engine";

/* ----------------------------------------------------------------
 * USB 描述符：Composite Keyboard + Mouse
 *
 * 通过 TinyUSB 的 tusb_hid_descriptor_device_cb 提供配置描述符。
 * F1/F2 阶段直接使用 TinyUSB HID class device 接口。
 * ---------------------------------------------------------------- */

#define EPNUM_KEYBOARD   0x81  /* EP1 IN */
#define EPNUM_MOUSE      0x82  /* EP2 IN */

#define REPORT_ID_KEYBOARD  CONFIG_SMART_HID_USB_HID_KEYBOARD_REPORT_ID
#define REPORT_ID_MOUSE     CONFIG_SMART_HID_USB_HID_MOUSE_REPORT_ID

/* HID Report Descriptor（Composite，含 report ID） */
static const uint8_t kHidReportDescriptor[] = {
    /* Keyboard Report (ID = REPORT_ID_KEYBOARD) */
    0x85, REPORT_ID_KEYBOARD,        /*   Report ID */
    HID_USAGE_PAGE ( HID_USAGE_PAGE_DESKTOP     ),
    HID_USAGE      ( HID_USAGE_DESKTOP_KEYBOARD ),
    HID_COLLECTION ( HID_COLLECTION_APPLICATION ),
        HID_REPORT_ID   ( REPORT_ID_KEYBOARD            ),
        HID_USAGE_PAGE  ( HID_USAGE_PAGE_KEYBOARD        ),
          0x19, 0xE0,  /*   Usage Minimum (0xE0 Left Control) */
          0x29, 0xE7,  /*   Usage Maximum (0xE7 Right GUI)    */
          0x15, 0x00,  /*   Logical Minimum (0)               */
          0x25, 0x01,  /*   Logical Maximum (1)               */
          0x75, 0x01,  /*   Report Size (1)                   */
          0x95, 0x08,  /*   Report Count (8)                  */
          0x81, 0x02,  /*   Input (Data,Var,Abs)              */
          0x95, 0x01,  /*   Report Count (1)                  */
          0x75, 0x08,  /*   Report Size (8)                   */
          0x81, 0x03,  /*   Input (Const,Var,Abs) [reserved]  */
          0x95, 0x05,  /*   Report Count (5)                  */
          0x75, 0x01,  /*   Report Size (1)                   */
          0x05, 0x08,  /*   Usage Page (LEDs)                 */
          0x19, 0x01,  /*   Usage Minimum                     */
          0x29, 0x05,  /*   Usage Maximum                     */
          0x91, 0x02,  /*   Output (Data,Var,Abs)             */
          0x95, 0x01,  /*   Report Count (1)                  */
          0x75, 0x03,  /*   Report Size (3)                   */
          0x91, 0x03,  /*   Output (Const,Var,Abs)            */
          0x95, 0x06,  /*   Report Count (6)                  */
          0x75, 0x08,  /*   Report Size (8)                   */
          0x15, 0x00,  /*   Logical Minimum (0)               */
          0x25, 0xE7,  /*   Logical Maximum (231)             */
          0x05, 0x07,  /*   Usage Page (Keyboard)             */
          0x19, 0x00,  /*   Usage Minimum                     */
          0x29, 0xE7,  /*   Usage Maximum                     */
          0x81, 0x00,  /*   Input (Data,Array,Abs)            */
    HID_COLLECTION_END,

    /* Mouse Report (ID = REPORT_ID_MOUSE) */
    0x85, REPORT_ID_MOUSE,           /*   Report ID */
    HID_USAGE_PAGE  ( HID_USAGE_PAGE_DESKTOP   ),
    HID_USAGE       ( HID_USAGE_DESKTOP_MOUSE  ),
    HID_COLLECTION  ( HID_COLLECTION_APPLICATION),
        HID_REPORT_ID    ( REPORT_ID_MOUSE             ),
        HID_USAGE        ( HID_USAGE_DESKTOP_POINTER    ),
        HID_COLLECTION   ( HID_COLLECTION_PHYSICAL      ),
            HID_USAGE_PAGE  ( HID_USAGE_PAGE_BUTTON ),
              0x19, 0x01,  /*   Usage Minimum (1)             */
              0x29, 0x03,  /*   Usage Maximum (3)             */
              0x15, 0x00,  /*   Logical Minimum (0)           */
              0x25, 0x01,  /*   Logical Maximum (1)           */
              0x95, 0x03,  /*   Report Count (3)              */
              0x75, 0x01,  /*   Report Size (1)               */
              0x81, 0x02,  /*   Input (Data,Var,Abs)          */
              0x95, 0x01,  /*   Report Count (1)              */
              0x75, 0x05,  /*   Report Size (5)               */
              0x81, 0x03,  /*   Input (Const,Var,Abs) [pad]   */
            HID_USAGE_PAGE  ( HID_USAGE_PAGE_DESKTOP ),
              0x09, 0x30,  /*   Usage (X)                     */
              0x09, 0x31,  /*   Usage (Y)                     */
              0x09, 0x38,  /*   Usage (Wheel)                 */
              0x15, 0x81,  /*   Logical Minimum (-127)        */
              0x25, 0x7F,  /*   Logical Maximum (127)         */
              0x75, 0x08,  /*   Report Size (8)               */
              0x95, 0x03,  /*   Report Count (3)              */
              0x81, 0x06,  /*   Input (Data,Var,Rel)          */
        HID_COLLECTION_END,
    HID_COLLECTION_END,
};

/* ----------------------------------------------------------------
 * TinyUSB HID device 回调（描述符 + 报告长度）
 * ---------------------------------------------------------------- */
uint8_t const *tud_hid_descriptor_report_cb(uint8_t instance) {
    (void)instance;
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
    if (!tud_mounted() || !tud_hid_ready()) return;
    uint8_t report[8] = {0};
    report[0] = modifier;
    if (keys) memcpy(&report[2], keys, 6);
    tud_hid_report(REPORT_ID_KEYBOARD, report, sizeof(report));
}

static void send_mouse_report(uint8_t buttons, int8_t dx, int8_t dy, int8_t wheel) {
    if (!tud_mounted() || !tud_hid_ready()) return;
    uint8_t report[4];
    report[0] = buttons;
    report[1] = (uint8_t)dx;
    report[2] = (uint8_t)dy;
    report[3] = (uint8_t)wheel;
    tud_hid_report(REPORT_ID_MOUSE, report, sizeof(report));
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
    return tud_mounted() && tud_hid_ready();
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
            while (remain_x != 0 || remain_y != 0) {
                int8_t step_x = (int8_t)((remain_x > 127) ? 127 :
                                         (remain_x < -127) ? -127 : remain_x);
                int8_t step_y = (int8_t)((remain_y > 127) ? 127 :
                                         (remain_y < -127) ? -127 : remain_y);
                send_mouse_report(s_pressed_buttons.button_mask, step_x, step_y, 0);
                remain_x -= step_x;
                remain_y -= step_y;
                if (remain_x != 0 || remain_y != 0) ms_sleep(5);  /* 避免合并 */
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
