# smart-hid-firmware

ESP32-S3 Smart HID 固件。USB Composite HID（Keyboard + Mouse）+ Wi-Fi + MQTT + BLE Provisioning。

```text
ControlHub ──MQTT──▶ ESP32-S3 ──USB HID──▶ 目标电脑
BLE Toolkit+ ──BLE──▶ ESP32-S3 （配网 / 诊断）
```

订阅 MQTT Command → 解析 → dedup → boot_id 校验 → 入队 → worker 串行执行 → 发 ACK。
MQTT callback 不直接发 HID Report。

## 当前状态

✅ **F1+F2 C 源码骨架完成**（2026-08-11）：
- USB Composite HID（Keyboard + Mouse）报告描述符与发送逻辑
- 协议层 JSON 解析/序列化（镜像 smart-ble TS 事实源）
- command_engine：queue(32) + dedup(256) + boot_id 校验 + TTL + worker task + lease tick
- mqtt_manager：连接 / 订阅 / LWT / publish ack+status+event
- wifi_manager / status_manager / device_identity(NVS)
- main.c 装配全部组件

✅ **F2 可靠性语义已通过 Go 参考实现端到端验证**（`../smart-hid-controlhub/cmd/mock-device` + `scripts/test-loop-f2.sh`，28/28 全过）：
- request_id 去重（duplicate）
- target_boot_id 防旧命令（rejected STALE_DEVICE_SESSION）
- TTL 过期 / 范围校验
- queue full 明确返回
- lease 超时自动释放
- system/release_all 清空所有 pressed keys/buttons
- MQTT 断开 → release_all

⚠️ **本机无 ESP-IDF 工具链**，C 代码尚未编译/烧录验证。语义对齐由 Go 参考实现保证。

## 目录结构

```text
smart-hid-firmware/
├── CMakeLists.txt              # 顶层 ESP-IDF 工程
├── Kconfig.projbuild           # Smart HID 配置项
├── partitions.csv              # 含双 OTA + NVS
├── sdkconfig.defaults          # 默认配置（target=esp32s3, TinyUSB HID）
├── main/
│   ├── CMakeLists.txt
│   └── main.c                  # app_main 装配
└── components/
    ├── smart_hid_protocol/     # 协议契约（镜像 TS 事实源）
    ├── device_identity/        # device_id(NVS) + boot_id
    ├── command_engine/         # queue + dedup + worker + lease tick
    │   └── dedup_cache.c
    ├── hid_engine/             # USB HID 报告 + lease + release_all
    │   └── hid_keymap.c
    ├── mqtt_manager/           # esp-mqtt 封装 + LWT
    ├── wifi_manager/           # STA + 断开 release_all
    └── status_manager/         # 心跳 status
```

## 组件 ↔ C 文件 ↔ Go 参考 对照

| 验收项 (§B) | C 组件 | Go 参考 (mock-device) |
|------------|--------|----------------------|
| request_id 去重 | `command_engine/dedup_cache.c` | `DedupCache.CheckAndAdd` |
| target_boot_id | `command_engine.c` + `device_identity.c` | `Device.HandleCommand` boot_id 分支 |
| lease 超时释放 | `hid_engine.c tick_leases` | `LeaseManager.startTicker` |
| MQTT 断开 release_all | `mqtt_manager.c` + `wifi_manager.c` | `OnConnectionLost` |
| queue full | `command_engine.c` queue_full 分支 | `Device.HandleCommand` queue 满 |
| system/release_all | `hid_engine_release_all` | `LeaseManager.ReleaseAll` |

## 编译烧录（需 ESP-IDF ≥ v5.0）

详见 [BUILD.md](BUILD.md)。

## 相关

- 协议公开定义：`../../smart-ble/core/protocols/hid-command-schema.ts`（事实源）
- 固件详细设计：`../docs/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md`
- MQTT / HTTP 协议：`../docs/04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md`
- 验收清单：`../docs/10_ACCEPTANCE_CHECKLIST.md` §B
- Go 参考实现：`../smart-hid-controlhub/cmd/mock-device/`
- F2 验证脚本：`../smart-hid-controlhub/scripts/test-loop-f2.sh`
