---
status: SUPERSEDED
authority: historical-only
do_not_implement: true
current_source: docs/current/CURRENT_STATE.md
---

> ⚠️ **历史资料（SUPERSEDED）**：本文件属 2026-08-11 设计资料包快照，其中 Cloud / Trial / License / 商业化等设计已于 2026-08 从产品移除。本文仅作设计推演的历史记录保留，**不得作为当前实现依据**；当前事实见 `docs/current/CURRENT_STATE.md`。

# Smart HID Web 用户中心 PRD v1.0

## 1. 定位

Smart HID Web 是唯一商业用户中心。

负责：
- 登录
- 套餐
- 购买
- 订单
- License
- 设备授权
- 续费
- 离线 License
- ControlHub 下载

不负责：
- BLE
- Wi-Fi
- MQTT 实时控制
- HID 实时状态

## 2. IA

```text
Smart HID Web
├── 首页
├── 套餐
├── 我的设备
├── 我的 License
├── 我的订单
├── 下载中心
├── 帮助
└── 账户
```

## 3. 购买

```text
套餐
→ 确认订单
→ 支付
→ License Created
→ 状态 UNUSED
```

购买与设备绑定分离。

## 4. 激活

```text
ControlHub
→ 生成 Activation Session
→ 打开 Web / 显示 Web QR
→ Web 登录
→ 选择 License
→ 绑定 Device
→ Cloud 签发
→ ControlHub Refresh
→ 本地验签
```

## 5. 离线电脑

```text
离线 ControlHub
→ Activation Code
→ 手机/其他电脑访问 Web
→ 购买/激活
→ 下载 .license
→ 拷贝
→ ControlHub Import
```

## 6. License 状态

- UNUSED
- ACTIVE
- EXPIRED
- DISABLED
- REVOKED

## 7. 设备

Web 中的 Device 是“授权意义上的设备”。

不要伪造实时 MQTT 在线状态。

## 8. 数据模型

```text
users
plans
orders
payments
devices
licenses
license_device_bindings
activations
```

## 9. Web 页面

V1：
- 首页
- 登录
- 概览
- 套餐
- 确认订单
- 支付结果
- 我的设备
- 设备详情
- 我的 License
- License 详情
- 激活
- 离线授权
- 我的订单
- 订单详情
- 下载中心
- 账户

## 10. 管理后台

可以与 smart-hid-web 同仓不同入口。

V1 Admin：
- Dashboard
- Users
- Devices
- Plans
- Orders
- Licenses
- Activations
