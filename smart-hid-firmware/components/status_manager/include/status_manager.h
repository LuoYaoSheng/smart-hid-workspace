/**
 * status_manager.h — 周期心跳 status + 即时上下线广播
 */
#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

int status_manager_init(void);

/** 立即广播一次 status */
void status_manager_publish_now(bool online);

#ifdef __cplusplus
}
#endif
