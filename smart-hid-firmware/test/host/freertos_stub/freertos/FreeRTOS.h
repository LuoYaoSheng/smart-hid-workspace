/**
 * freertos/FreeRTOS.h — Host 测试用 FreeRTOS 最小 stub。
 *
 * 仅供 dedup_cache 等纯逻辑组件在 host 上编译。真实设备使用 ESP-IDF 自带 FreeRTOS。
 * 这里把互斥锁 stub 成 no-op（host 单测单线程，无需真实同步）。
 */
#pragma once
#include <stdint.h>

typedef void *SemaphoreHandle_t;
typedef int BaseType_t;

#define pdTRUE 1
#define pdFALSE 0
#define portMAX_DELAY 0xFFFFFFFFU
