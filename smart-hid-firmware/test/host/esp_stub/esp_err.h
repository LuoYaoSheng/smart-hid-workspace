/**
 * esp_err.h — host 测试 stub（最小 API 面）
 */
#pragma once
#include <stddef.h>

typedef int esp_err_t;

#define ESP_OK 0
#define ESP_FAIL -1
#define ESP_ERR_NVS_NOT_FOUND 0x1102
#define ESP_ERR_NVS_NO_FREE_PAGES 0x1101

static inline const char *esp_err_to_name(esp_err_t e) {
    switch (e) {
        case ESP_OK: return "ESP_OK";
        case ESP_ERR_NVS_NOT_FOUND: return "ESP_ERR_NVS_NOT_FOUND";
        default: return "ESP_ERR_UNKNOWN";
    }
}
