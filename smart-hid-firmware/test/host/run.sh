#!/bin/sh
# Smart HID Firmware host 单测运行器
#
# 用 host gcc 编译纯逻辑组件（hid_keymap + dedup_cache + runtime_config +
# provisioning + ble_proto）+ 测试，无需 ESP-IDF 工具链/硬件。
# 依赖 stub：freertos_stub/（信号量 no-op）、esp_stub/（esp_log/nvs 内存实现）。
# cJSON 依赖 ESP-IDF 安装（ble_proto / provisioning 状态 JSON 与目标机同源）：
#   优先 $IDF_PATH，回退 ~/esp/esp-idf。
#
# 用法：./run.sh
# 退出码：0 全过；1 有失败。
set -e

cd "$(dirname "$0")"
CC=${CC:-cc}
ROOT="../../"

IDF="${IDF_PATH:-$HOME/esp/esp-idf}"
CJSON_DIR="$IDF/components/json/cJSON"
if [ ! -f "$CJSON_DIR/cJSON.c" ]; then
    echo "ERROR: ESP-IDF cJSON not found at $CJSON_DIR (set IDF_PATH)" >&2
    exit 1
fi

INCLUDES="-I esp_stub \
  -I freertos_stub \
  -I ${ROOT}components/hid_engine/include \
  -I ${ROOT}components/command_engine/include \
  -I ${ROOT}components/smart_hid_protocol/include \
  -I ${ROOT}components/runtime_config/include \
  -I ${ROOT}components/provisioning/include \
  -I ${ROOT}components/ble_provision/include \
  -I $CJSON_DIR"

SRC="${ROOT}components/hid_engine/hid_keymap.c \
     ${ROOT}components/command_engine/dedup_cache.c \
     ${ROOT}components/runtime_config/runtime_config.c \
     ${ROOT}components/provisioning/provisioning.c \
     ${ROOT}components/ble_provision/ble_proto.c \
     esp_stub/nvs_stub.c \
     $CJSON_DIR/cJSON.c"

TESTS="test_hid_keymap.c test_dedup_cache.c \
       test_runtime_config.c test_provisioning.c test_ble_proto.c test_main.c"

OUT="${TMPDIR:-/tmp}/smarthid_host_test"

$CC -std=c11 -Wall -Wextra -O0 -g -o "$OUT" $SRC $TESTS $INCLUDES
"$OUT"
