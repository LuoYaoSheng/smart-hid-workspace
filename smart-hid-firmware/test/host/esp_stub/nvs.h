/**
 * nvs.h — host 测试 stub：进程内 NVS（namespace → key → value）
 *
 * 提供被测组件用到的最小 API + 测试控制接口（reset / fail_next_commit /
 * 直接写原始值模拟损坏或未来版本）。
 */
#pragma once
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"

typedef uint32_t nvs_handle_t;

#define NVS_READWRITE 1
#define NVS_READONLY  0

esp_err_t nvs_open(const char *ns, int mode, nvs_handle_t *out);
esp_err_t nvs_get_str(nvs_handle_t h, const char *key, char *out, size_t *len);
esp_err_t nvs_get_u8(nvs_handle_t h, const char *key, uint8_t *out);
esp_err_t nvs_get_u16(nvs_handle_t h, const char *key, uint16_t *out);
esp_err_t nvs_get_u32(nvs_handle_t h, const char *key, uint32_t *out);
esp_err_t nvs_set_str(nvs_handle_t h, const char *key, const char *val);
esp_err_t nvs_set_u8(nvs_handle_t h, const char *key, uint8_t v);
esp_err_t nvs_set_u16(nvs_handle_t h, const char *key, uint16_t v);
esp_err_t nvs_set_u32(nvs_handle_t h, const char *key, uint32_t v);
esp_err_t nvs_erase_all(nvs_handle_t h);
esp_err_t nvs_commit(nvs_handle_t h);
esp_err_t nvs_close(nvs_handle_t h);

/* ---- 测试控制 ---- */
void nvs_stub_reset(void);
void nvs_stub_fail_commit(bool fail);          /* 下一次 commit 返回 ESP_FAIL */
void nvs_stub_fail_ns(const char *ns);         /* 该 namespace 的 commit 全部失败直至 clear */
void nvs_stub_reset_fail_ns(void);
int  nvs_stub_raw_set_u8(const char *ns, const char *key, uint8_t v); /* 绕过句柄直写 */
