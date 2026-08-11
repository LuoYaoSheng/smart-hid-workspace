# smart-hid-firmware

ESP32-S3 Smart HID 固件。USB Composite HID（Keyboard + Mouse）+ Wi-Fi + MQTT + BLE Provisioning。

## 角色定位

```text
ControlHub ──MQTT──▶ ESP32-S3 ──USB HID──▶ 目标电脑
BLE Toolkit+ ──BLE──▶ ESP32-S3 （配网 / 诊断）
```

订阅 MQTT Command，解析 → 入队 → 串行执行 → 发 ACK。MQTT callback 不直接发 HID Report。

## 核心职责

- USB Composite HID（Keyboard + Mouse）
- BLE Provisioning
- Wi-Fi
- ControlHub Pairing
- MQTT Client
- Command Queue（size 32，bounded，serial execution，payload max 2048 bytes）
- Dedup（RAM 保存最近约 256 个 request_id）
- HID Engine
- Lease（key_down / button_down 必须带 lease_ms）
- release_all（任何控制链异常优先触发）
- Status
- Device Identity
- NVS
- Diagnostics
- OTA Foundation

## USB Composite

```text
Smart HID
├── Keyboard HID
└── Mouse HID
```

开发版可启 CDC Debug，生产版默认不暴露 CDC。

## 建议组件结构

来自资料包 `starter/firmware`：

```text
device_identity
config_manager
ble_provision
wifi_manager
hub_pairing
mqtt_manager
command_engine
hid_engine
status_manager
trial_anchor
diagnostics
watchdog
firmware_update
```

## MQTT Topic

```text
订阅：smart-hid/v1/devices/{device_id}/command
发布：smart-hid/v1/devices/{device_id}/ack
      smart-hid/v1/devices/{device_id}/status
      smart-hid/v1/devices/{device_id}/event
```

正确处理路径：

```text
MQTT RX → Parser → Command Queue → Command Worker → HID Engine
```

## Fail-safe

任何控制链异常优先 `hid_release_all()`，至少在以下场景触发：

- MQTT disconnect
- Wi-Fi disconnect
- USB reset / re-enumeration
- command engine error
- reboot
- lease timeout

## boot_id

每次启动新生成。Command 中 `target_boot_id` 必须匹配，不匹配返回 `STALE_DEVICE_SESSION`。

## NVS Namespace

```text
identity  network  hub  security  trial
```

普通 reset 不清 `identity` / `trial` anchor。

## Security Profile

必须分：`debug` / `qa` / `production`。Production 才启用 Secure Boot / Flash Encryption / NVS Encryption / Firmware Signing。

> **不要在功能闭环之前锁生产 eFuse。**

## 开发里程碑

| 里程碑 | 内容 |
|--------|------|
| F1 | USB Keyboard/Mouse + 固定 Wi-Fi/MQTT + ControlHub |
| F2 | ACK / request_id / dedup / boot_id / TTL |
| F3 | BLE Provision + Pairing |
| F4 | NVS Security + Trial Anchor + OTA Foundation |
| F5 | Production Security + Factory Tool |

第一里程碑：固定 Wi-Fi / MQTT + USB HID。

## 当前状态

⚠️ **脚手架阶段**。仅有目录骨架与文档落位，未实现任何功能代码。详见 `../docs/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md`。

## 相关

- BLE 配网协议公开定义：`../../smart-ble/core/protocols/hid-provisioning-protocol.ts`
- MQTT Command Schema 公开定义：`../../smart-ble/core/protocols/hid-command-schema.ts`
- 验收清单：`../docs/10_ACCEPTANCE_CHECKLIST.md` §B
