# Smart HID Workspace

本地开发工作区，承载 Smart HID 系统（除公开的 `smart-ble` 外）的私有仓库与共享协议。

日期基线：2026-08-11（对应 `smart-hid-development-pack-v1.0`）。

## 1. 系统边界

```text
第三方程序
  ↓ HTTP
ControlHub（Windows 本地）
  ↓ MQTT
ESP32-S3
  ↓ USB HID
目标电脑
```

辅助链路：

```text
BLE Toolkit+ 微信小程序  ──BLE──▶  ESP32-S3   （设备配置 / 配网 / 诊断）
Smart HID Web            ──HTTPS──▶ Smart HID Cloud （账号 / 订单 / License）
```

实时控制永远本地优先，不经过云。

## 2. 仓库组成

| 仓库 | 角色 | 可见性 | 位置 |
|------|------|--------|------|
| `smart-ble` | BLE Toolkit+ + Smart HID 配网页面 + 公开协议定义 | Public | `../smart-ble/`（独立仓库，本工作区引用） |
| `smart-hid-controlhub` | Windows 本地控制程序（Go） | Private | `./smart-hid-controlhub/` |
| `smart-hid-firmware` | ESP32-S3 固件 | Private | `./smart-hid-firmware/` |
| `smart-hid-cloud` | 账号 / 订单 / License / 签发 | Private | `./smart-hid-cloud/` |
| `smart-hid-web` | 用户门户 + 管理后台 | Private | `./smart-hid-web/` |
| `protocols` | 私有侧共享协议 Schema 与示例 | Private | `./protocols/` |
| `docs` | 资料包文档（事实源镜像） | Private | `./docs/` |

> `smart-ble` 是独立仓库，不复制进本工作区。开发时通过相对路径或 git submodule 引用。

## 3. 事实源（Source of Truth）

| 内容 | 位置 |
|------|------|
| BLE Provisioning Protocol（公开） | `../smart-ble/core/protocols/hid-provisioning-protocol.ts` |
| MQTT Command Schema（公开） | `../smart-ble/core/protocols/hid-command-schema.ts` |
| ControlHub HTTP API（私有） | `./smart-hid-controlhub/docs/openapi.yaml` |
| License 格式（私有） | `./smart-hid-cloud/docs/license-format.md` |
| 协议 JSON Schema（私有侧共享） | `./protocols/schemas/` |

公开协议定义放在 `smart-ble`（公开仓库）；私有侧设计文档与 HTTP/License 规格放在本工作区对应子项目。

## 4. 开发顺序（资料包 Phase 路线）

1. **Phase 0** — 准备工作区（本步骤已完成）
2. **Phase 1** — 本地控制最小闭环：ControlHub HTTP → MQTT → ESP32 → ACK
3. **Phase 2** — 可靠性（request_id / dedup / boot_id / TTL / QoS1 / LWT / lease）
4. **Phase 3** — BLE Provision（smart-ble 小程序 HID 模块 + Firmware Provision Mode）
5. **Phase 4** — ControlHub 产品化（Tray / Local Web / SQLite / Pairing UI / Installer）
6. **Phase 5** — Trial（设备锚点 / 有效控制会话 / 体验限制）
7. **Phase 6** — 商业化（Cloud 用户/套餐/订单/支付/License/签发；Web 门户；ControlHub 激活/刷新/离线导入）
8. **Phase 7** — 生产安全（Secure Boot / Flash Encryption / NVS Encryption / 固件签名 / 工厂烧录工具）

详见 `docs/09_LOCAL_DEVELOPMENT_ROADMAP.md`。

> 当前工作区仅含脚手架与文档落位，未实现任何 Phase 的真实功能代码。

## 5. 当前结论（资料包）

- 微信小程序是开源个人小程序，不做会员 / 支付 / 订单 / License。
- Smart HID 设备当前**没有设备二维码**；小程序通过"搜索附近 Smart HID → BLE 连接 → 读取 Device Info"识别设备。
- 当前唯一需要二维码的是 ControlHub 动态 Pairing QR。
- 实时控制本地优先，不经过云。
- 免费体验限制"累计有效控制时间"，不阉割键鼠功能。
- 付费授权通过云端签名 License，ControlHub 本地离线验签；License 主要绑定 ESP32 Device ID。
- ControlHub 默认双击运行、系统托盘、浏览器本地控制台，不做 Windows Service V1。
- 小程序底部导航：`设备 | HID | 广播 | 关于`。

## 6. 文档索引

`docs/` 下为资料包原样文档：

- `00_README.md` — 资料包总览
- `01_PRODUCT_PRD.md` — 产品 PRD
- `02_SYSTEM_ARCHITECTURE.md` — 系统架构与仓库拆分
- `03_BLE_PROVISIONING_PROTOCOL_V1.1.md` — BLE 配网协议
- `04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md` — MQTT 协议与 ControlHub HTTP API
- `05_CONTROLHUB_DETAIL_DESIGN_V1.0.md` — ControlHub 详细设计
- `06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md` — 固件详细设计
- `07_SMART_HID_WEB_PRD_V1.0.md` — Web PRD
- `08_MINIAPP_HID_MODULE_V1.2.md` — 小程序 HID 模块
- `09_LOCAL_DEVELOPMENT_ROADMAP.md` — 本地开发路线图
- `10_ACCEPTANCE_CHECKLIST.md` — 验收清单
- `11_SUPERSEDED_DECISIONS.md` — 已废弃决策
- `manifest.json` — 资料包清单

## 7. 备注

- 本工作区**不修改线上 `smart-ble` 仓库的对外行为**；smart-ble 内的 HID 扩展作为新增模块，独立演进。
- 资料包文档为镜像落位，**不在此工作区内改写文档原文**；如需修订协议，先改事实源（`smart-ble/core/protocols/` 或对应子项目 docs），再回流资料包。
