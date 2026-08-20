# Smart BLE × ControlHub 网络 HID 控制系统 PRD v1.0

## 1. 产品定义

Smart HID 是一个本地优先的网络 HID 控制系统，由：

- ESP32-S3 Smart HID 硬件
- ControlHub Windows 本地程序
- BLE Toolkit+ 微信小程序

组成。

核心价值：

> 将第三方软件的 HTTP API 指令，经 ControlHub → MQTT → ESP32-S3 → USB HID 转换为真实物理键盘/鼠标输入。

## 2. 目标用户

- 需要通过标准 USB HID 控制电脑的开发者
- 自动化测试
- 本地设备联动
- 授权范围内的键鼠自动操作
- 希望目标电脑无需安装专用驱动/控制客户端的场景

## 3. 系统角色

### ESP32-S3
- USB Keyboard
- USB Mouse
- Wi-Fi
- MQTT
- BLE Provisioning
- Device Identity
- Command Execute

### ControlHub
- HTTP API
- Embedded MQTT Broker
- Device Manager
- Command Engine
- Local Web UI
- Tray
- SQLite

### BLE Toolkit+
- BLE 搜索
- Smart HID 配网
- Wi-Fi 配置
- ControlHub Pairing
- 诊断
- 通用 BLE 工具

## 4. 核心运行链路

```mermaid
flowchart LR
A[第三方程序] -->|HTTP| B[ControlHub]
B -->|MQTT| C[ESP32-S3]
C -->|USB HID| D[目标电脑]
```

实时控制不依赖互联网。

## 5. V1 HID 功能

### Keyboard
- tap
- hotkey
- key_down
- key_up

### Mouse
- relative move
- click
- button_down
- button_up
- wheel

### System
- release_all

V1 不做：
- Arbitrary text typing
- Macro Script Engine
- Shell
- File Transfer
- Remote Desktop
- Clipboard Read
- Screen Capture

## 6. 开放使用

无账号、无订阅、无授权门禁：
- 键盘可用
- 鼠标可用
- HTTP API 可用
- 本地 MQTT 可用

下载、编译、运行即为完全体。

## 7. 首次使用

```text
安装 ControlHub
→ 插入 Smart HID
→ 打开 BLE Toolkit+
→ 搜索附近 Smart HID
→ BLE 连接
→ 读取 Device ID
→ 扫 ControlHub 动态二维码
→ 选择 Wi-Fi
→ 配置
→ ESP32 Pair ControlHub
→ MQTT Online
→ 免费开始使用
```

## 8. V1 成功标准

- 无云环境下（本系统默认无云），控制链路完整可用。
- ESP32 离线期间不积压 HID Command。
- 重连后不重放旧 Command。
- QoS1 重投不会导致同一 HID 动作重复执行。
- 按键/鼠标按钮不会因网络断开永久卡住。
- 小程序完全不依赖任何云端系统。
