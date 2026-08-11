#!/bin/sh
# Smart HID Firmware host 单测运行器
#
# 用 host gcc 编译纯逻辑组件（hid_keymap + dedup_cache）+ 测试，无需 ESP-IDF/硬件。
# dedup_cache 的 FreeRTOS 依赖由 freertos_stub/ 提供（no-op 信号量）。
#
# 用法：./run.sh
# 退出码：0 全过；1 有失败。
set -e

cd "$(dirname "$0")"
CC=${CC:-cc}
ROOT="../../"

INCLUDES="-I freertos_stub \
  -I ${ROOT}components/hid_engine/include \
  -I ${ROOT}components/command_engine/include \
  -I ${ROOT}components/smart_hid_protocol/include"

SRC="${ROOT}components/hid_engine/hid_keymap.c \
     ${ROOT}components/command_engine/dedup_cache.c"

TESTS="test_hid_keymap.c test_dedup_cache.c test_main.c"

OUT="${TMPDIR:-/tmp}/smarthid_host_test"

$CC -std=c11 -Wall -Wextra -O0 -g -o "$OUT" $SRC $TESTS $INCLUDES
"$OUT"
