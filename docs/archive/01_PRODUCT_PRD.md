---
status: SUPERSEDED
authority: historical-only
do_not_implement: true
current_source: docs/current/CURRENT_STATE.md
---

> ⚠️ **历史资料（SUPERSEDED）**：本文件属 2026-08-11 设计资料包快照，其中 Cloud / Trial / License / 商业化等设计已于 2026-08 从产品移除。本文仅作设计推演的历史记录保留，**不得作为当前实现依据**；当前事实见 `docs/current/CURRENT_STATE.md`。

# Smart BLE × ControlHub 网络 HID 控制系统 PRD v1.0

## 1. 产品定义

Smart HID 是一个本地优先的网络 HID 控制系统，由：

- ESP32-S3 Smart HID 硬件
- ControlHub Windows 本地程序
- BLE Toolkit+ 微信小程序
- Smart HID Web
- Smart HID Cloud

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
- Trial
- License Verify
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

### Smart HID Web
- 登录
- 套餐
- 订单
- License
- 设备授权
- 离线授权下载

### Smart HID Cloud
- User
- Plan
- Order
- Payment
- License
- Activation
- Device Binding
- License Signer

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

## 6. Trial

免费用户：
- 键盘可用
- 鼠标可用
- HTTP API 可用
- 本地 MQTT 可用

限制方式：
- 累计有效控制时间
- 第一条成功 HID Command 才启动 Trial Session
- 无操作时不消耗
- 配置/状态/诊断不消耗

具体总体验时长不在 UI 和协议中写死。

## 7. License

License Cloud 签名，ControlHub 本地验签。

推荐载荷：

```text
license_id
account_id
plan_id
device_id
issued_at
valid_from
expires_at
features
license_version
signature
```

主绑定对象：ESP32 Device ID。

## 8. 购买与激活

购买和绑定分离：

```text
购买套餐
→ 获得 UNUSED License
→ 选择 Device
→ 激活
→ 云端签发
→ ControlHub 下载/导入
→ 本地验签
```

## 9. 首次使用

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

## 10. V1 成功标准

- 无云环境下，本地有效 License 可以继续控制。
- ESP32 离线期间不积压 HID Command。
- 重连后不重放旧 Command。
- QoS1 重投不会导致同一 HID 动作重复执行。
- 按键/鼠标按钮不会因网络断开永久卡住。
- 小程序完全不依赖会员/License/订单系统。
