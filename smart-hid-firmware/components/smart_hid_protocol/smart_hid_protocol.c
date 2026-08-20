/**
 * smart_hid_protocol.c — Smart HID 协议 JSON 序列化/反序列化
 *
 * 事实源：smart-ble/core/protocols/hid-command-schema.ts
 * 依据：docs/archive/04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md
 *
 * 字段顺序与 TS interface 对齐；新增字段时务必同步本文件 + TS。
 */
#include "smart_hid_protocol.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "cJSON.h"

/* ----------------------------------------------------------------
 * 枚举 ↔ 字符串
 * ---------------------------------------------------------------- */
const char *smart_hid_type_str(smart_hid_type_t t) {
    switch (t) {
        case SMART_HID_TYPE_KEYBOARD: return "keyboard";
        case SMART_HID_TYPE_MOUSE:    return "mouse";
        case SMART_HID_TYPE_SYSTEM:   return "system";
        default: return "unknown";
    }
}

const char *smart_hid_action_str(smart_hid_action_t a) {
    switch (a) {
        case SMART_HID_ACTION_TAP:        return "tap";
        case SMART_HID_ACTION_HOTKEY:     return "hotkey";
        case SMART_HID_ACTION_KEY_DOWN:   return "key_down";
        case SMART_HID_ACTION_KEY_UP:     return "key_up";
        case SMART_HID_ACTION_MOVE:       return "move";
        case SMART_HID_ACTION_CLICK:      return "click";
        case SMART_HID_ACTION_BUTTON_DOWN:return "button_down";
        case SMART_HID_ACTION_BUTTON_UP:  return "button_up";
        case SMART_HID_ACTION_WHEEL:      return "wheel";
        case SMART_HID_ACTION_RELEASE_ALL:return "release_all";
        default: return "unknown";
    }
}

const char *smart_hid_ack_status_str(smart_hid_ack_status_t s) {
    switch (s) {
        case SMART_HID_ACK_RECEIVED:  return "received";
        case SMART_HID_ACK_EXECUTING: return "executing";
        case SMART_HID_ACK_EXECUTED:  return "executed";
        case SMART_HID_ACK_REJECTED:  return "rejected";
        case SMART_HID_ACK_EXPIRED:   return "expired";
        case SMART_HID_ACK_DUPLICATE: return "duplicate";
        default: return "unknown";
    }
}

/* 字符串 → 枚举（解析用） */
static smart_hid_type_t parse_type(const char *s) {
    if (strcmp(s, "keyboard") == 0) return SMART_HID_TYPE_KEYBOARD;
    if (strcmp(s, "mouse")    == 0) return SMART_HID_TYPE_MOUSE;
    if (strcmp(s, "system")   == 0) return SMART_HID_TYPE_SYSTEM;
    return (smart_hid_type_t)-1;
}

static smart_hid_action_t parse_action(const char *s) {
    if (strcmp(s, "tap")         == 0) return SMART_HID_ACTION_TAP;
    if (strcmp(s, "hotkey")      == 0) return SMART_HID_ACTION_HOTKEY;
    if (strcmp(s, "key_down")    == 0) return SMART_HID_ACTION_KEY_DOWN;
    if (strcmp(s, "key_up")      == 0) return SMART_HID_ACTION_KEY_UP;
    if (strcmp(s, "move")        == 0) return SMART_HID_ACTION_MOVE;
    if (strcmp(s, "click")       == 0) return SMART_HID_ACTION_CLICK;
    if (strcmp(s, "button_down") == 0) return SMART_HID_ACTION_BUTTON_DOWN;
    if (strcmp(s, "button_up")   == 0) return SMART_HID_ACTION_BUTTON_UP;
    if (strcmp(s, "wheel")       == 0) return SMART_HID_ACTION_WHEEL;
    if (strcmp(s, "release_all") == 0) return SMART_HID_ACTION_RELEASE_ALL;
    return SMART_HID_ACTION_INVALID;
}

/* ----------------------------------------------------------------
 * 安全字符串拷贝
 * ---------------------------------------------------------------- */
static void copy_str(char *dst, size_t dst_size, const char *src) {
    if (dst_size == 0) return;
    if (src == NULL) { dst[0] = '\0'; return; }
    strncpy(dst, src, dst_size - 1);
    dst[dst_size - 1] = '\0';
}

/* ----------------------------------------------------------------
 * 解析 keyboard payload
 * ---------------------------------------------------------------- */
