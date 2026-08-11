/**
 * mqtt_manager.c — esp-mqtt 封装
 *
 * LWT：online=false retained status → ControlHub 启动时重建在线视图。
 * Fail-safe：MQTT 断开 → 触发 hid_engine_release_all()（通过 weak hook）。
 */
#include "mqtt_manager.h"
#include "device_identity.h"
#include "hid_engine.h"

#include <string.h>
#include "esp_log.h"
#include "mqtt_client.h"
#include "sdkconfig.h"

static const char *TAG = "mqtt_manager";

static esp_mqtt_client_handle_t s_client = NULL;
static volatile bool s_connected = false;

/* command topic 收到回调（command_engine 注入；app_main 装配） */
typedef void (*mqtt_cmd_handler_t)(const char *topic, const char *payload, int len);
/* 通过弱符号让 app_main 覆盖 */
__attribute__((weak)) void app_mqtt_on_command(const char *topic, const char *payload, int len) {
    (void)topic; (void)payload; (void)len;
}

/* ----------------------------------------------------------------
 * 构造 status JSON 字符串（用于 LWT 与 publish）
 * ---------------------------------------------------------------- */
static char *build_status_json(bool online) {
    smart_hid_status_t st = {0};
    strncpy(st.protocol,  SMART_HID_PROTOCOL_VERSION, sizeof(st.protocol) - 1);
    strncpy(st.device_id, device_identity_get_device_id(), sizeof(st.device_id) - 1);
    st.online        = online;
    strncpy(st.boot_id, device_identity_get_boot_id(), sizeof(st.boot_id) - 1);
    st.usb_hid_ready = hid_engine_is_ready();
    strncpy(st.firmware, device_identity_get_firmware(), sizeof(st.firmware) - 1);
    st.timestamp = (int64_t)time(NULL);
    char *buf = NULL; size_t len = 0;
    smart_hid_build_status_json(&st, &buf, &len);
    return buf;
}

static char *build_topic_str(const char *fmt) {
    const char *did = device_identity_get_device_id();
    size_t need = strlen(fmt) + strlen(did) + 1;
    char *t = malloc(need);
    if (t) smart_hid_build_topic(fmt, did, t, need);
    return t;
}

/* ----------------------------------------------------------------
 * 事件回调
 * ---------------------------------------------------------------- */
static void mqtt_event_handler(void *args, esp_event_base_t base,
                               int32_t id, void *event_data) {
    (void)args; (void)base;
    esp_mqtt_event_handle_t ev = event_data;
    switch ((esp_mqtt_event_id_t)id) {
        case MQTT_EVENT_CONNECTED: {
            s_connected = true;
            ESP_LOGI(TAG, "connected to broker");
            /* 订阅 command topic */
            char *ctopic = build_topic_str(SMART_HID_TOPIC_COMMAND_FMT);
            if (ctopic) {
                esp_mqtt_client_subscribe(s_client, ctopic, 1);
                ESP_LOGI(TAG, "subscribed: %s", ctopic);
                free(ctopic);
            }
            /* 上线 status */
            char *sjson = build_status_json(true);
            if (sjson) {
                char *stopic = build_topic_str(SMART_HID_TOPIC_STATUS_FMT);
                if (stopic) {
                    esp_mqtt_client_publish(s_client, stopic, sjson, 0, 1, 1); /* retain=1 */
                    free(stopic);
                }
                free(sjson);
            }
            break;
        }
        case MQTT_EVENT_DISCONNECTED: {
            s_connected = false;
            ESP_LOGW(TAG, "disconnected from broker → release_all");
            /* Fail-safe：MQTT 断开 → release_all */
            hid_engine_release_all();
            break;
        }
        case MQTT_EVENT_DATA: {
            /* command topic 收到 → 转发到 command_engine（弱符号 hook） */
            /* topic 与 payload 都可能不 NUL 终止，需要拷贝 */
            char topic[128];
            int tlen = ev->topic_len < (int)sizeof(topic) - 1 ? ev->topic_len : (int)sizeof(topic) - 1;
            memcpy(topic, ev->topic, tlen); topic[tlen] = '\0';
            char *payload = malloc(ev->data_len + 1);
            if (payload == NULL) break;
            memcpy(payload, ev->data, ev->data_len);
            payload[ev->data_len] = '\0';
            ESP_LOGI(TAG, "command RX: topic=%s len=%d", topic, ev->data_len);
            app_mqtt_on_command(topic, payload, ev->data_len);
            free(payload);
            break;
        }
        case MQTT_EVENT_ERROR:
            ESP_LOGE(TAG, "MQTT_EVENT_ERROR");
            break;
        default:
            break;
    }
}

