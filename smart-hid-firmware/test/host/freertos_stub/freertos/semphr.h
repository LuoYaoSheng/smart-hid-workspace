/**
 * freertos/semphr.h — Host 测试用信号量 stub（no-op）。
 */
#pragma once
#include "FreeRTOS.h"

static inline SemaphoreHandle_t xSemaphoreCreateMutex(void) {
    return (SemaphoreHandle_t)1; /* 非 NULL，表示"已创建" */
}
static inline BaseType_t xSemaphoreTake(SemaphoreHandle_t h, uint32_t timeout) {
    (void)h; (void)timeout;
    return pdTRUE;
}
static inline BaseType_t xSemaphoreGive(SemaphoreHandle_t h) {
    (void)h;
    return pdTRUE;
}
