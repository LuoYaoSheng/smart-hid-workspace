/**
 * smart_hid_protocol.h — Smart HID MQTT Command/Ack/Status/Event 协议
 *
 * 事实源：smart-ble/core/protocols/hid-command-schema.ts
 * 依据：docs/archive/04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md
 *
 * 任何字段/常量/枚举变更必须先改 TS 事实源，再同步本文件。
 */
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ============================================================
 * 协议版本
 * ============================================================ */
#define SMART_HID_PROTOCOL_VERSION "1.0"

/* ============================================================
 * Topic 模板（{device_id} 占位）
 * ============================================================ */
#define SMART_HID_TOPIC_BASE        "smart-hid/v1/devices"
#define SMART_HID_TOPIC_COMMAND_FMT SMART_HID_TOPIC_BASE "/%s/command"  /* retain=false */
#define SMART_HID_TOPIC_ACK_FMT     SMART_HID_TOPIC_BASE "/%s/ack"      /* retain=false */
#define SMART_HID_TOPIC_STATUS_FMT  SMART_HID_TOPIC_BASE "/%s/status"   /* retain=true  */
#define SMART_HID_TOPIC_EVENT_FMT   SMART_HID_TOPIC_BASE "/%s/event"    /* retain=false */

/* ============================================================
 * 协议常量（来自 hid-command-schema.ts COMMAND_CONSTANTS）
 * ============================================================ */
#define SMART_HID_COMMAND_QUEUE_SIZE   32
#define SMART_HID_PAYLOAD_MAX_BYTES    2048
#define SMART_HID_DEDUP_CACHE_SIZE     256
#define SMART_HID_TTL_MS_MIN           100
#define SMART_HID_TTL_MS_MAX           10000
#define SMART_HID_REQUEST_ID_MAX_LEN   96
#define SMART_HID_DEVICE_ID_MAX_LEN    16   /* "HID-XXXXXXXX" + '\0' */
#define SMART_HID_BOOT_ID_MAX_LEN      16   /* "B-XXXXXX" + '\0' */
#define SMART_HID_STALE_DEVICE_SESSION "STALE_DEVICE_SESSION"

/* ============================================================
 * 类型枚举
 * ============================================================ */
typedef enum {
    SMART_HID_TYPE_KEYBOARD = 0,
    SMART_HID_TYPE_MOUSE    = 1,
    SMART_HID_TYPE_SYSTEM   = 2
} smart_hid_type_t;

typedef enum {
    /* keyboard */
    SMART_HID_ACTION_TAP     = 0,  /* keyboard */
    SMART_HID_ACTION_HOTKEY  = 1,  /* keyboard */
    SMART_HID_ACTION_KEY_DOWN = 2, /* keyboard，必须带 lease_ms */
    SMART_HID_ACTION_KEY_UP   = 3, /* keyboard */
    /* mouse */
    SMART_HID_ACTION_MOVE       = 10, /* mouse */
    SMART_HID_ACTION_CLICK      = 11, /* mouse */
    SMART_HID_ACTION_BUTTON_DOWN = 12, /* mouse，必须带 lease_ms */
    SMART_HID_ACTION_BUTTON_UP   = 13, /* mouse */
    SMART_HID_ACTION_WHEEL      = 14, /* mouse */
    /* system */
    SMART_HID_ACTION_RELEASE_ALL = 20, /* system */
    SMART_HID_ACTION_INVALID    = 0xFF
} smart_hid_action_t;

/* ============================================================
 * ACK 状态
 * ============================================================ */
typedef enum {
    SMART_HID_ACK_RECEIVED  = 0,
    SMART_HID_ACK_EXECUTING = 1,
    SMART_HID_ACK_EXECUTED  = 2,
    SMART_HID_ACK_REJECTED  = 3,
    SMART_HID_ACK_EXPIRED   = 4,
    SMART_HID_ACK_DUPLICATE = 5
} smart_hid_ack_status_t;

/* ACK 错误码（非 0 即失败） */
#define SMART_HID_CODE_OK                       0
#define SMART_HID_CODE_REJECTED_STALE_BOOT      4001
#define SMART_HID_CODE_REJECTED_BAD_REQUEST     4002
#define SMART_HID_CODE_REJECTED_QUEUE_FULL      4003
#define SMART_HID_CODE_REJECTED_PAYLOAD_TOO_BIG 4004
#define SMART_HID_CODE_REJECTED_HID_BUSY        4005
#define SMART_HID_CODE_EXPIRED                  5001

/* ============================================================
 * 数据结构
 * ============================================================ */

/* Keyboard payload 子字段（解析后保存在 command 中） */
typedef struct {
    /* tap / hotkey 时使用：key 单键；keys 多键组合 */
    char        key[8];                 /* 单键名，如 "ENTER" / "A" / "F1"，"" 表示未设 */
    char        keys[8][8];             /* 多键组合，最多 8 键 */
    uint8_t     keys_count;
    uint32_t    hold_ms;                /* 默认 40 */
    uint32_t    lease_ms;               /* key_down 必须带 */
} smart_hid_keyboard_payload_t;

