/**
 * dedup_cache.c — request_id 环形缓存
 */
#include "dedup_cache.h"
#include "smart_hid_protocol.h"
#include <string.h>
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"

#define CACHE_SIZE  SMART_HID_DEDUP_CACHE_SIZE

static char s_ids[CACHE_SIZE][SMART_HID_REQUEST_ID_MAX_LEN + 1];
static int  s_head = 0;   /* 下一个写入位置 */
static int  s_count = 0;  /* 已存条数（用于避免首圈把空槽判为命中） */
static SemaphoreHandle_t s_lock = NULL;

void dedup_cache_init(void) {
    if (s_lock == NULL) s_lock = xSemaphoreCreateMutex();
    s_head = 0;
    s_count = 0;
    memset(s_ids, 0, sizeof(s_ids));
}

bool dedup_cache_check_and_add(const char *request_id) {
    if (request_id == NULL) return false;
    if (s_lock == NULL) dedup_cache_init();
    xSemaphoreTake(s_lock, portMAX_DELAY);
    bool hit = false;
    int n = (s_count < CACHE_SIZE) ? s_count : CACHE_SIZE;
    for (int i = 0; i < n; i++) {
        if (strcmp(s_ids[i], request_id) == 0) { hit = true; break; }
    }
    if (!hit) {
        strncpy(s_ids[s_head], request_id, SMART_HID_REQUEST_ID_MAX_LEN);
        s_ids[s_head][SMART_HID_REQUEST_ID_MAX_LEN] = '\0';
        s_head = (s_head + 1) % CACHE_SIZE;
        if (s_count < CACHE_SIZE) s_count++;
    }
    xSemaphoreGive(s_lock);
    return hit;
}

void dedup_cache_clear(void) {
    if (s_lock == NULL) return;
    xSemaphoreTake(s_lock, portMAX_DELAY);
    memset(s_ids, 0, sizeof(s_ids));
    s_head = 0;
    s_count = 0;
    xSemaphoreGive(s_lock);
}
