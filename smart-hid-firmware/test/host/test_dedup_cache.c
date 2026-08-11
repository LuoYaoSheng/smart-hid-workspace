/**
 * test_dedup_cache.c — request_id 环形去重缓存的 host 单测。
 *
 * 覆盖语义：check_and_add 首次 miss+插入，二次 hit；clear 重置；环形覆盖（256+1 踢出最旧）。
 * 事实源：smart_hid_protocol.h SMART_HID_DEDUP_CACHE_SIZE=256。
 */
#include "dedup_cache.h"
#include "smart_hid_protocol.h"
#include "test_framework.h"
#include <stdio.h>

void test_dedup_cache_basic(void) {
    dedup_cache_init();
    CHECK(!dedup_cache_check_and_add(NULL), "NULL → false");

    CHECK(!dedup_cache_check_and_add("req-a"), "首次 req-a miss");
    CHECK(dedup_cache_check_and_add("req-a"), "二次 req-a hit");

    CHECK(!dedup_cache_check_and_add("req-b"), "首次 req-b miss");
    CHECK(dedup_cache_check_and_add("req-b"), "二次 req-b hit");

    /* req-a 仍应命中（未被 req-b 影响） */
    CHECK(dedup_cache_check_and_add("req-a"), "req-a 仍命中");
}

void test_dedup_cache_clear(void) {
    dedup_cache_init();
    dedup_cache_check_and_add("req-x");
    CHECK(dedup_cache_check_and_add("req-x"), "clear 前应命中");
    dedup_cache_clear();
    CHECK(!dedup_cache_check_and_add("req-x"), "clear 后应 miss");
}

void test_dedup_cache_ring_wraparound(void) {
    dedup_cache_init();
    char id[32];

    /* 填满 256 条（全 miss） */
    for (int i = 0; i < SMART_HID_DEDUP_CACHE_SIZE; i++) {
        snprintf(id, sizeof(id), "req-%03d", i);
        CHECK(!dedup_cache_check_and_add(id), "填入阶段应全 miss");
    }
    /* 填满后全部应命中 */
    for (int i = 0; i < SMART_HID_DEDUP_CACHE_SIZE; i++) {
        snprintf(id, sizeof(id), "req-%03d", i);
        CHECK(dedup_cache_check_and_add(id), "填满后全部应命中");
    }
    /* 插入第 257 个 → 覆盖 s_ids[0]（即最早的 req-000） */
    CHECK(!dedup_cache_check_and_add("req-256"), "第 257 条应 miss");
    /* req-000 应已被环形覆盖踢出（注：本次 check 会副作用地重新插入 req-000，
       但返回值反映"覆盖那一刻不在缓存"，仍验证了环形淘汰生效） */
    CHECK(!dedup_cache_check_and_add("req-000"), "req-000 应被环形淘汰");
    /* req-256 仍在缓存（刚插入） */
    CHECK(dedup_cache_check_and_add("req-256"), "req-256 应命中");
}
