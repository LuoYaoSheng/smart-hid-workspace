/**
 * test_main.c — 固件 host 单测 runner。
 *
 * 编译：见 run.sh（host gcc + freertos stub，不依赖 ESP-IDF/硬件）。
 * 运行：./smarthid_host_test，退出码 0=全过，1=有失败。
 */
#include "test_framework.h"

void test_hid_keymap_all(void);
void test_dedup_cache_basic(void);
void test_dedup_cache_clear(void);
void test_dedup_cache_ring_wraparound(void);

int g_suites_run = 0, g_suites_fail = 0;

int main(void) {
    struct {
        const char *name;
        void (*fn)(void);
    } suites[] = {
        { "hid_keymap: 全键码覆盖 + 别名 + 大小写 + 无效输入", test_hid_keymap_all },
        { "dedup_cache: 基本 miss/hit", test_dedup_cache_basic },
        { "dedup_cache: clear 重置", test_dedup_cache_clear },
        { "dedup_cache: 环形覆盖（256+1 淘汰最旧）", test_dedup_cache_ring_wraparound },
    };
    int n = (int)(sizeof(suites) / sizeof(suites[0]));

    printf("=== Smart HID Firmware host 单测 ===\n");
    for (int i = 0; i < n; i++) {
        g_suites_run++;
        printf("[RUN] %s\n", suites[i].name);
        suites[i].fn();
    }

    printf("\n=== %d suite, %d passed, %d failed ===\n",
           n, n - g_suites_fail, g_suites_fail);
    return g_suites_fail ? 1 : 0;
}