/* Mouse payload 子字段 */
typedef struct {
    int32_t     dx;
    int32_t     dy;
    char        button[8];              /* "LEFT" / "RIGHT" / "MIDDLE"，"" 表示未设 */
    uint8_t     count;                  /* click 次数，默认 1 */
    int32_t     delta;                  /* wheel 增量 */
    uint32_t    lease_ms;               /* button_down 必须带 */
} smart_hid_mouse_payload_t;

/**
 * Smart HID Command envelope（完整解析后）
 *
 * payload 原始字节保存在 payload_json（≤ PAYLOAD_MAX_BYTES），
 * 解析后的子字段按 type 分存 keyboard / mouse 联合语义字段。
 */
typedef struct {
    char        protocol[8];            /* "1.0" */
    char        request_id[SMART_HID_REQUEST_ID_MAX_LEN + 1];
    char        device_id[SMART_HID_DEVICE_ID_MAX_LEN + 1];
    char        target_boot_id[SMART_HID_BOOT_ID_MAX_LEN + 1];
    smart_hid_type_t   type;
    smart_hid_action_t action;
    uint32_t    ttl_ms;
    /* 原始 payload JSON（用于诊断/日志） */
    char        payload_json[SMART_HID_PAYLOAD_MAX_BYTES + 1];
    /* 解析后子字段 */
    smart_hid_keyboard_payload_t keyboard;
    smart_hid_mouse_payload_t    mouse;
} smart_hid_command_t;

/**
 * Smart HID ACK envelope
 */
typedef struct {
    char        protocol[8];
    char        request_id[SMART_HID_REQUEST_ID_MAX_LEN + 1];
    char        device_id[SMART_HID_DEVICE_ID_MAX_LEN + 1];
    char        boot_id[SMART_HID_BOOT_ID_MAX_LEN + 1];
    smart_hid_ack_status_t status;
    int         code;
    uint32_t    execution_ms;           /* 仅 executed 时填，序列化时若 0 省略 */
} smart_hid_ack_t;

/**
 * Smart HID Status envelope
 */
typedef struct {
    char        protocol[8];
    char        device_id[SMART_HID_DEVICE_ID_MAX_LEN + 1];
    bool        online;
    char        boot_id[SMART_HID_BOOT_ID_MAX_LEN + 1];
    bool        usb_hid_ready;
    char        firmware[32];
    int64_t     timestamp;              /* Unix 秒 */
} smart_hid_status_t;

/**
 * Smart HID Event envelope
 */
typedef struct {
    char        protocol[8];
    char        device_id[SMART_HID_DEVICE_ID_MAX_LEN + 1];
    char        event[32];              /* 'release_all_triggered' / 'queue_full' / 'mqtt_disconnected' 等 */
    char        detail[128];            /* JSON 对象字符串（可为 "{}"） */
    int64_t     timestamp;
} smart_hid_event_t;

/* ============================================================
 * 公开函数
 * ============================================================ */

/**
 * 解析 MQTT command JSON 为 smart_hid_command_t。
 *
 * @param json     JSON 字符串（无需以 \0 结尾，长度由 json_len 提供）
 * @param json_len 字节长度
 * @param out      输出
 * @return 0 成功；非 0 失败（code 用 SMART_HID_CODE_REJECTED_BAD_REQUEST）
 */
int smart_hid_parse_command(const char *json, size_t json_len, smart_hid_command_t *out);

/**
 * 构造 ACK JSON 字符串（调用方负责释放 *out_buf）。
 *
 * @param ack       ACK 结构
 * @param out_buf   输出 malloc 缓冲区
 * @param out_len   输出 JSON 长度（不含 \0）
 * @return 0 成功；-1 失败
 */
int smart_hid_build_ack_json(const smart_hid_ack_t *ack, char **out_buf, size_t *out_len);

/**
 * 构造 Status JSON。
 */
int smart_hid_build_status_json(const smart_hid_status_t *status, char **out_buf, size_t *out_len);

/**
 * 构造 Event JSON。
 */
int smart_hid_build_event_json(const smart_hid_event_t *event, char **out_buf, size_t *out_len);

/**
 * 渲染 topic（{device_id} 替换）。
 */
void smart_hid_build_topic(const char *fmt, const char *device_id, char *out, size_t out_size);

/* ============================================================
 * 字符串工具（用于枚举 ↔ 字符串）
 * ============================================================ */
const char *smart_hid_type_str(smart_hid_type_t t);
const char *smart_hid_action_str(smart_hid_action_t a);
const char *smart_hid_ack_status_str(smart_hid_ack_status_t s);

#ifdef __cplusplus
}
#endif