/* ----------------------------------------------------------------
 * init
 * ---------------------------------------------------------------- */
int mqtt_manager_init(void) {
    /* LWT payload */
    char *lwt = build_status_json(false);
    char *lwt_topic = build_topic_str(SMART_HID_TOPIC_STATUS_FMT);

    esp_mqtt_client_config_t cfg = {
        .broker.address.hostname = CONFIG_SMART_HID_MQTT_BROKER_HOST,
        .broker.address.port     = CONFIG_SMART_HID_MQTT_BROKER_PORT,
        .broker.address.transport = MQTT_TRANSPORT_OVER_TCP,
        .credentials.username    = CONFIG_SMART_HID_MQTT_USERNAME,
        .credentials.authentication.password = CONFIG_SMART_HID_MQTT_PASSWORD,
        .credentials.client_id   = device_identity_get_device_id(),
        .network.reconnect_timeout_ms = 3000,
        .buffer.size             = 4096,
        .buffer.out_size         = 4096,
        .session.last_will       = {
            .topic    = lwt_topic ? lwt_topic : "",
            .msg      = lwt ? lwt : "",
            .msg_len  = lwt ? (int)strlen(lwt) : 0,
            .qos      = 1,
            .retain   = true,
        },
    };

    s_client = esp_mqtt_client_init(&cfg);
    if (lwt) free(lwt);
    if (lwt_topic) free(lwt_topic);
    if (s_client == NULL) {
        ESP_LOGE(TAG, "esp_mqtt_client_init failed");
        return -1;
    }
    esp_mqtt_client_register_event(s_client, ESP_EVENT_ANY_ID, mqtt_event_handler, NULL);
    esp_mqtt_client_start(s_client);
    return 0;
}

/* ----------------------------------------------------------------
 * publish 包装
 * ---------------------------------------------------------------- */
void mqtt_manager_publish_ack(const smart_hid_ack_t *ack) {
    if (!s_connected || s_client == NULL) return;
    char *json = NULL; size_t len = 0;
    if (smart_hid_build_ack_json(ack, &json, &len) != 0) return;
    char *topic = build_topic_str(SMART_HID_TOPIC_ACK_FMT);
    if (topic) {
        esp_mqtt_client_publish(s_client, topic, json, (int)len, 1, 0);  /* QoS1, retain=0 */
        free(topic);
    }
    free(json);
}

void mqtt_manager_publish_status(const smart_hid_status_t *status) {
    if (!s_connected || s_client == NULL) return;
    char *json = NULL; size_t len = 0;
    if (smart_hid_build_status_json(status, &json, &len) != 0) return;
    char *topic = build_topic_str(SMART_HID_TOPIC_STATUS_FMT);
    if (topic) {
        esp_mqtt_client_publish(s_client, topic, json, (int)len, 1, 1);  /* QoS1, retain=1 */
        free(topic);
    }
    free(json);
}

void mqtt_manager_publish_event(const smart_hid_event_t *event) {
    if (!s_connected || s_client == NULL) return;
    char *json = NULL; size_t len = 0;
    if (smart_hid_build_event_json(event, &json, &len) != 0) return;
    char *topic = build_topic_str(SMART_HID_TOPIC_EVENT_FMT);
    if (topic) {
        esp_mqtt_client_publish(s_client, topic, json, (int)len, 1, 0);  /* QoS1, retain=0 */
        free(topic);
    }
    free(json);
}

bool mqtt_manager_is_connected(void) { return s_connected; }
