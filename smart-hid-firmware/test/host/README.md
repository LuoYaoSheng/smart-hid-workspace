# 固件 host 单测

不依赖 ESP-IDF 工具链、不烧录，用本机 gcc 编译固件中的**纯逻辑组件**并在 host 上运行测试。

## 覆盖范围

| 组件 | 可 host 测？ | 说明 |
|------|------------|------|
| `hid_keymap.c` | ✅ 零依赖 | 键名 → HID Usage ID 映射（安全关键：错映射 = 按错键） |
| `dedup_cache.c` | ✅ 需 FreeRTOS stub | request_id 环形去重（可靠性关键：FIFO + 256 淘汰） |
| `smart_hid_protocol.c` | ⚠️ 依赖 cJSON | JSON 解析/构造，未来可扩展（需引入 cJSON 源） |
| `command_engine.c` | ❌ | 依赖 FreeRTOS queue + esp_timer |
| `hid_engine.c` | ❌ | 依赖 tinyusb（USB HAL） |
| `device_identity.c` | ❌ | 依赖 esp_mac/esp_timer |
| `wifi/mqtt/status_manager` | ❌ | 依赖 ESP-IDF 网络栈 |

## 运行

```bash
cd smart-hid-firmware/test/host
./run.sh
```

输出：每个 suite 的 `[RUN]` + 失败明细（如有），末尾汇总。退出码 0=全过。

## 机制

- `test_framework.h`：极简 `CHECK(cond, msg)` 宏，零依赖（不引入 unity/ESP-IDF）。一处失败 `return` 跳出当前 suite，后续 suite 继续。
- `freertos_stub/freertos/`：FreeRTOS 的最小 host stub（信号量 no-op）。`dedup_cache.c` 的 `#include "freertos/..."` 经 `-I freertos_stub` 解析到这里的假头。
- `run.sh`：gcc 编译「被测 .c + 测试 .c」，链接为单可执行文件直接跑。

## 扩展新测试

1. 确认目标组件是纯逻辑（不依赖 ESP-IDF HAL）或可 stub 其依赖
2. 新增 `test_<component>.c`，写 `void test_<component>_all(void)`
3. 在 `test_main.c` 的 suites 数组注册
4. 在 `run.sh` 的 `$SRC` 加被测源文件（若为新组件）
