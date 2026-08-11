/**
 * hid_keymap.c — Key name → HID Usage ID
 *
 * 覆盖 ControlHub 文档 §6 全部 key 名 + 常见补充。
 * 未命中的 key 在 hid_engine 内以 warning 日志记录但不阻断流程。
 */
#include "hid_keymap.h"
#include <string.h>
#include <strings.h>  /* strcasecmp */

typedef struct {
    const char *name;
    uint8_t     usage;        /* HID Usage ID（Keyboard/Keypad Page 0x07），或 modifier bitmask */
    bool        is_modifier;
} keymap_entry_t;

/* 字母 + 数字（usage 0x04-0x1D）直接走快速路径，不进表 */

static const keymap_entry_t kTable[] = {
    /* === Modifiers === */
    { "CTRL",    HID_MOD_LCTRL,  true },
    { "CONTROL", HID_MOD_LCTRL,  true },
    { "SHIFT",   HID_MOD_LSHIFT, true },
    { "ALT",     HID_MOD_LALT,   true },
    { "GUI",     HID_MOD_LGUI,   true },
    { "META",    HID_MOD_LGUI,   true },
    { "WIN",     HID_MOD_LGUI,   true },
    { "CMD",     HID_MOD_LGUI,   true },
    { "OPTION",  HID_MOD_LALT,   true },

    /* === Special / Function / Arrows（HID Usage Tables §10） === */
    { "ENTER",   0x28, false },  /* Keyboard Enter (Return) */
    { "RETURN",  0x28, false },
    { "ESC",     0x29, false },
    { "ESCAPE",  0x29, false },
    { "BACKSPACE", 0x2A, false },
    { "TAB",     0x2B, false },
    { "SPACE",   0x2C, false },
    { "CAPSLOCK",0x39, false },
    { "CAPS",    0x39, false },

    /* 方向 */
    { "LEFT",    0x50, false },
    { "RIGHT",   0x51, false },
    { "UP",      0x52, false },
    { "DOWN",    0x53, false },

    /* F1-F12 */
    { "F1",  0x3A, false }, { "F2",  0x3B, false },
    { "F3",  0x3C, false }, { "F4",  0x3D, false },
    { "F5",  0x3E, false }, { "F6",  0x3F, false },
    { "F7",  0x40, false }, { "F8",  0x41, false },
    { "F9",  0x42, false }, { "F10", 0x43, false },
    { "F11", 0x44, false }, { "F12", 0x45, false },

    /* 编辑 */
    { "INSERT",    0x49, false },
    { "INS",       0x49, false },
    { "HOME",      0x4A, false },
    { "PAGEUP",    0x4B, false },
    { "PGUP",      0x4B, false },
    { "DELETE",    0x4C, false },
    { "DEL",       0x4C, false },
    { "END",       0x4D, false },
    { "PAGEDOWN",  0x4E, false },
    { "PGDN",      0x4E, false },

    /* 槽位数字键 1-0（usage 0x1E-0x27）—— 显式名避免与字母 a-z 冲突 */
    { "DIGIT0", 0x27, false }, { "DIGIT1", 0x1E, false },
    { "DIGIT2", 0x1F, false }, { "DIGIT3", 0x20, false },
    { "DIGIT4", 0x21, false }, { "DIGIT5", 0x22, false },
    { "DIGIT6", 0x23, false }, { "DIGIT7", 0x24, false },
    { "DIGIT8", 0x25, false }, { "DIGIT9", 0x26, false },
};

bool hid_keymap_lookup(const char *name, uint8_t *usage_out, bool *is_modifier_out) {
    if (name == NULL || name[0] == '\0') return false;

    /* 单字母 a-z → usage 0x04 + (c - 'a') */
    if (strlen(name) == 1) {
        char c = name[0];
        if (c >= 'a' && c <= 'z') {
            *usage_out = (uint8_t)(0x04 + (c - 'a'));
            *is_modifier_out = false;
            return true;
        }
        if (c >= 'A' && c <= 'Z') {
            *usage_out = (uint8_t)(0x04 + (c - 'A'));
            *is_modifier_out = false;
            return true;
        }
    }

    /* 表查询 */
    size_t n = sizeof(kTable) / sizeof(kTable[0]);
    for (size_t i = 0; i < n; i++) {
        if (strcasecmp(name, kTable[i].name) == 0) {
            *usage_out       = kTable[i].usage;
            *is_modifier_out = kTable[i].is_modifier;
            return true;
        }
    }
    return false;
}
