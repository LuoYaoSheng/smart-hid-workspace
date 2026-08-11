/**
 * dedup_cache.h — request_id 环形去重缓存（FIFO，256 条）
 *
 * 依据 docs/06 §9 / hid-command-schema.ts COMMAND_CONSTANTS.DEDUP_CACHE_SIZE
 *
 * 命中策略：look-up 返回 true 时表示是重复 request_id（应返回 duplicate）。
 * 同 request_id 第二次收到 → 命中；第一次 → miss + 自动插入。
 */
#pragma once

#include <stdbool.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

void dedup_cache_init(void);

/**
 * 查 request_id；首次见则插入，返回 false。
 * 已在缓存中 → 返回 true（重复）。
 */
bool dedup_cache_check_and_add(const char *request_id);

/** 清空（boot_id 变化后调用） */
void dedup_cache_clear(void);

#ifdef __cplusplus
}
#endif