static int parse_keyboard_payload(const cJSON *payload, smart_hid_command_t *out) {
    /* key（单键） */
    const cJSON *key = cJSON_GetObjectItem(payload, "key");
    if (cJSON_IsString(key) && key->valuestring[0]) {
        copy_str(out->keyboard.key, sizeof(out->keyboard.key), key->valuestring);
    }
    /* keys（多键） */
    const cJSON *keys = cJSON_GetObjectItem(payload, "keys");
    if (cJSON_IsArray(keys)) {
        int n = cJSON_GetArraySize(keys);
        if (n > 8) n = 8;
        out->keyboard.keys_count = (uint8_t)n;
        for (int i = 0; i < n; i++) {
            const cJSON *k = cJSON_GetArrayItem(keys, i);
            if (cJSON_IsString(k)) {
                copy_str(out->keyboard.keys[i], sizeof(out->keyboard.keys[i]), k->valuestring);
            }
        }
    }
    /* hold_ms（默认 40） */
    const cJSON *hold = cJSON_GetObjectItem(payload, "hold_ms");
    out->keyboard.hold_ms = cJSON_IsNumber(hold) ? (uint32_t)hold->valuedouble : 40;
    /* lease_ms */
    const cJSON *lease = cJSON_GetObjectItem(payload, "lease_ms");
    out->keyboard.lease_ms = cJSON_IsNumber(lease) ? (uint32_t)lease->valuedouble : 0;
    return 0;
}

/* ----------------------------------------------------------------
 * 解析 mouse payload
 * ---------------------------------------------------------------- */
static int parse_mouse_payload(const cJSON *payload, smart_hid_command_t *out) {
    const cJSON *dx = cJSON_GetObjectItem(payload, "dx");
    out->mouse.dx = cJSON_IsNumber(dx) ? (int32_t)dx->valuedouble : 0;
    const cJSON *dy = cJSON_GetObjectItem(payload, "dy");
    out->mouse.dy = cJSON_IsNumber(dy) ? (int32_t)dy->valuedouble : 0;
    const cJSON *btn = cJSON_GetObjectItem(payload, "button");
    if (cJSON_IsString(btn)) copy_str(out->mouse.button, sizeof(out->mouse.button), btn->valuestring);
    const cJSON *cnt = cJSON_GetObjectItem(payload, "count");
    out->mouse.count = cJSON_IsNumber(cnt) ? (uint8_t)cnt->valuedouble : 1;
    const cJSON *dlt = cJSON_GetObjectItem(payload, "delta");
    out->mouse.delta = cJSON_IsNumber(dlt) ? (int32_t)dlt->valuedouble : 0;
    const cJSON *lease = cJSON_GetObjectItem(payload, "lease_ms");
    out->mouse.lease_ms = cJSON_IsNumber(lease) ? (uint32_t)lease->valuedouble : 0;
    return 0;
}

/* ----------------------------------------------------------------
 * 解析 Command
 * ---------------------------------------------------------------- */
int smart_hid_parse_command(const char *json, size_t json_len, smart_hid_command_t *out) {
    if (json == NULL || out == NULL) return -1;
    memset(out, 0, sizeof(*out));

    /* 解析时限制长度（payload 不超过 PAYLOAD_MAX_BYTES） */
    cJSON *root = cJSON_ParseWithLength(json, json_len);
    if (root == NULL) return SMART_HID_CODE_REJECTED_BAD_REQUEST;

    int rc = 0;
    do {
        const cJSON *p;
        p = cJSON_GetObjectItem(root, "protocol");
        if (!cJSON_IsString(p) || strcmp(p->valuestring, SMART_HID_PROTOCOL_VERSION) != 0) {
            rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break;
        }
        copy_str(out->protocol, sizeof(out->protocol), p->valuestring);

        p = cJSON_GetObjectItem(root, "request_id");
        if (!cJSON_IsString(p) || p->valuestring[0] == '\0' ||
            strlen(p->valuestring) > SMART_HID_REQUEST_ID_MAX_LEN) {
            rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break;
        }
        copy_str(out->request_id, sizeof(out->request_id), p->valuestring);

        p = cJSON_GetObjectItem(root, "device_id");
        if (!cJSON_IsString(p)) { rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break; }
        copy_str(out->device_id, sizeof(out->device_id), p->valuestring);

        p = cJSON_GetObjectItem(root, "target_boot_id");
        if (!cJSON_IsString(p)) { rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break; }
        copy_str(out->target_boot_id, sizeof(out->target_boot_id), p->valuestring);

        p = cJSON_GetObjectItem(root, "type");
        if (!cJSON_IsString(p)) { rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break; }
        smart_hid_type_t t = parse_type(p->valuestring);
        if ((int)t < 0) { rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break; }
        out->type = t;

        p = cJSON_GetObjectItem(root, "action");
        if (!cJSON_IsString(p)) { rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break; }
        smart_hid_action_t a = parse_action(p->valuestring);
        if (a == SMART_HID_ACTION_INVALID) { rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break; }
        out->action = a;

        p = cJSON_GetObjectItem(root, "ttl_ms");
        if (!cJSON_IsNumber(p) ||
            p->valuedouble < SMART_HID_TTL_MS_MIN ||
            p->valuedouble > SMART_HID_TTL_MS_MAX) {
            rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break;
        }
        out->ttl_ms = (uint32_t)p->valuedouble;

        p = cJSON_GetObjectItem(root, "payload");
        if (!cJSON_IsObject(p)) { rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break; }

        /* payload 原文（重新序列化为紧凑 JSON 存档，限制长度） */
        char *pjs = cJSON_PrintUnformatted(p);
        if (pjs != NULL) {
            size_t plen = strlen(pjs);
            if (plen > SMART_HID_PAYLOAD_MAX_BYTES) {
                free(pjs);
                rc = SMART_HID_CODE_REJECTED_PAYLOAD_TOO_BIG; break;
            }
            copy_str(out->payload_json, sizeof(out->payload_json), pjs);
            free(pjs);
        }

        /* 按 type 分发解析子字段 */
        if (out->type == SMART_HID_TYPE_KEYBOARD) {
            if (parse_keyboard_payload(p, out) != 0) {
                rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break;
            }
        } else if (out->type == SMART_HID_TYPE_MOUSE) {
            if (parse_mouse_payload(p, out) != 0) {
                rc = SMART_HID_CODE_REJECTED_BAD_REQUEST; break;
            }
        }
        /* system / release_all 无 payload 子字段 */
    } while (0);

    cJSON_Delete(root);
    return rc;
}

