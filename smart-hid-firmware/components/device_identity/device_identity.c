/**
 * device_identity.c — Device ID + Boot ID 实现
 */
#include "device_identity.h"

#include <string.h>
#include "esp_log.h"
#include "esp_random.h"
#include "esp_timer.h"
#include "nvs_flash.h"
#include "nvs.h"

static const char *TAG = "device_identity";

#define NVS_NAMESPACE  "identity"
#define NVS_KEY_DEVICE "device_id"

/* 固件版本（与 sdkconfig / build tag 一致；F4 OTA 后由 OTA metadata 覆盖） */
#define FIRMWARE_VERSION "1.0.0-f1f2"

static char s_device_id[SMART_HID_DEVICE_ID_MAX_LEN] = {0};
static char s_boot_id[SMART_HID_BOOT_ID_MAX_LEN]     = {0};

/* ----------------------------------------------------------------
 * 生成 boot_id：B-XXXXXX（6 hex，来自硬件 RNG）
 * ---------------------------------------------------------------- */
static void generate_boot_id(char *out, size_t out_size) {
    uint32_t r = esp_random();
    /* 加上启动计数式的额外熵（esp_timer）避免极短时间重启重复 */
    int64_t t = esp_timer_get_time();
    r ^= (uint32_t)(t >> 4);
    r ^= (uint32_t)(t & 0xFFFFFFFFu);
    snprintf(out, out_size, "B-%06X", (unsigned int)(r & 0xFFFFFFu));
}

/* ----------------------------------------------------------------
 * 简单 device_id 格式校验：^HID-[A-Z0-9]{8}$
 * ---------------------------------------------------------------- */
static bool is_valid_device_id(const char *s) {
    if (s == NULL) return false;
    if (strncmp(s, "HID-", 4) != 0) return false;
    size_t n = strlen(s);
    if (n != 12) return false;  /* "HID-" + 8 chars */
    for (size_t i = 4; i < n; i++) {
        char c = s[i];
        if (!((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'))) return false;
    }
    return true;
}

int device_identity_init(const char *fallback_default) {
    nvs_handle_t h;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READWRITE, &h);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "nvs_open(%s) failed: %s", NVS_NAMESPACE, esp_err_to_name(err));
        return (int)err;
    }

    /* 读 device_id */
    char buf[SMART_HID_DEVICE_ID_MAX_LEN] = {0};
    size_t required = sizeof(buf);
    err = nvs_get_str(h, NVS_KEY_DEVICE, buf, &required);

    if (err == ESP_OK && is_valid_device_id(buf)) {
        /* 命中持久化值 */
        memcpy(s_device_id, buf, sizeof(s_device_id));
        s_device_id[sizeof(s_device_id) - 1] = '\0';
        ESP_LOGI(TAG, "device_id loaded from NVS: %s", s_device_id);
    } else {
        /* 未命中或非法 → 用 fallback 写入 */
        const char *def = (fallback_default != NULL && fallback_default[0]) ? fallback_default : "HID-00000001";
        if (!is_valid_device_id(def)) {
            ESP_LOGW(TAG, "fallback device_id invalid '%s', using HID-00000001", def);
            def = "HID-00000001";
        }
        strncpy(s_device_id, def, sizeof(s_device_id) - 1);
        s_device_id[sizeof(s_device_id) - 1] = '\0';

        err = nvs_set_str(h, NVS_KEY_DEVICE, s_device_id);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "nvs_set_str(device_id) failed: %s", esp_err_to_name(err));
            nvs_close(h);
            return (int)err;
        }
        err = nvs_commit(h);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "nvs_commit failed: %s", esp_err_to_name(err));
            nvs_close(h);
            return (int)err;
        }
        ESP_LOGI(TAG, "device_id written to NVS: %s (first boot)", s_device_id);
    }
    nvs_close(h);

    /* 生成本次 boot_id */
    generate_boot_id(s_boot_id, sizeof(s_boot_id));
    ESP_LOGI(TAG, "boot_id generated: %s", s_boot_id);

    return 0;
}

const char *device_identity_get_device_id(void) {
    return s_device_id;
}

const char *device_identity_get_boot_id(void) {
    return s_boot_id;
}

const char *device_identity_get_firmware(void) {
    return FIRMWARE_VERSION;
}
