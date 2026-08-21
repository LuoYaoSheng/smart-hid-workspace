# smart-hid-firmware

ESP32-S3 Smart HID 固件。USB Composite HID（Keyboard + Mouse）+ Wi-Fi + MQTT + BLE Provisioning。

```text
ControlHub ──MQTT──▶ ESP32-S3 ──USB HID──▶ 目标电脑
BLE Toolkit+ ──BLE──▶ ESP32-S3 （配网 / 诊断）
```

订阅 MQTT Command → 解析 → dedup → boot_id 校验 → 入队 → worker 串行执行 → 发 ACK。
MQTT callback 不直接发 HID Report。

## 当前状态

✅ **F1+F2+F3 C 源码完成 + ESP-IDF v5.4.4 双配置编译通过**：
- USB Composite HID（Keyboard + Mouse）：esp_tinyusb managed component + `tinyusb_driver_install` + 官方 `TUD_HID_REPORT_DESC_*` 模板 + 2 个 HID interface
- 协议层 JSON 解析/序列化（镜像 smart-ble TS 事实源）
- command_engine：queue(32) + dedup(256) + boot_id 校验 + TTL + worker task + lease tick
- mqtt_manager：连接 / 订阅 / LWT / publish ack+status+event + 运行时 `configure()`（凭据来自配网，不再编译期写死）
- wifi_manager（`init` / `connect_sta` 拆分，配网流程可先起 BLE 再连 Wi-Fi）/ status_manager / device_identity（NVS + `esp_app_desc` 版本注入）
- F3 配网栈：`runtime_config`（NVS rt_active/rt_pending 双配置 + schema_version 守卫）+ `provisioning` 状态机（纯逻辑层，可宿主单测）+ `hub_pairing`（HTTP :17892 配对）+ `ble_provision`（NimBLE GATT 服务 + 分帧协议）

✅ **`idf.py build` 通过**（默认配置 + `sdkconfig.dev.defaults` DEV 配置）：
产出 `smart-hid-firmware.bin`（1,074,768 字节，factory 分区 1536K，分区表 3×1536K + NVS 0x8000）+ bootloader.bin + partition-table.bin + ota_data_initial.bin。
- 工具链：ESP-IDF v5.4.4 / xtensa-esp-elf-gcc 14.2 / Python 3.12 / macOS arm64
- esp_tinyusb 2.x + NimBLE（managed components，`main/idf_component.yml` 声明）
- 版本自根 `VERSION` 文件 → CMake `PROJECT_VER` → `esp_app_desc` 注入（`device_identity_get_firmware()` 运行时可读）

✅ **F2 可靠性语义已通过 Go 参考实现端到端验证**（`../smart-hid-controlhub/cmd/mock-device` + `scripts/test-loop-f2.sh`，28/28 全过）：
- request_id 去重（duplicate）
- target_boot_id 防旧命令（rejected STALE_DEVICE_SESSION）
- TTL 过期 / 范围校验
- queue full 明确返回
- lease 超时自动释放
- system/release_all 清空所有 pressed keys/buttons
- MQTT 断开 → release_all

✅ **宿主单测 36/36 通过**（`test/host/`，不依赖 ESP-IDF 工具链与硬件）：
- F2 层：`hid_keymap`（键名→Usage 映射，安全关键）、`dedup_cache`（环形去重）
- F3 层：`runtime_config`（active/pending 原子切换、schema_version 守卫、commit 前失败不污染 active）、`provisioning`（无配置进配网、token 消费时序、MQTT 失败入 RECOVERY 不重做 Wi-Fi、密钥不出现在日志）、`ble_proto`（分帧重组 / 乱序拒绝 / 超长拒绝 / candidate 解析）

⚠️ **尚未在真实 ESP32-S3 硬件上烧录验证**（USB 枚举 / BLE 实测 / Wi-Fi 实连需硬件）。
   语义对齐由 Go 参考实现保证；配网逻辑由宿主单测保证；编译正确性由 `idf.py build` 通过保证。

## 配网模型（M1-G3 起）

- **NVS 运行时配置是正式事实源**：BLE Provisioning 写入 `rt_pending`，全链路验证通过后原子提升为 `rt_active`；失败永不触碰旧配置（不会变砖）
- Kconfig 仅为显式 DEV 兜底（`CONFIG_SMART_HID_DEV_STATIC_CONFIG`，默认 n，仅驻内存绝不写 NVS）
- 新设备无配置 → 自动进入 BLE Provision Mode（广播配网服务，等待写入候选配置）
- 崩溃边界规则：已 complete 的 pending 上电自动提升（token 已在服务端消费 + 凭据已持久化，是唯一无死态路径）；未 complete 的 pending 直接丢弃
- 协议正典：`../protocols/ble/PROVISIONING_V1.md`（服务/特征 UUID、分帧格式、候选 JSON、错误码、NimBLE 安全模型如实声明）

