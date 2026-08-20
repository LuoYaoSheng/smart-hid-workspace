/**
 * esp_log.h — host 测试 stub（打印到 stdout；真实行为见 ESP-IDF）
 */
#pragma once
#include <stdio.h>

#define ESP_LOGI(tag, fmt, ...) printf("[I][%s] " fmt "\n", tag, ##__VA_ARGS__)
#define ESP_LOGW(tag, fmt, ...) printf("[W][%s] " fmt "\n", tag, ##__VA_ARGS__)
#define ESP_LOGE(tag, fmt, ...) printf("[E][%s] " fmt "\n", tag, ##__VA_ARGS__)
#define ESP_LOGD(tag, fmt, ...) ((void)0)
