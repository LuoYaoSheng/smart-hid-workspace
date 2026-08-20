/**
 * ble_proto.c — 分帧重组 + flat JSON 编解码（cJSON，与 smart_hid_protocol 同栈）
 */
#include "ble_proto.h"

#include <stdio.h>
#include <string.h>

#include "cJSON.h"

void ble_frame_reset(ble_frame_assembler_t *a) {
    if (a == NULL) return;
    a->len = 0;
    a->total = 0;
    a->next_seq = 0;
}

ble_proto_result_t ble_frame_feed(ble_frame_assembler_t *a,
                                  const uint8_t *data, uint16_t data_len) {
    if (a == NULL || data == NULL) return BLE_PROTO_ERR_FRAME;
    if (data_len < 3) return BLE_PROTO_ERR_FRAME;

    uint8_t seq = data[0];
    uint8_t total = data[1];
    uint8_t len = data[2];
    if (total == 0 || total > 64) return BLE_PROTO_ERR_FRAME;      /* ≤ 64 块 */
    if (len > BLE_PROTO_CHUNK_MAX || (uint16_t)3 + len != data_len) {
        return BLE_PROTO_ERR_FRAME;
    }

    if (seq == 0) {
        /* 新传输开始（也用于错误后的客户端重发） */
        ble_frame_reset(a);
        a->total = total;
    } else {
        if (a->total == 0 || seq != a->next_seq || total != a->total) {
            return BLE_PROTO_ERR_SEQ;
        }
    }

    if ((size_t)a->len + len > BLE_PROTO_ASSEMBLE_MAX) return BLE_PROTO_ERR_OVERFLOW;
    memcpy(a->buf + a->len, data + 3, len);
    a->len = (uint16_t)(a->len + len);
    a->next_seq = (uint8_t)(seq + 1);

    if (seq + 1 == a->total) return BLE_PROTO_COMPLETE;
    return BLE_PROTO_NEED_MORE;
}

int ble_proto_parse_candidate(const char *json, size_t json_len,
                              runtime_candidate_t *out) {
    if (json == NULL || out == NULL) return -1;
    memset(out, 0, sizeof(*out));

    cJSON *root = cJSON_ParseWithLength(json, json_len);
    if (root == NULL) return -1;

    int ok = 0;
    do {
        const cJSON *v = cJSON_GetObjectItem(root, "v");
        if (!cJSON_IsNumber(v) || v->valueint != 1) break; /* 只支持 v1 */

        const cJSON *ssid = cJSON_GetObjectItem(root, "wifi_ssid");
        const cJSON *pass = cJSON_GetObjectItem(root, "wifi_password");
        const cJSON *host = cJSON_GetObjectItem(root, "hub_host");
        const cJSON *port = cJSON_GetObjectItem(root, "hub_port");
        const cJSON *tok  = cJSON_GetObjectItem(root, "token");

        if (!cJSON_IsString(ssid) || ssid->valuestring == NULL || ssid->valuestring[0] == '\0') break;
        if (!cJSON_IsString(pass) || pass->valuestring == NULL) break;
        if (!cJSON_IsString(host) || host->valuestring == NULL || host->valuestring[0] == '\0') break;
        if (!cJSON_IsString(tok)  || tok->valuestring == NULL || tok->valuestring[0] == '\0') break;

        strlcpy(out->wifi_ssid, ssid->valuestring, sizeof(out->wifi_ssid));
        strlcpy(out->wifi_password, pass->valuestring, sizeof(out->wifi_password));
        strlcpy(out->hub_host, host->valuestring, sizeof(out->hub_host));
        out->hub_port = (port != NULL && cJSON_IsNumber(port) && port->valueint > 0)
                            ? (uint16_t)port->valueint : 17892;
        strlcpy(out->token, tok->valuestring, sizeof(out->token));
        ok = 1;
    } while (0);

    cJSON_Delete(root);
    return ok ? 0 : -1;
}

int ble_proto_build_info(char *buf, size_t buflen, const char *device_id,
                         const char *firmware, prov_state_t st, bool provisioned) {
    if (buf == NULL || buflen == 0) return -1;
    int n = snprintf(buf, buflen,
        "{\"product\":\"smart-hid\",\"protocol\":\"1.0\","
        "\"device_id\":\"%s\",\"firmware\":\"%s\","
        "\"state\":\"%s\",\"provisioned\":%s}",
        device_id ? device_id : "", firmware ? firmware : "",
        prov_state_str(st), provisioned ? "true" : "false");
    return (n > 0 && (size_t)n < buflen) ? 0 : -1;
}

int ble_proto_build_status(char *buf, size_t buflen, prov_state_t st,
                           const char *step, prov_error_t err) {
    if (buf == NULL || buflen == 0) return -1;
    const char *estr = prov_error_str(err);
    int n;
    if (estr != NULL) {
        n = snprintf(buf, buflen, "{\"state\":\"%s\",\"step\":\"%s\",\"error\":\"%s\"}",
                     prov_state_str(st), step ? step : prov_state_str(st), estr);
    } else {
        n = snprintf(buf, buflen, "{\"state\":\"%s\",\"step\":\"%s\",\"error\":null}",
                     prov_state_str(st), step ? step : prov_state_str(st));
    }
    return (n > 0 && (size_t)n < buflen) ? 0 : -1;
}
