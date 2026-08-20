# Smart HID

**把 HTTP API 调用变成真实的键鼠输入 —— 本地优先的开源网络 HID 控制系统。**

> **English:** Smart HID turns HTTP API calls into real physical keyboard & mouse input. Your program talks to ControlHub over HTTP; commands travel over MQTT to an ESP32-S3, which emits standard USB HID reports to the target computer. The control path never leaves your LAN — there is no cloud, no account, no license gate.

![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)
![ESP-IDF](https://img.shields.io/badge/ESP--IDF-v5.4.4-C43E00)
![SQLite](https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite)
![USB](https://img.shields.io/badge/USB-HID%20Keyboard%20%2B%20Mouse-3C7EBB)
![open-source](https://img.shields.io/badge/完全开放-无门禁-2EA44F)

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
- **完全本地**：控制链路不出局域网；没有账号、没有云端、没有授权门禁，运行即完全体

## 核心特性

- **真实 HID 输出**：USB Composite HID（键盘 + 鼠标），目标机无需安装任何软件
- **可靠性语义**：命令不是"发出去就完"——去重、防旧命令、TTL、队列满明确报错、lease 超时释放、掉线自动释放全部按键（见下表）
- **动态设备配对**：ControlHub 生成 Pairing QR（`shid://pair?...`），避免预烧凭证；设备靠 BLE 扫描识别
- **BLE 配网**：通过 [smart-ble](https://github.com/LuoYaoSheng/smart-ble)（微信小程序 BLE Toolkit+）完成 Wi-Fi 配网与设备诊断
- **产品化完成度**：系统托盘常驻、本地 Web 控制台、SQLite 持久化、API Key 鉴权与轮换、LAN 模式开关
- **网页模拟键鼠**：ControlHub 内置演示台 `/demo.html`——另一台电脑用浏览器里的可视化键盘/触控板/实体键盘直通遥控本机；多设备芯片条一键切换目标，**广播模式**一次操作群控所有设备
- **WebSocket 实时事件通道**：`/api/v1/realtime` 推送设备状态与命令回执——多端打开演示页即可实时观战

### 可靠性语义

| 机制 | 作用 |
|---|---|
| `request_id` 去重 | 同一命令重复投递只执行一次，幂等 |
| `target_boot_id` | 设备重启后旧会话命令直接拒绝（STALE_DEVICE_SESSION） |
| TTL | 过期命令不再执行，避免迟到动作 |
| 队列满明确报错 | 不静默丢弃，调用方可知、可重试 |
| lease | 长按类操作超时自动释放，防止按键卡死 |
| LWT → release_all | MQTT 异常掉线时设备端自动清空所有已按下键鼠 |

以上语义已由 Go 参考实现（`smart-hid-controlhub/cmd/mock-device` + `smart-hid-controlhub/scripts/test-loop-f2.sh`）28 项端到端测试覆盖。

## 仓库组成

| 目录 | 内容 | 技术栈 |
|---|---|---|
| `smart-hid-firmware/` | ESP32-S3 固件：USB Composite HID + Wi-Fi/MQTT + 命令引擎 | C，ESP-IDF v5.4.4 |
| `smart-hid-controlhub/` | 本地控制程序：HTTP API + 内嵌 MQTT Broker + 系统托盘 + 本地控制台 + 动态配对 | Go，SQLite |
| `smart-hid-web/` | 官网落地页 + 文档站 + 下载中心 | 原生 ES5，零构建静态站 |
| `protocols/` | MQTT Command / ACK / Status JSON Schema 与示例 | JSON Schema |
| `docs/` | 设计资料包（PRD / 架构 / 协议 / 详细设计 / 路线图 / 验收清单） | Markdown |

> 配套独立仓库 [smart-ble](https://github.com/LuoYaoSheng/smart-ble)：BLE Toolkit+ 微信小程序（设备配网 / 诊断），BLE Provisioning 协议与 MQTT Command Schema 的 TS 事实源所在。
>
> 注：`docs/` 资料包为历史设计镜像，保留了设计推演全过程（含已从代码库移除的商业授权层设计记录），按时间线阅读更有味道。

## 快速开始

### 方式一：预编译包（免构建）

发行物直接放在仓库内，附 SHA256 校验；正式 Release 见 [GitHub Releases](https://github.com/LuoYaoSheng/smart-hid-workspace/releases)：

| 包 | 路径 | 说明 |
|---|---|---|
| ControlHub | `smart-hid-web/downloads/controlhub/` | macOS arm64 / Windows amd64 双平台 |
| 固件烧录包 | `smart-hid-web/downloads/firmware/` | ESP32-S3 四个 bin + `flash.sh` 烧录脚本 |

### 方式二：源码构建

```bash
# ControlHub（Go workspace，Go 1.25）
cd smart-hid-controlhub && go build ./cmd/controlhub

# 固件：ESP-IDF v5.4.4（详见 smart-hid-firmware/BUILD.md）
cd smart-hid-firmware && idf.py build

# Web：纯静态零构建，本地预览即开即用
cd smart-hid-web && python3 -m http.server 8090
```

详细上手步骤见[官网快速开始](https://luoyaosheng.github.io/smart-hid-workspace/docs/quick-start.html)。

### 跑测试

```bash
cd smart-hid-controlhub && go test ./...
# 可靠性语义 28 项端到端（Go 参考实现，无需硬件）：
bash smart-hid-controlhub/scripts/test-loop-f2.sh
```

## 配置参考（ControlHub config.yaml）

全部可配、默认全开；不提供 config.yaml 时使用内置默认值（`smart-hid-controlhub/config.example.yaml` 有完整注释）：

| 字段 | 默认 | 说明 |
|---|---|---|
| `http.host` / `http.port` | `127.0.0.1` / `17890` | 本地 HTTP 服务（API + 内置页面） |
| `http.lan_mode` | `false` | 启动即监听 `0.0.0.0`（控制台运行时开关持久化后优先） |
| `http.enable_api` | `true` | `false` = 不注册 `/api/v1`（纯静态模式） |
| `mqtt.port` | `17891` | 嵌入式 MQTT Broker |
| `pairing.enabled` / `pairing.port` | `true` / `17892` | 设备侧配对服务（QR 载荷端口同步生效） |
| `web.console` / `web.demo` | `true` / `true` | 控制台 / 模拟键鼠演示台页面开关 |
| `web.realtime` | `true` | WebSocket 实时事件通道 |

## 当前状态（2026-08）

| 组件 | 状态 |
|---|---|
| 固件 | ✅ F1 控制 + F2 可靠性源码完成，ESP-IDF v5.4.4 编译通过；28/28 可靠性语义端到端验证。⚠️ 尚未在真实 ESP32-S3 硬件上烧录验证 |
| ControlHub | ✅ 产品化完成：托盘常驻 / 本地控制台 / SQLite / 动态配对 / API Key 鉴权 |
| Web | ✅ 落地页 / 文档站 / 下载中心（纯静态零构建） |
| 待办 | 真机烧录验证、固件 Provision Mode、生产安全（Secure Boot / Flash Encryption / 固件签名） |

## 安全设计

- 控制链路不出局域网，系统本身没有云端
- 控制中心 ControlHub：控制 API 默认只听 `127.0.0.1`；LAN API 需用户显式开启 + Bearer API Key；内嵌 MQTT Broker 带认证与 ACL；配对端口仅在配对会话期开放
- 设计层面的威胁模型见 `docs/` 资料包各协议文档

## 文档

| 文档 | 内容 |
|---|---|
| `docs/01_PRODUCT_PRD.md` | 产品 PRD |
| `docs/02_SYSTEM_ARCHITECTURE.md` | 系统架构与仓库拆分 |
| `docs/03_BLE_PROVISIONING_PROTOCOL_V1.1.md` | BLE 配网协议 |
| `docs/04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md` | MQTT 协议与 ControlHub HTTP API |
| `docs/05_CONTROLHUB_DETAIL_DESIGN_V1.0.md` | ControlHub 详细设计 |
| `docs/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md` | 固件详细设计 |
| `docs/08_MINIAPP_HID_MODULE_V1.2.md` | 小程序 HID 模块 |
| `docs/09_LOCAL_DEVELOPMENT_ROADMAP.md` | 开发路线图 |
| `docs/10_ACCEPTANCE_CHECKLIST.md` | 验收清单 |

其他事实源：协议 JSON Schema（`protocols/schemas/`）、ControlHub HTTP API（`smart-hid-controlhub/docs/openapi.yaml`，在线版见官网 API 文档页）。

## 反馈与贡献

- 功能需求 / Bug 报告 / 方案讨论：[GitHub Issues](https://github.com/LuoYaoSheng/smart-hid-workspace/issues) 或 [Gitee Issues](https://gitee.com/luoyaosheng/smart-hid-workspace/issues)
- 提 Issue 前建议先看[开发路线](https://luoyaosheng.github.io/smart-hid-workspace/docs/roadmap.html)，也许你想要的能力已经在计划里

## 关于作者 / 委托开发

作者 **LuoYaoSheng** —— 独立开发者，专注嵌入式（ESP-IDF / BLE）、后端（Go）、全栈交付的完整闭环。

这个项目从协议设计、固件、桌面端到官网全部一个人完成。如果你有类似的需求——IoT 设备联动、桌面工具、自动化方案、从零到一的完整产品——欢迎委托开发：

- **邮箱**：[lys1988_cool@126.com](mailto:lys1988_cool@126.com)
- **微信 / 视频号**：（完善中，欢迎先邮件联系）

## 发布与分发

- **源码**：[GitHub 主仓库](https://github.com/LuoYaoSheng/smart-hid-workspace) + [Gitee 镜像](https://gitee.com/luoyaosheng/smart-hid-workspace)
- **官网与文档站**：<https://luoyaosheng.github.io/smart-hid-workspace/>
- **下载包**：[GitHub Releases](https://github.com/LuoYaoSheng/smart-hid-workspace/releases)（ControlHub 双平台二进制 + 固件烧录包，附 SHA256）；仓库内 `smart-hid-web/downloads/` 同步存放

## 开源协议

[Apache-2.0](LICENSE)

## 相关项目

- [smart-ble](https://github.com/LuoYaoSheng/smart-ble) — BLE Toolkit+ 微信小程序：Smart HID 设备配网 / 诊断入口
- [ESP-IDF](https://github.com/espressif/esp-idf) / [TinyUSB](https://github.com/hathach/tinyusb) / [Go](https://go.dev)
