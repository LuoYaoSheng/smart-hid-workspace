# Smart HID

**把 HTTP API 调用变成真实的键鼠输入 —— 本地优先的网络 HID 控制系统。**

> **English:** Smart HID turns HTTP API calls into real physical keyboard & mouse input. Your program talks to ControlHub over HTTP; commands travel over MQTT to an ESP32-S3, which emits standard USB HID reports to the target computer. The real-time control path never leaves your LAN.

![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)
![ESP-IDF](https://img.shields.io/badge/ESP--IDF-v5.4.4-C43E00)
![SQLite](https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite)
![USB](https://img.shields.io/badge/USB-HID%20Keyboard%20%2B%20Mouse-3C7EBB)
![local-first](https://img.shields.io/badge/control%20path-local--first-2EA44F)

## 它是什么

第三方程序（脚本、自动化、联动规则……）要操控一台电脑，常规做法是软件级注入：往目标机装软件、模拟系统输入事件——受权限、杀毒软件、受控环境限制，BIOS / 登录界面等场景直接无解。

Smart HID 换了一条路：**让一块 ESP32-S3 以真实 USB 键盘 + 鼠标的身份插在目标电脑上**，任何能发 HTTP 请求的程序都能驱动它：

```text
第三方程序 ──HTTP:17890──▶ ControlHub ──MQTT:17891──▶ ESP32-S3 ──USB HID──▶ 目标电脑
                              │ ▲
                              │ └─ Device Pairing :17892（仅配对会话期开放，供固件 Provision Mode 接入）
```

- **目标机零软件**：插上就是标准键盘鼠标，Windows / macOS / Linux 即插即认，系统层、BIOS 层通用
- **全标准协议**：不发明新协议——HTTP、MQTT、USB HID 全是业界标准，任何语言一行 HTTP 就能驱动
- **本地优先**：实时控制链路不经过云，断网可继续控制；云只负责授权（License 签发 / 刷新）

## 核心特性

- **真实 HID 输出**：USB Composite HID（键盘 + 鼠标），目标机无需安装任何软件
- **可靠性语义**：命令不是"发出去就完"——去重、防旧命令、TTL、队列满明确报错、lease 超时释放、掉线自动释放全部按键（见下表）
- **License 离线验签**：云端 Ed25519 签发，ControlHub 只内置公钥、本地离线验证，绑定 ESP32 Device ID
- **动态设备配对**：ControlHub 生成 Pairing QR（`shid://pair?...`），避免预烧凭证；设备无出厂二维码，靠 BLE 扫描识别
- **BLE 配网**：通过 [smart-ble](https://github.com/LuoYaoSheng/smart-ble)（微信小程序 BLE Toolkit+）完成 Wi-Fi 配网与设备诊断
- **管理闭环**：Cloud + Web 提供用户 / 套餐 / 订单 / 激活码 / License 管理后台；落地页带公开需求反馈通道（honeypot + 限频反垃圾），采纳的需求进入公开路线图

### 可靠性语义

| 机制 | 作用 |
|---|---|
| `request_id` 去重 | 同一命令重复投递只执行一次，幂等 |
| `target_boot_id` | 设备重启后旧会话命令直接拒绝（STALE_DEVICE_SESSION） |
| TTL | 过期命令不再执行，避免迟到动作 |
| 队列满明确报错 | 不静默丢弃，调用方可知、可重试 |
| lease | 长按类操作超时自动释放，防止按键卡死 |
| LWT → release_all | MQTT 异常掉线时设备端自动清空所有已按下键鼠 |

以上语义已由 Go 参考实现（`smart-hid-controlhub/cmd/mock-device` + `smart-hid-firmware/scripts/test-loop-f2.sh`）28 项端到端测试覆盖。

## 仓库组成

| 目录 | 内容 | 技术栈 |
|---|---|---|
| `smart-hid-firmware/` | ESP32-S3 固件：USB Composite HID + Wi-Fi/MQTT + 命令引擎 | C，ESP-IDF v5.4.4 |
| `smart-hid-controlhub/` | Windows 本地控制程序：HTTP API + 内嵌 MQTT Broker + 系统托盘 + 本地控制台 + License 验签 | Go，SQLite |
| `smart-hid-cloud/` | 账号 / 套餐 / 订单 / 支付 / License 签发服务 | Go，SQLite |
| `smart-hid-web/` | 官网落地页 + 用户门户 + 管理后台 | 原生 ES5，零构建静态站 |
| `protocols/` | MQTT Command / ACK / Status JSON Schema 与示例 | JSON Schema |
| `docs/` | 完整设计资料包（PRD / 架构 / 协议 / 详细设计 / 路线图 / 验收清单） | Markdown |

> 配套独立仓库 [smart-ble](https://github.com/LuoYaoSheng/smart-ble)：BLE Toolkit+ 微信小程序（设备配网 / 诊断），BLE Provisioning 协议与 MQTT Command Schema 的 TS 事实源所在。

## 快速开始

### 方式一：预编译包（免构建）

发行物直接放在仓库内，附 SHA256 校验：

| 包 | 路径 | 说明 |
|---|---|---|
| ControlHub | `smart-hid-web/downloads/controlhub/` | macOS arm64 / Windows amd64 双平台 |
| 固件烧录包 | `smart-hid-web/downloads/firmware/` | ESP32-S3 四个 bin + `flash.sh` 烧录脚本 |

### 方式二：源码构建

```bash
# ControlHub 与 Cloud 同属一个 Go workspace（go.work，Go 1.25）
cd smart-hid-controlhub && go build ./cmd/controlhub

# Cloud：先复制 config.example.yaml 为 config.yaml，生成 License 密钥对
cd smart-hid-cloud && cp config.example.yaml config.yaml && bash scripts/gen-keys.sh && go build ./cmd/cloud

# 固件：ESP-IDF v5.4.4（详见 smart-hid-firmware/BUILD.md）
cd smart-hid-firmware && idf.py build

# Web：纯静态零构建，可由 Cloud 直接托管（config.yaml 的 http.web_root 指向 smart-hid-web/）
```

详细上手步骤见 `smart-hid-web/docs/quick-start.html`（源码内直接用浏览器打开即可）。

### 跑测试

```bash
cd smart-hid-controlhub && go test ./...
cd smart-hid-cloud && go test ./...
# 固件可靠性语义 28 项（Go 参考实现，无需硬件）：
bash smart-hid-controlhub/scripts/test-loop-f2.sh
```

## 授权（License）模型

- **试用**：限制"累计有效控制时间"，不阉割任何键鼠功能
- **授权**：云端 Ed25519 签发 `.license` 文件，绑定 ESP32 Device ID；ControlHub 本地离线验签
- **两条激活路径**：激活码在线激活（ControlHub 输码 → Cloud 换 License），或离线导入 `.license` 文件
- **离线优先**：已导入的 License 断网仍有效；云端禁用 / 吊销会阻止重新下载与续费，但不追杀已离线导入的 License（实时吊销 CRL 属 Phase 7 生产安全）
- 格式规格：`smart-hid-cloud/docs/license-format.md`

## 当前状态（2026-08）

| 组件 | 状态 |
|---|---|
| 固件 | ✅ F1 控制 + F2 可靠性源码完成，ESP-IDF v5.4.4 编译通过（921KB，factory 分区 1MB 内）；28/28 可靠性语义端到端验证通过。⚠️ 尚未在真实 ESP32-S3 硬件上烧录验证 |
| ControlHub | ✅ 产品化完成：托盘常驻 / 本地控制台 / SQLite 持久化 / 动态配对 / Trial / License 离线导入·在线激活·刷新 |
| Cloud | ✅ 用户 / 套餐 / 订单 / 支付 / License / 设备 / 激活码 / admin 后台 + 公开需求反馈闭环，e2e 全通 |
| Web | ✅ 落地页 / 文档站 / 用户门户 / 管理后台 / 路线图社区需求区 |
| 待办 | 真机烧录验证、固件 Provision Mode、生产安全（Secure Boot / Flash Encryption / 固件签名，Phase 7） |

## 安全设计

- 实时控制链路不出局域网；Cloud 不在控制链路内
- ControlHub：控制 API 默认只听 `127.0.0.1`；LAN API 需用户显式开启 + Bearer API Key；内嵌 MQTT Broker 带认证与 ACL；敏感数据 Windows DPAPI 保护
- 公开反馈端点的威胁模型与反代注意事项：`smart-hid-cloud/docs/feedback.md`

## 文档

设计资料包（`docs/`）：

| 文档 | 内容 |
|---|---|
| `01_PRODUCT_PRD.md` | 产品 PRD |
| `02_SYSTEM_ARCHITECTURE.md` | 系统架构与仓库拆分 |
| `03_BLE_PROVISIONING_PROTOCOL_V1.1.md` | BLE 配网协议 |
| `04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md` | MQTT 协议与 ControlHub HTTP API |
| `05_CONTROLHUB_DETAIL_DESIGN_V1.0.md` | ControlHub 详细设计 |
| `06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md` | 固件详细设计 |
| `07_SMART_HID_WEB_PRD_V1.0.md` | Web PRD |
| `08_MINIAPP_HID_MODULE_V1.2.md` | 小程序 HID 模块 |
| `09_LOCAL_DEVELOPMENT_ROADMAP.md` | 开发路线图 |
| `10_ACCEPTANCE_CHECKLIST.md` | 验收清单 |

其他事实源：协议 JSON Schema（`protocols/schemas/`）、ControlHub HTTP API（`smart-hid-controlhub/docs/openapi.yaml`）、License 格式（`smart-hid-cloud/docs/license-format.md`）、在线文档站（`smart-hid-web/docs/`，含快速开始 / 架构 / 协议 / 路线图）。

## 发布与分发

- **源码**：[GitHub 主仓库](https://github.com/LuoYaoSheng/smart-hid-workspace) + [Gitee 镜像](https://gitee.com/luoyaosheng/smart-hid-workspace)（只同步源码、不在镜像上构建）
- **落地页与文档站**：GitHub Pages 托管 —— <https://luoyaosheng.github.io/smart-hid-workspace/>
- **下载包**：[GitHub Releases](https://github.com/LuoYaoSheng/smart-hid-workspace/releases)（ControlHub 双平台二进制 + 固件烧录包，附 SHA256）；Gitee 侧直接存放预编译包

## 反馈与贡献

- 欢迎 Issue：功能需求 / Bug 报告 / 方案讨论
- 落地页"需求与反馈"表单提交后进入分诊看板，被采纳的需求会出现在公开路线图上——你提的，看得见

## 开源协议

[Apache-2.0](LICENSE)

## 相关项目

- [smart-ble](https://github.com/LuoYaoSheng/smart-ble) — BLE Toolkit+ 微信小程序：Smart HID 设备配网 / 诊断入口
- [ESP-IDF](https://github.com/espressif/esp-idf) / [TinyUSB](https://github.com/hathach/tinyusb) / [Go](https://go.dev)
