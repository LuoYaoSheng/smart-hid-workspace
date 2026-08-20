---
status: SUPERSEDED
authority: historical-only
do_not_implement: true
current_source: docs/current/CURRENT_STATE.md
---

# docs/archive — 历史设计资料包（SUPERSEDED）

本目录是 2026-08-11 的设计资料包快照，原样保留设计推演全过程。

- 其中 Cloud / Trial / License / Order / Payment / Portal / Admin / 商业化
  相关设计**已从产品移除**（2026-08 开源瘦身），不会恢复。
- 所有文件带 `status: SUPERSEDED` 标识，`do_not_implement: true`。
- **本文档不得作为当前实现依据**；当前事实见
  [docs/current/CURRENT_STATE.md](../current/CURRENT_STATE.md)。

| 文件 | 内容 |
|---|---|
| `00_README.md` | 资料包总览（含当时的产品边界结论，已被取代） |
| `01_PRODUCT_PRD.md` | 产品 PRD v1.0（含已移除的商业角色） |
| `02_SYSTEM_ARCHITECTURE.md` | 系统架构与仓库拆分 v1.1（含 Cloud 层，已移除） |
| `03_BLE_PROVISIONING_PROTOCOL_V1.1.md` | BLE 配网协议设计（固件侧未实现，M1-G3） |
| `04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md` | MQTT / HTTP 协议 v1.0（现行协议的雏形） |
| `05_CONTROLHUB_DETAIL_DESIGN_V1.0.md` | ControlHub 详细设计（含 Trial/License 模块，已移除） |
| `06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md` | 固件详细设计 |
| `07_SMART_HID_WEB_PRD_V1.0.md` | 商业用户中心 PRD（整体已移除，DO NOT IMPLEMENT） |
| `08_MINIAPP_HID_MODULE_V1.2.md` | 小程序 HID 模块页面结构（smart-ble 仓的规划参考） |
| `09_LOCAL_DEVELOPMENT_ROADMAP.md` | 旧 Phase 0～7 路线图（已被 M1/M2 Gate 模型取代） |
| `10_ACCEPTANCE_CHECKLIST.md` | 旧验收清单（含 Trial/License/Web 商业项，已被取代） |
| `11_SUPERSEDED_DECISIONS.md` | 当时的废弃决定记录（其本身也被后续演进取代） |
| `manifest.json` | 资料包元数据 |
