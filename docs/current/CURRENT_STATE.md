---
status: CURRENT
authority: canonical
---

# CURRENT_STATE — 当前事实状态

> 本文是 Smart HID「现在是什么 / 不是什么 / 完成到哪里」的**唯一权威事实源**。
> 与代码冲突时，以 main 分支代码为准，并立即更新本文。
> 历史设计资料在 `docs/archive/`，不得作为当前实现依据。

最后核对：2026-08-20，基线 commit `5c2e5dc`。

## 产品定位

主链路（已实现）：

```text
第三方程序
  → HTTP
  → ControlHub（本机，Go）
  → MQTT（内嵌 broker，仅局域网）
  → ESP32-S3
  → USB HID
  → 目标电脑
```

辅助链路（**规划中，固件侧未实现**）：

```text
BLE Toolkit+ 微信小程序（独立仓库 smart-ble）
  → BLE
  → ESP32-S3（配网 / 诊断）
```

诚实状态：BLE 配网目前只有协议设计（历史文档 `docs/archive/03`）与独立小程序仓库
smart-ble 的规划。**固件侧没有任何 BLE / NVS Provisioning 代码**——Wi-Fi 与 MQTT
参数来自编译期 Kconfig 固定值（`CONFIG_SMART_HID_WIFI_SSID` 等）。BLE Provision
整体属 PLANNED，排在 M1-G3。

## 当前不是什么（REMOVED — 禁止恢复）

以下能力已于 2026-08（OS-1 开源瘦身）从代码库**移除**，状态 SUPERSEDED /
DO NOT IMPLEMENT：

| 已移除 | 处置 |
|---|---|
| smart-hid-cloud | 云端整目录已删除（账号 / 订单 / 支付 / License 签发） |
| Trial / License / Entitlement 门禁 | ControlHub 内部模块、402 路径、用量统计全部删除 |
| Usage / License 查询 API | openapi 从 17 path 缩减到 11（已移除） |
| Web Portal / Admin 商业前端 | 页面已删除，官网转纯静态作品集 |
| 小程序商业中心 | 早期设计已废弃（见 archive/11） |

保留途径：git 历史 + 本地 `commercial-backup` 分支（未推送）。除非用户明确重新立项，
**任何情况下不得恢复或重新实现**（见 [DEVELOPMENT_RULES](DEVELOPMENT_RULES.md) §2）。

## 组件状态

状态值语义：`IMPLEMENTED` 已实现且验证 ／ `PARTIAL` 部分实现 ／ `PLANNED` 仅规划
无代码 ／ `NOT VERIFIED ON HARDWARE` 源码完成但未经真机 ／ `SUPERSEDED` 已废弃。
禁止「差不多完成 / 基本完成」等模糊表述。

| 组件 | 状态 | 说明 |
|---|---|---|
| ControlHub（Go） | IMPLEMENTED | HTTP API + 内嵌 MQTT + 配对 + API Key + SQLite + 托盘 + Web 三页面；`go test` 全绿 + mock e2e |
| 固件（C，ESP-IDF v5.4.4） | IMPLEMENTED ＋ NOT VERIFIED ON HARDWARE | USB HID / 命令引擎 / 可靠性语义源码完成、编译通过、28 项 mock e2e；**从未在真实 ESP32-S3 上烧录验证** |
| mock-device（cmd/mock-device） | IMPLEMENTED | Go 参考实现 = 虚拟 ESP32，测试替身；连非默认端口需 `--mqtt-port` |
| 官网（smart-hid-web） | IMPLEMENTED | 纯静态落地页 / 文档站 / 下载中心，GitHub Pages 托管 |
| protocols/ JSON Schema | IMPLEMENTED | command / ack / status 三 schema + 示例 |
| BLE 配网（固件侧） | PLANNED | 无代码，Wi-Fi/MQTT 为 Kconfig 固定值；M1-G3 |
| BLE 配网（小程序侧） | PARTIAL（外部独立仓 smart-ble） | 不在本仓库，进度以该仓库为准 |
| OTA | PLANNED | 仅分区表预留双 OTA 分区 |
| Secure Boot / Flash Encryption / 固件签名 | PLANNED | M2-G3 |
| 真机烧写与硬件验收 | NOT EXECUTED | 独立任务（M2-G1），明确不在 M1 内做 |

## ControlHub 能力清单（按代码核对）

- HTTP API：11 path，事实源 `smart-hid-controlhub/docs/openapi.yaml`
- 内嵌 MQTT broker（mochi-mqtt）+ PerDeviceHook：hub 账号 + 每设备随机凭据 + per-device topic ACL
- 命令闭环：POST → publish（QoS1，严禁 retain）→ 同步等 ACK；TTL 内未终态返回 202
- 配对：Web 建 session → 动态 QR（`shid://pair?...`）→ 设备 POST `:17892` 换 MQTT 凭据（一次性 token，5 分钟）
- API Key：SHA-256 入库、明文不落库、可轮换；HTTP 用 Bearer，WebSocket 用 query key
- Web 三页面：控制台 `/`、模拟键鼠演示台 `/demo.html`（可视化键盘 / 触控板 / 实体键盘直通 / 文本连打 / 多设备广播）、实时事件通道 `/api/v1/realtime`
- 配置面：`http.lan_mode` / `http.enable_api`、`mqtt.*`、`pairing.enabled` / `pairing.port`、`web.console` / `web.demo` / `web.realtime`；缺省 = 全开内置默认
- SQLite 持久化：migrations 0001 / 0002
- 系统托盘常驻（`-tray`）与 headless 双生命周期

## 固件能力清单（按代码核对）

- USB Composite HID：键盘 + 鼠标（TinyUSB）
- 命令引擎：串行队列（32）／ request_id 去重（256）／ TTL ／ target_boot_id ／ lease 超时释放 ／ release_all
- Fail-safe：MQTT / Wi-Fi 断开 → 设备端自动释放全部按键（LWT 语义）
- Wi-Fi / MQTT：Kconfig 编译期固定值（无运行时配置，无 NVS 配网）
- 宿主单测：dedup_cache、hid_keymap（`smart-hid-firmware/test/host/`）

## 验证状态（诚实边界）

- `go test ./...` 全绿；`test-loop-f2.sh` 28/28（mock-device 端到端，无需硬件）
- 配置面 e2e、WebSocket e2e、演示台 Playwright 真浏览器实测均通过（2026-08-20）
- 以上**全部基于 mock / 本机环境**：没有任何真机验证（ESP32-S3 烧写、USB 枚举、
  BIOS / 登录界面、三操作系统、soak 均未执行）
