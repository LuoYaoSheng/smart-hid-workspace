/**
 * hid_keymap.h — Key name → HID Usage ID 映射
 *
 * 来源：USB HID Usage Tables 1.21 §10 (Keyboard/Keypad Page 0x07)
 * 覆盖 docs/04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md §6 列举的所有 key 名。
 */
#pragma once

#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/**
 * 查 key 名（如 "ENTER" / "A" / "F1" / "CTRL"）对应的 HID Usage ID。
 *
 * Modifier 名（CTRL/SHIFT/ALT/GUI/META/WIN/CMD）返回的是 modifier bitmask，
 * 通过 *is_modifier_out 标识；普通键返回 Keycode Page (0x07) usage。
 *
 * @param name              key 名（大小写不敏感）
 * @param usage_out         输出 usage id
 * @param is_modifier_out   输出：是否 modifier
 * @return true 命中；false 未命中（无效 key 名）
 */
bool hid_keymap_lookup(const char *name, uint8_t *usage_out, bool *is_modifier_out);

/**
 * HID Keyboard modifier bitmask（HID report byte 0）。
 */
#define HID_MOD_LCTRL  0x01
#define HID_MOD_LSHIFT 0x02
#define HID_MOD_LALT   0x04
#define HID_MOD_LGUI   0x08
#define HID_MOD_RCTRL  0x10
#define HID_MOD_RSHIFT 0x20
#define HID_MOD_RALT   0x40
#define HID_MOD_RGUI   0x80

#ifdef __cplusplus
}
#endif
