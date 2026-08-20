---
status: SUPERSEDED
authority: historical-only
do_not_implement: true
current_source: docs/current/CURRENT_STATE.md
---

> ⚠️ **历史资料（SUPERSEDED）**：本文件属 2026-08-11 设计资料包快照，其中 Cloud / Trial / License / 商业化等设计已于 2026-08 从产品移除。本文仅作设计推演的历史记录保留，**不得作为当前实现依据**；当前事实见 `docs/current/CURRENT_STATE.md`。

# 已废弃 / 已修订决定

这份文件用于避免本地开发时误用前面的旧设计。

## 1. 废弃：小程序商业中心

曾经规划：
- 我的
- 微信登录
- 会员
- License
- 订单
- 支付

**全部废弃。**

当前：
- 小程序只做 BLE 工具 + Smart HID 配置
- 商业能力全部进 Smart HID Web

## 2. 废弃：设备二维码前置

曾经规划：

```text
扫设备二维码
→ Device ID
→ Provision Credential
→ BLE
```

**废弃。**

当前：

```text
搜索附近 Smart HID
→ BLE connect
→ get_info
→ Device ID
```

设备当前没有二维码。

## 3. 保留：ControlHub 动态二维码

这个二维码不是设备二维码。

用途：
- Hub ID
- Host
- Pairing Port
- Temporary Pairing Token

继续保留。

## 4. 修订：Security 2

曾经把“每设备二维码凭据 + Security 2”当成 V1 固定前提。

当前：
- 不把每设备二维码凭据写死
- 开发阶段先完成 BLE Provisioning 闭环
- 量产阶段单独冻结设备认证方式

## 5. 修订：私有仓数量

之前：
- smart-hid-admin 单独仓

当前建议：
- smart-hid-web 同时容纳 portal/admin 两个入口
- 保留 4 个私有仓：

```text
smart-hid-controlhub
smart-hid-firmware
smart-hid-cloud
smart-hid-web
```
