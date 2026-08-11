/**
 * test_framework.h — 固件 host 单测的极简测试宏（零依赖，不引入 unity/ESP-IDF）。
 *
 * 设计：每个测试函数返回 void；CHECK 失败时打印、累加失败计数、return 跳出当前函数。
 * 后续 suite 仍会继续执行（一处失败不阻塞其它用例）。
 */
#pragma once
#include <stdio.h>

extern int g_suites_run, g_suites_fail;

#define CHECK(cond, msg)                                                       \
    do {                                                                       \
        if (!(cond)) {                                                         \
            printf("    FAIL: %s\n", msg);                                     \
            g_suites_fail++;                                                   \
            return;                                                            \
        }                                                                      \
    } while (0)
