/**
 * hub_pairing.c — esp_http_client 实现（仅目标机编译；host 测试经适配器注入）
 */
#include "hub_pairing.h"

#include <stdio.h>
#include <string.h>

#include "esp_http_client.h"
#include "esp_log.h"
#include "cJSON.h"

static const char *TAG = "hub_pairing";

#define PAIR_PATH "/api/v1/pairing/device"

/* 分块接收（pairing 响应 ~300B，2KB 缓冲足够） */
typedef struct {
    char buf[2048];
    int  len;
} pair_resp_t;

static esp_err_t on_http_event(esp_http_client_event_t *evt) {
    pair_resp_t *r = (pair_resp_t *)evt->user_data;
    if (evt->event_id == HTTP_EVENT_ON_DATA && r != NULL && r->len < (int)sizeof(r->buf) - 1) {
        int n = evt->data_len;
        if (r->len + n > (int)sizeof(r->buf) - 1) n = (int)sizeof(r->buf) - 1 - r->len;
        memcpy(r->buf + r->len, evt->data, n);
        r->len += n;
        r->buf[r->len] = '\0';
    }
    return ESP_OK;
}

int hub_pairing_perform(const char *host, uint16_t port, const char *token,
                        const char *device_id, const char *boot_id,
                        const char *firmware, const char *hardware,
                        hub_pairing_creds_t *creds_out) {
    if (host == NULL || token == NULL || device_id == NULL || creds_out == NULL) return -1;

    char url[160];
    snprintf(url, sizeof(url), "http://%s:%u%s", host, (unsigned)port, PAIR_PATH);

    char body[512];
    int blen = snprintf(body, sizeof(body),
        "{\"token\":\"%s\",\"device_id\":\"%s\",\"boot_id\":\"%s\","
        "\"firmware\":\"%s\",\"hardware\":\"%s\"}",
        token, device_id, boot_id ? boot_id : "",
        firmware ? firmware : "", hardware ? hardware : "");
    if (blen <= 0 || (size_t)blen >= sizeof(body)) return -1;

    pair_resp_t resp = {0};
    esp_http_client_config_t cfg = {
        .url = url,
        .method = HTTP_METHOD_POST,
        .timeout_ms = 10000,
        .event_handler = on_http_event,
        .user_data = &resp,
        /* LAN 内 http；TLS 属后续 Production Security（M2-G3） */
    };
    esp_http_client_handle_t client = esp_http_client_init(&cfg);
    if (client == NULL) return -1;
    esp_http_client_set_header(client, "Content-Type", "application/json");
    esp_http_client_set_post_field(client, body, blen);

    esp_err_t err = esp_http_client_perform(client);
    int status = (err == ESP_OK) ? esp_http_client_get_status_code(client) : 0;
    esp_http_client_cleanup(client);

    if (err != ESP_OK) {
        ESP_LOGW(TAG, "pairing request failed: %s url=%s", esp_err_to_name(err), url);
        return -1; /* controlhub_unreachable */
    }
    switch (status) {
        case 200: break;
        case 404: return -404;
        case 409: return -409;
        case 410: return -410;
        case 503: return -503;
        default:
            ESP_LOGW(TAG, "pairing unexpected status=%d", status);
            return -1;
    }

    /* 解析响应（cJSON；密码只进 creds_out，不打日志） */
    cJSON *root = cJSON_ParseWithLength(resp.buf, (size_t)resp.len);
    if (root == NULL) {
        ESP_LOGE(TAG, "pairing response not JSON (len=%d)", resp.len);
        return -1;
    }
    const cJSON *h = cJSON_GetObjectItem(root, "mqtt_host");
    const cJSON *p = cJSON_GetObjectItem(root, "mqtt_port");
    const cJSON *u = cJSON_GetObjectItem(root, "mqtt_username");
    const cJSON *c = cJSON_GetObjectItem(root, "mqtt_credential");
    int ok = cJSON_IsString(h) && h->valuestring && h->valuestring[0]
          && cJSON_IsNumber(p) && p->valueint > 0 && p->valueint <= 65535
          && cJSON_IsString(u) && u->valuestring && u->valuestring[0]
          && cJSON_IsString(c) && c->valuestring && c->valuestring[0];
    if (ok) {
        strlcpy(creds_out->mqtt_host, h->valuestring, sizeof(creds_out->mqtt_host));
        creds_out->mqtt_port = (uint16_t)p->valueint;
        strlcpy(creds_out->mqtt_username, u->valuestring, sizeof(creds_out->mqtt_username));
        strlcpy(creds_out->mqtt_password, c->valuestring, sizeof(creds_out->mqtt_password));
        ESP_LOGI(TAG, "pairing success: mqtt_host=%s port=%u user=%s",
                 creds_out->mqtt_host, (unsigned)creds_out->mqtt_port, creds_out->mqtt_username);
    }
    cJSON_Delete(root);
    return ok ? 0 : -1;
}