## 目录结构

```text
smart-hid-firmware/
├── CMakeLists.txt              # 顶层 ESP-IDF 工程（读根 VERSION 注入 PROJECT_VER）
├── partitions.csv              # 3×1536K app 分区 + NVS 0x8000（预留双 OTA）
├── sdkconfig.defaults          # 默认配置（target=esp32s3, TINYUSB_HID_COUNT=2, NimBLE）
├── sdkconfig.dev.defaults      # DEV 静态配置（CI 双配置编译验证用）
├── main/
│   ├── CMakeLists.txt
│   ├── Kconfig.projbuild       # DEV_STATIC 开关 + DEV 静态字段（默认全关）
│   ├── idf_component.yml       # esp_tinyusb / NimBLE managed components
│   └── main.c                  # app_main：装配 + prov_task + 断线 5 分钟 RECOVERY
├── components/
│   ├── smart_hid_protocol/     # 协议契约（镜像 TS 事实源）
│   ├── device_identity/        # device_id(NVS) + boot_id + 固件版本(esp_app_desc)
│   ├── command_engine/         # queue + dedup + worker + lease tick
│   │   └── dedup_cache.c
│   ├── hid_engine/             # USB HID 报告 + lease + release_all
│   │   └── hid_keymap.c
│   ├── mqtt_manager/           # esp-mqtt 封装 + LWT + 运行时 configure
│   ├── wifi_manager/           # STA + 断开 release_all（init/connect 拆分）
│   ├── status_manager/         # 心跳 status
│   ├── runtime_config/         # NVS rt_active/rt_pending 双配置 + schema_version 守卫
│   ├── provisioning/           # 配网状态机（纯逻辑，adapter 注入，可宿主单测）
│   ├── hub_pairing/            # ControlHub :17892 配对（esp_http_client）
│   └── ble_provision/          # NimBLE GATT 服务 + BLE 分帧协议
└── test/host/                  # 宿主单测（gcc + stub，36 suite）
    ├── esp_stub/  freertos_stub/
    ├── test_hid_keymap.c  test_dedup_cache.c
    ├── test_runtime_config.c  test_provisioning.c  test_ble_proto.c
    └── run.sh
```

## 组件 ↔ Go 参考 / 宿主单测 对照

| 验收项 (§B) | C 组件 | Go 参考 (mock-device) |
|------------|--------|----------------------|
| request_id 去重 | `command_engine/dedup_cache.c` | `DedupCache.CheckAndAdd` |
| target_boot_id | `command_engine.c` + `device_identity.c` | `Device.HandleCommand` boot_id 分支 |
| lease 超时释放 | `hid_engine.c tick_leases` | `LeaseManager.startTicker` |
| MQTT 断开 release_all | `mqtt_manager.c` + `wifi_manager.c` | `OnConnectionLost` |
| queue full | `command_engine.c` queue_full 分支 | `Device.HandleCommand` queue 满 |
| system/release_all | `hid_engine_release_all` | `LeaseManager.ReleaseAll` |

| 配网验收项 | C 组件 | 宿主单测 |
|-----------|--------|---------|
| active/pending 原子切换 + 崩溃边界 | `runtime_config` | `test_runtime_config.c` |
| 状态机全路径（含 token 时序 / RECOVERY） | `provisioning` | `test_provisioning.c` |
| BLE 分帧重组 + candidate 校验 | `ble_provision/ble_proto` | `test_ble_proto.c` |

## 编译烧录（需 ESP-IDF ≥ v5.0）

详见 [BUILD.md](BUILD.md)。宿主单测：`cd test/host && ./run.sh`（需本机 ESP-IDF 的 cJSON）。

## 相关

- BLE 配网协议正典：`../protocols/ble/PROVISIONING_V1.md`
- 协议公开定义：`../../smart-ble/core/protocols/hid-command-schema.ts`（事实源）
- 固件详细设计：`../docs/archive/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md`
- MQTT / HTTP 协议：`../docs/archive/04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md`
- 验收清单：`../docs/archive/10_ACCEPTANCE_CHECKLIST.md` §B
- Go 参考实现：`../smart-hid-controlhub/cmd/mock-device/`
- F2 验证脚本：`../smart-hid-controlhub/scripts/test-loop-f2.sh`
- 发布构建：`../scripts/build-firmware.sh`（fullclean + 产物清单）
