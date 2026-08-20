---
status: SUPERSEDED
authority: historical-only
do_not_implement: true
current_source: docs/current/CURRENT_STATE.md
---

> ⚠️ **历史资料（SUPERSEDED）**：本文件属 2026-08-11 设计资料包快照，其中 Cloud / Trial / License / 商业化等设计已于 2026-08 从产品移除。本文仅作设计推演的历史记录保留，**不得作为当前实现依据**；当前事实见 `docs/current/CURRENT_STATE.md`。

# Smart HID 本地开发资料包 v1.0

日期：2026-08-11

## 1. 当前最终产品边界

```text
第三方程序
  ↓ HTTP
ControlHub（Windows 本地）
  ↓ MQTT
ESP32-S3
  ↓ USB HID
目标电脑
```

另外两条辅助链路：

```text
BLE Toolkit+ 微信小程序
  ↓ BLE
ESP32-S3
```

用于设备配置、Wi-Fi、ControlHub 配对和诊断。

```text
Smart HID Web
  ↓ HTTPS
Smart HID Cloud
```

用于账号、购买、订单、License、设备授权和离线 License 下载。

## 2. 当前正式结论

- 微信小程序是开源个人小程序，不做会员、支付、订单、License。
- Smart HID 设备当前**没有设备二维码**。
- 小程序通过“搜索附近 Smart HID → BLE 连接 → 读取 Device Info”识别设备。
- 当前唯一需要二维码的是 ControlHub 动态 Pairing QR。
- 实时控制永远本地优先，不经过云。
- 免费体验限制“累计有效控制时间”，不阉割键鼠功能。
- 付费授权通过云端签名 License，ControlHub 本地离线验签。
- License 主要绑定 ESP32 Device ID。
- ControlHub 默认双击运行、系统托盘、浏览器本地控制台，不做 Windows Service V1。
- 小程序底部导航：设备 / HID / 广播 / 关于。

## 3. 文档索引

1. `01_PRODUCT_PRD.md`
2. `02_SYSTEM_ARCHITECTURE.md`
3. `03_BLE_PROVISIONING_PROTOCOL_V1.1.md`
4. `04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md`
5. `05_CONTROLHUB_DETAIL_DESIGN_V1.0.md`
6. `06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md`
7. `07_SMART_HID_WEB_PRD_V1.0.md`
8. `08_MINIAPP_HID_MODULE_V1.2.md`
9. `09_LOCAL_DEVELOPMENT_ROADMAP.md`
10. `10_ACCEPTANCE_CHECKLIST.md`
11. `11_SUPERSEDED_DECISIONS.md`

## 4. Starter

`starter/` 里放了本地开发用的：

- 仓库结构建议
- 协议 Schema 起始文件
- Command / ACK / Status 示例
- 各子项目 README 起始文件

这不是完整代码，只是为了让本地开发快速开工。
