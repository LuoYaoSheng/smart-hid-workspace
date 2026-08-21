# Smart HID 本地开发路线图

## Phase 0：准备工作区

```text
Smart-HID-Workspace/
├── smart-ble/
├── smart-hid-controlhub/
└── smart-hid-firmware/
└── smart-hid-web/
```

## Phase 1：本地控制最小闭环

### ControlHub
只做：
- Go app
- HTTP health
- Embedded MQTT
- Device Manager
- Command API

### Firmware
只做：
- USB Keyboard / Mouse
- 固定 Wi-Fi
- 固定 MQTT
- Command Parser
- ACK

验收：

```text
curl
→ ControlHub
→ MQTT
→ ESP32
→ ENTER
```

然后：
- mouse move
- click
- hotkey

## Phase 2：可靠性

实现：
- request_id
- dedup
- boot_id
- TTL
- QoS1
- status
- LWT
- lease
- release_all
- queue

## Phase 3：BLE Provision

小程序：
- 首页通用扫描识别 Smart HID Profile
- 自动 BLE connect + Device Info 验证
- 单页填写 Wi-Fi / ControlHub，二维码带入一次性 token
- 下发 candidate 并查看 status

Firmware：
- Provision Mode
- Device Info
- Wi-Fi Provision
- Pair ControlHub
- Persist config

## Phase 4：ControlHub 产品化

- Tray
- Local Web
- Single Instance
- SQLite
- Pairing UI
- Installer
- Startup
- Diagnostics

## Phase 5：Production Security

Firmware:
- Secure Boot
- Flash Encryption
- NVS Encryption
- Firmware Signing
- Factory Provisioning Tool

不要在功能闭环之前锁生产 eFuse。
