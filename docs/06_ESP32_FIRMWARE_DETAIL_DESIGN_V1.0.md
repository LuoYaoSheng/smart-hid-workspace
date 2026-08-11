# ESP32-S3 Smart HID Firmware 详细设计 v1.0

## 1. 核心职责

- USB Composite HID
- BLE Provisioning
- Wi-Fi
- ControlHub Pairing
- MQTT Client
- Command Queue
- Dedup
- HID Engine
- Lease
- release_all
- Status
- Device Identity
- NVS
- Diagnostics
- OTA Foundation

## 2. USB

V1 Composite：

```text
Smart HID
├── Keyboard HID
└── Mouse HID
```

开发版可启 CDC Debug，生产版默认不暴露 CDC。

## 3. Keyboard

- tap
- hotkey
- key_down
- key_up

内部维护当前 pressed keys / modifiers。

key_down 必须带 Lease。

## 4. Mouse

- relative move
- click
- button_down
- button_up
- wheel

大 dx/dy 由 Firmware 自动拆多个 HID Report。

## 5. Fail-safe

任何控制链异常优先：

```text
hid_release_all()
```

至少触发：
- MQTT disconnect
- Wi-Fi disconnect
- USB reset
- USB re-enumeration
- command engine error
- reboot
- lease timeout

## 6. MQTT

订阅：

```text
smart-hid/v1/devices/{device_id}/command
```

发布：
- ack
- status
- event

MQTT callback 不直接发 HID Report。

正确：

```text
MQTT RX
→ Parser
→ Command Queue
→ Command Worker
→ HID Engine
```

## 7. Command Queue

建议：
- size 32
- bounded
- serial execution
- payload max 2048 bytes

## 8. boot_id

每次启动新生成。

Command 中 target_boot_id 必须匹配。

不匹配：
`STALE_DEVICE_SESSION`

## 9. Dedup

RAM 保存最近约 256 个 request_id。

同 request_id 第二次收到：
- 不执行 HID
- 返回 duplicate

## 10. BLE Provision

当前无 Device QR。

流程：
- Provision Mode 广播
- 小程序搜索
- BLE connect
- get_info
- hub-config
- Wi-Fi Provision
- Pair ControlHub
- MQTT

## 11. NVS

Namespace 建议：

```text
identity
network
hub
security
trial
```

普通 reset 不清 identity / trial anchor。

## 12. Trial Anchor

Firmware 只保存辅助锚点，不负责完整 Trial 计费和会员判断。

## 13. OTA

从 V1 Partition Table 就预留双 OTA。

## 14. Security Profile

必须分：

```text
debug
qa
production
```

Production 再启用：
- Secure Boot
- Flash Encryption
- NVS Encryption
- Firmware Signing

不要第一阶段就锁 eFuse。

## 15. 开发里程碑

### F1
USB Keyboard/Mouse + 固定 Wi-Fi/MQTT + ControlHub

### F2
ACK / request_id / dedup / boot_id / TTL

### F3
BLE Provision + Pairing

### F4
NVS Security + Trial Anchor + OTA Foundation

### F5
Production Security + Factory Tool