/* ----------------------------------------------------------------
 * 构造 ACK JSON
 * ---------------------------------------------------------------- */
int smart_hid_build_ack_json(const smart_hid_ack_t *ack, char **out_buf, size_t *out_len) {
    if (ack == NULL || out_buf == NULL) return -1;
    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "protocol",   ack->protocol[0] ? ack->protocol : SMART_HID_PROTOCOL_VERSION);
    cJSON_AddStringToObject(root, "request_id", ack->request_id);
    cJSON_AddStringToObject(root, "device_id",  ack->device_id);
    cJSON_AddStringToObject(root, "boot_id",    ack->boot_id);
    cJSON_AddStringToObject(root, "status",     smart_hid_ack_status_str(ack->status));
    cJSON_AddNumberToObject(root, "code",       (double)ack->code);
    if (ack->status == SMART_HID_ACK_EXECUTED && ack->execution_ms > 0) {
        cJSON_AddNumberToObject(root, "execution_ms", (double)ack->execution_ms);
    }
    char *s = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);
    if (s == NULL) return -1;
    if (out_len) *out_len = strlen(s);
    *out_buf = s;
    return 0;
}

/* ----------------------------------------------------------------
 * 构造 Status JSON
 * ---------------------------------------------------------------- */
int smart_hid_build_status_json(const smart_hid_status_t *st, char **out_buf, size_t *out_len) {
    if (st == NULL || out_buf == NULL) return -1;
    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "protocol",      st->protocol[0] ? st->protocol : SMART_HID_PROTOCOL_VERSION);
    cJSON_AddStringToObject(root, "device_id",     st->device_id);
    cJSON_AddBoolToObject  (root, "online",        st->online);
    cJSON_AddStringToObject(root, "boot_id",       st->boot_id);
    cJSON_AddBoolToObject  (root, "usb_hid_ready", st->usb_hid_ready);
    if (st->firmware[0]) {
        cJSON_AddStringToObject(root, "firmware", st->firmware);
    }
    cJSON_AddNumberToObject(root, "timestamp", (double)st->timestamp);
    char *s = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);
    if (s == NULL) return -1;
    if (out_len) *out_len = strlen(s);
    *out_buf = s;
    return 0;
}

/* ----------------------------------------------------------------
 * 构造 Event JSON
 * ---------------------------------------------------------------- */
int smart_hid_build_event_json(const smart_hid_event_t *ev, char **out_buf, size_t *out_len) {
    if (ev == NULL || out_buf == NULL) return -1;
    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "protocol",  ev->protocol[0] ? ev->protocol : SMART_HID_PROTOCOL_VERSION);
    cJSON_AddStringToObject(root, "device_id", ev->device_id);
    cJSON_AddStringToObject(root, "event",     ev->event);
    /* detail 必须是合法 JSON 对象字符串；若为空用 "{}" */
    if (ev->detail[0]) {
        cJSON *detail = cJSON_Parse(ev->detail);
        if (detail != NULL) {
            cJSON_AddItemToObject(root, "detail", detail);
        } else {
            cJSON_AddObjectToObject(root, "detail");
        }
    } else {
        cJSON_AddObjectToObject(root, "detail");
    }
    cJSON_AddNumberToObject(root, "timestamp", (double)ev->timestamp);
    char *s = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);
    if (s == NULL) return -1;
    if (out_len) *out_len = strlen(s);
    *out_buf = s;
    return 0;
}

/* ----------------------------------------------------------------
 * 渲染 topic
 * ---------------------------------------------------------------- */
void smart_hid_build_topic(const char *fmt, const char *device_id, char *out, size_t out_size) {
    snprintf(out, out_size, fmt, device_id);
}
