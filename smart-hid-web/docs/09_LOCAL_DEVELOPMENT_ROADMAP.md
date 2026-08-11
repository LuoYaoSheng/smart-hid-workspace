# Smart HID 本地开发路线图

## Phase 0：准备工作区

```text
Smart-HID-Workspace/
├── smart-ble/
├── smart-hid-controlhub/
├── smart-hid-firmware/
├── smart-hid-cloud/
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
- HID Tab
- Search Smart HID
- BLE connect
- get_info
- ControlHub QR
- Wi-Fi
- status

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

## Phase 5：Trial

- Device-based Trial Anchor
- Effective Control Session
- Session idle
- Usage persistence
- Trial expired gate

## Phase 6：Commercial

Cloud:
- User
- Plan
- Order
- Payment
- License
- Activation
- Signer

Web:
- Portal
- License
- Device
- Order
- Download

ControlHub:
- activation
- refresh
- offline import

## Phase 7：Production Security

Firmware:
- Secure Boot
- Flash Encryption
- NVS Encryption
- Firmware Signing
- Factory Provisioning Tool

不要在功能闭环之前锁生产 eFuse。
