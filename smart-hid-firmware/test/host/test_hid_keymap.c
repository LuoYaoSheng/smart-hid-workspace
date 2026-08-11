/**
 * test_hid_keymap.c — hid_keymap_lookup 的 host 单测。
 *
 * 覆盖：字母/数字/修饰键/特殊键/方向键/F键/编辑键/别名/大小写不敏感/无效输入。
 * HID Usage ID 事实源：USB HID Usage Tables 1.21 §10。
 */
#include "hid_keymap.h"
#include "test_framework.h"

void test_hid_keymap_all(void) {
    uint8_t usage = 0;
    bool is_mod = false;

    /* --- NULL / 空 --- */
    CHECK(!hid_keymap_lookup(NULL, &usage, &is_mod), "NULL → false");
    CHECK(!hid_keymap_lookup("", &usage, &is_mod), "空串 → false");

    /* --- 字母 a-z / A-Z（usage 0x04-0x1D） --- */
    CHECK(hid_keymap_lookup("a", &usage, &is_mod) && usage == 0x04 && !is_mod, "a → 0x04");
    CHECK(hid_keymap_lookup("Z", &usage, &is_mod) && usage == 0x1D && !is_mod, "Z → 0x1D");
    CHECK(hid_keymap_lookup("m", &usage, &is_mod) && usage == 0x10, "m → 0x10 (0x04+12)");

    /* --- 修饰键 --- */
    CHECK(hid_keymap_lookup("CTRL", &usage, &is_mod) && usage == HID_MOD_LCTRL && is_mod, "CTRL → LCTRL");
    CHECK(hid_keymap_lookup("CONTROL", &usage, &is_mod) && usage == HID_MOD_LCTRL, "CONTROL == CTRL 别名");
    CHECK(hid_keymap_lookup("SHIFT", &usage, &is_mod) && usage == HID_MOD_LSHIFT, "SHIFT → LSHIFT");
    CHECK(hid_keymap_lookup("ALT", &usage, &is_mod) && usage == HID_MOD_LALT, "ALT → LALT");
    CHECK(hid_keymap_lookup("OPTION", &usage, &is_mod) && usage == HID_MOD_LALT, "OPTION == ALT 别名");
    CHECK(hid_keymap_lookup("GUI", &usage, &is_mod) && usage == HID_MOD_LGUI, "GUI → LGUI");
    CHECK(hid_keymap_lookup("META", &usage, &is_mod) && usage == HID_MOD_LGUI, "META == GUI 别名");
    CHECK(hid_keymap_lookup("WIN", &usage, &is_mod) && usage == HID_MOD_LGUI, "WIN == GUI 别名");
    CHECK(hid_keymap_lookup("CMD", &usage, &is_mod) && usage == HID_MOD_LGUI, "CMD == GUI 别名");

    /* --- 特殊键 --- */
    CHECK(hid_keymap_lookup("ENTER", &usage, &is_mod) && usage == 0x28 && !is_mod, "ENTER → 0x28");
    CHECK(hid_keymap_lookup("RETURN", &usage, &is_mod) && usage == 0x28, "RETURN == ENTER 别名");
    CHECK(hid_keymap_lookup("ESC", &usage, &is_mod) && usage == 0x29, "ESC → 0x29");
    CHECK(hid_keymap_lookup("ESCAPE", &usage, &is_mod) && usage == 0x29, "ESCAPE == ESC 别名");
    CHECK(hid_keymap_lookup("TAB", &usage, &is_mod) && usage == 0x2B, "TAB → 0x2B");
    CHECK(hid_keymap_lookup("SPACE", &usage, &is_mod) && usage == 0x2C, "SPACE → 0x2C");
    CHECK(hid_keymap_lookup("BACKSPACE", &usage, &is_mod) && usage == 0x2A, "BACKSPACE → 0x2A");
    CHECK(hid_keymap_lookup("CAPSLOCK", &usage, &is_mod) && usage == 0x39, "CAPSLOCK → 0x39");
    CHECK(hid_keymap_lookup("CAPS", &usage, &is_mod) && usage == 0x39, "CAPS == CAPSLOCK 别名");

    /* --- 方向键 --- */
    CHECK(hid_keymap_lookup("LEFT", &usage, &is_mod) && usage == 0x50, "LEFT → 0x50");
    CHECK(hid_keymap_lookup("RIGHT", &usage, &is_mod) && usage == 0x51, "RIGHT → 0x51");
    CHECK(hid_keymap_lookup("UP", &usage, &is_mod) && usage == 0x52, "UP → 0x52");
    CHECK(hid_keymap_lookup("DOWN", &usage, &is_mod) && usage == 0x53, "DOWN → 0x53");

    /* --- F1-F12（0x3A-0x45） --- */
    CHECK(hid_keymap_lookup("F1", &usage, &is_mod) && usage == 0x3A, "F1 → 0x3A");
    CHECK(hid_keymap_lookup("F6", &usage, &is_mod) && usage == 0x3F, "F6 → 0x3F");
    CHECK(hid_keymap_lookup("F12", &usage, &is_mod) && usage == 0x45, "F12 → 0x45");

    /* --- 编辑键 --- */
    CHECK(hid_keymap_lookup("INSERT", &usage, &is_mod) && usage == 0x49, "INSERT → 0x49");
    CHECK(hid_keymap_lookup("INS", &usage, &is_mod) && usage == 0x49, "INS == INSERT 别名");
    CHECK(hid_keymap_lookup("HOME", &usage, &is_mod) && usage == 0x4A, "HOME → 0x4A");
    CHECK(hid_keymap_lookup("DELETE", &usage, &is_mod) && usage == 0x4C, "DELETE → 0x4C");
    CHECK(hid_keymap_lookup("DEL", &usage, &is_mod) && usage == 0x4C, "DEL == DELETE 别名");
    CHECK(hid_keymap_lookup("END", &usage, &is_mod) && usage == 0x4D, "END → 0x4D");
    CHECK(hid_keymap_lookup("PAGEUP", &usage, &is_mod) && usage == 0x4B, "PAGEUP → 0x4B");
    CHECK(hid_keymap_lookup("PGUP", &usage, &is_mod) && usage == 0x4B, "PGUP == PAGEUP 别名");
    CHECK(hid_keymap_lookup("PAGEDOWN", &usage, &is_mod) && usage == 0x4E, "PAGEDOWN → 0x4E");
    CHECK(hid_keymap_lookup("PGDN", &usage, &is_mod) && usage == 0x4E, "PGDN == PAGEDOWN 别名");

    /* --- 槽位数字键（usage 0x1E-0x27） --- */
    CHECK(hid_keymap_lookup("DIGIT0", &usage, &is_mod) && usage == 0x27, "DIGIT0 → 0x27");
    CHECK(hid_keymap_lookup("DIGIT1", &usage, &is_mod) && usage == 0x1E, "DIGIT1 → 0x1E");
    CHECK(hid_keymap_lookup("DIGIT9", &usage, &is_mod) && usage == 0x26, "DIGIT9 → 0x26");

    /* --- 大小写不敏感（strcasecmp） --- */
    CHECK(hid_keymap_lookup("enter", &usage, &is_mod) && usage == 0x28, "小写 enter");
    CHECK(hid_keymap_lookup("Enter", &usage, &is_mod) && usage == 0x28, "混合 Enter");
    CHECK(hid_keymap_lookup("Ctrl", &usage, &is_mod) && usage == HID_MOD_LCTRL, "混合 Ctrl");

    /* --- 无效输入 --- */
    CHECK(!hid_keymap_lookup("FOO", &usage, &is_mod), "无效 FOO → false");
    CHECK(!hid_keymap_lookup("1", &usage, &is_mod), "单字符数字 '1' 无效（非字母，表无）");
    CHECK(!hid_keymap_lookup("F13", &usage, &is_mod), "F13 超范围");
    CHECK(!hid_keymap_lookup("ENTER ", &usage, &is_mod), "带尾空格无效（精确匹配）");
    CHECK(!hid_keymap_lookup(" ENTER", &usage, &is_mod), "带头空格无效");
}
