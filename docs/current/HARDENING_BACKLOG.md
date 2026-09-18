---
status: CURRENT
authority: canonical
---

# HARDENING_BACKLOG — 加固待办

> M1-G1 审计（2026-08-20，基线 5c2e5dc）逐项核实过的已知问题。
> 本文件只记录与排期，**不在 G1 修**；各项归属的 Gate 开工前再细化方案。

## M1-G2 Core Correctness ✅（2026-08-20 完成）

原登记 9 项全部落地（commit 见 git log M1-G2a~d）：

| # | 原问题 | 处置 |
|---|---|---|
| 1 | request_id 无服务端去重 | ✅ 幂等注册表 + fingerprint（sha256(device/type/action/canonical payload)，键序无关） |
| 2 | 并发同 request_id 覆盖 waiter | ✅ 单一 owner execution；同指纹 join 等同一结果；异指纹 409 request_id_conflict |
| 3 | DB 写错误被丢弃 | ✅ INSERT 错误分类：UNIQUE→重放/冲突判定，其余→500 且不 publish；终态 ACK 落库失败记 Error |
| 4 | 配对 token 消费 TOCTOU | ✅ 单事务 CAS（UPDATE...WHERE status='pending' AND expires_at>=now，RowsAffected==1）|
| 5 | 凭据签发与 session 跨事务 | ✅ issueDeviceCredentialsTx 与 session 状态同事务；失败整体回滚，无半状态 |
| 6 | RealtimeHub 锁外读 len | ✅ RWMutex + 快照广播；订阅 channel 永不 close（无 send-on-closed 路径） |
| 7 | ACK 信任边界 | ✅ 三方绑定（topic device == ack.device_id == 在途 execution 期望设备）+ protocol/status 合法性；非法 ACK 记 warning 丢弃 |
| 8 | payload 深度校验缺失 | ✅ ValidatePayload：键名（镜像固件 keymap，大小写不敏感）/hotkey≤8/lease 范围/dx,dy≤4096/wheel 硬限[-127,127]（固件 int8 强转）/count 1-10/button 枚举 |
| 9 | `go test -race` 未纳入常态 | ✅ 全仓 -race 绿；并发包 -count 高倍通过；50 并发配对消费 / 20 并发同 id join 等压力测试在库 |

request_id 语义（现行）：**Idempotency Key**——首次执行；并发同命令 join（恰好 publish 一次）；同 id 异命令 409；终态后重放直接返回既有结果（不再执行 HID）；非终态重放按 202 语义（不重发）。迁移：commands 表新增 fingerprint 列（0004）。

### G2 期间新发现（Deferred，不属 G2 修复范围）

| 发现 | 归属 |
|---|---|
| 固件 `key[8]`/`keys[8][8]` 缓冲截断：≥8 字符键名（BACKSPACE/CAPSLOCK/PAGEDOWN）被截断后设备端 lookup 失败→rejected（keymap 有名无实） | 固件任务（M2 或独立固件修复） |
| mock-device 不校验键名（与固件 keymap 分叉，历史上 LEFTSHIFT 等伪键名能通过 e2e） | mock 改进（低优先） |
| WS realtime 认证用 query key + CheckOrigin 放行（LAN 有意取舍）→ ticket 化认证 | M1-G3 评估 |
| replay 返回的 ACK 无 boot_id（commands 表未存设备侧 boot_id） | 记录，影响极小 |

## M1-G3 Network / Provisioning ✅（2026-08-20 完成）

原登记 6 项处置（commit 见 git log M1-G3a~e）：

| # | 原问题 | 处置 |
|---|---|---|
| 1 | mqtt.host 一字段三用 | ✅ 拆为 `bind_host`（默认 0.0.0.0）/ `advertise_host`（resolver 解析）/ 内部连接地址（推导）；legacy `mqtt.host` 自动迁移 + deprecated 警告；内部凭据留空 = 每启动随机（`change-me-in-production` 从代码库消失） |
| 2 | LAN IP 选择单薄 | ✅ `internal/netaddr` Resolver：显式配置 → 请求 LocalAddr → peer UDP 出口 → 唯一可用 LAN IPv4（过滤 docker/veth/环回/链路本地）→ 多候选明确失败并列出候选；环回 peer（本机 mock）唯一例外；pairing 先解析后消费 token（失败 503 + token 保持 pending） |
| 3 | 固件无运行时网络配置 | ✅ `runtime_config` 组件（NVS rt_active/rt_pending、schema_version 守卫、generation、pending-set_creds-then-promote、factory clear）；Kconfig 降级为 `SMART_HID_DEV_STATIC_CONFIG`（默认 OFF）显式 DEV fallback |
| 4 | BLE Provision 全链路未实现 | ✅ 固件侧完成：NimBLE GATT 服务（分帧 + INPUT 明文 write + status notify）、`hub_pairing` HTTP 客户端、provisioning 状态机（含崩溃边界 boot promote）；canonical 协议 `protocols/ble/PROVISIONING_V1.md`。V1 简化不发起 SMP/系统配对；小程序侧（smart-ble 仓）已完成 V1 镜像锁、Profile 接入与三阶段配网流程；**微信工具与真机全链路未验** |
| 5 | 设备凭据仅单行旋转 | 记录：维持单行旋转 + security_events 留痕设计不变（G3 未改语义；重配对覆盖旧凭据是有意行为） |
| 6 | WS CheckOrigin 全放行 | 记录：LAN 多端访问的有意取舍，维持现状（ticket 化认证的收益不抵复杂度，如需再启 gate） |

### G3 期间新发现（Deferred）

| 发现 | 归属 |
|---|---|
| BLE Provision V1 明文直连可被近场被动嗅探；没有设备身份根与传输机密性 | M2-G3 Production Security（协议 V2 或生产安全方案） |
| `runtime_config_clear()` 已具备但无安全用户触发方式（物理按键 / 出厂流程） | M2-G1 硬件验收登记 |
| ~~小程序客户端真机联调~~ → 2026-09-05 已由 F-AND（Flutter 安卓真机）E14 轮全链闭环；U-AND / 桌面双线 / F-MAC 深链仍未验 | smart-ble 仓 windows-mobile 主矩阵（WIN-007 等） |
| 配网真机闭环证据挂 smart-ble 旧基线 3638f66，当前 HEAD 无同等证据 | M2-G1 硬件验收（重刷重跑） |
| 分区表从 3×1M 扩到 3×1536K（NimBLE + HTTP 组件使固件超 1M）；设备未烧录过，无迁移成本 | 已落地（无需后续） |
| mock-device 经环回 peer 拿到 127.0.0.1 advertise（合法本机例外），真机场景不受影响 | 记录 |

## M1-G4 CI / Release Engineering ✅（2026-08-20 完成）

原登记 6 项处置（commit 见 git log M1-G4a~e）：

| # | 原问题 | 处置 |
|---|---|---|
| 1 | 版本默认值 scaffold | ✅ 根 `VERSION` 文件唯一事实源（x.y.z）；build-releases 读文件且形态校验；scaffold 字样全树清除（治理守卫拦截） |
| 2 | 固件包非 clean build / 无版本嵌入 | ✅ `scripts/build-firmware.sh`（fullclean + set-target + build）；固件版本 = PROJECT_VER ← VERSION 文件（esp_app_desc 运行时可读，device_identity 硬编码删除） |
| 3 | SHA256SUMS 自包含 | ✅ 显式文件清单 + `-c` 自校验；SUMS 改名 controlhub-SHA256SUMS / firmware-SHA256SUMS（资产名唯一） |
| 4 | 无版本/构建清单 | ✅ manifest.json（version/commit/build_time/dirty + 每 artifact sha256/size/type + 固件构建元数据）+ README_RELEASE.md + ControlHub `-version` 自证 |
| 5 | 无 CI | ✅ 三 workflow：ci.yml（go fmt/vet/test/race + 协议/OpenAPI 门 + 治理 + shellcheck + 固件宿主单测与双配置构建）、release.yml（tag 驱动，tag↔VERSION 一致校验）、docs.yml（Pages） |
| 6 | openapi 投影漂移 | ✅ build-releases 投影后 diff 防漂移 + validate-protocols.py 常态校验投影一致 |

附加发现并修复：flash.sh / 固件 README 烧录偏移停留在 G3 旧分区表（0xd000/0x10000 → 0x11000/0x20000）；旧 per-dir `SHA256SUMS` 命名与 Release 资产唯一名冲突。

## M2-G1 Hardware Acceptance（独立任务，只列不排期）

ESP32-S3 flash ／ USB 枚举 ／ keyboard ／ mouse ／ hotkey ／ lease ／
release_all ／ Wi-Fi 重连 ／ MQTT 重连 ／ ControlHub 重启 ／ ESP 重启 ／
Windows ／ macOS ／ Linux ／ BIOS ／ 登录界面 ／ soak。

## M2-G2～G4（占位）

OTA / Recovery；Production Security（Secure Boot / Flash Encryption /
固件签名）；Diagnostics / Supportability。

## 电源管理设计（2026-08-20 真机联调后登记，随硬件向 Gate 排期）

> 背景：WIFI_PS_NONE 已定为默认（真机验证 modem sleep 破坏实时性，见固件 README
> bug ⑤）。本设备 USB 总线供电、无电池，深度省电本身收益极小；但以下两个
> "场景化休眠/唤醒"缺口是真实的产品能力，值得随 M2 一并设计。

| # | 缺口 | 证据 | 说明 |
|---|---|---|---|
| 1 | USB suspend/resume 驱动的跟随休眠未实现 | 固件未启用 `TINYUSB_SUSPEND_CALLBACK` / `TINYUSB_RESUME_CALLBACK`（Kconfig 默认 n，esp_tinyusb 提供） | 目标电脑睡眠 → USB 总线挂起 → 设备可转入 modem sleep 等低功耗态；宿主唤醒 → USB resume 即天然唤醒源（零额外硬件）。这是唯一不破坏"网络随时可达"语义的场景化休眠：宿主睡了 HID 无处生效，省电不损失功能 |
| 2 | HID Remote Wakeup 声明了但未实现 | hid_engine.c 配置描述符带 `TUSB_DESC_CONFIG_ATT_REMOTE_WAKEUP`，但固件从未调用 `tud_remote_wakeup()` | 描述符向宿主宣称"设备可唤醒主机"，实际不支持——网络唤醒睡眠中电脑（按任意键开机级体验）是自然产品能力。需实现 resume 流程，或先摘掉属性位避免虚假声明 |

## 体验与工程提案（2026-08-21 登记，待评审排期）

| # | 提案 | 背景 | 说明 |
|---|---|---|---|
| 1 | ✅ 本机回环免鉴权（2026-08-21 落地） | 单人测试时"控制台还要找 Key"是纯摩擦：能碰到 127.0.0.1 的本机进程本就可读 key 文件，回环鉴权无安全价值 | authMiddleware：回环请求带 `X-ControlHub-Local: 1`（CSRF 边界，浏览器恶意网页跨域发不出自定义头）即免 Key；无头回环退回 Bearer（curl/脚本向后兼容）；非回环维持 Bearer。前端三页（console/demo/api-test）已带头；控制台 Key 输入框仅在 401 时显示。实测五路径：回环+头✓ / 回环+Bearer✓ / 回环裸 401✓ / LAN无Key 401✓ / LAN+Key 200✓ |
| 2 | 配网 V2：设备直连 + 主人批准 | 现行 QR→手机→BLE 转交 token 流程（PROVISIONING_V1）让手机承担"凭证搬运工"，单人测试时主人/配网者角色一人分饰、体验混乱 | 改为：BLE 只递 Wi-Fi+hub 地址 → 设备直连 ControlHub 自报 device_id+首次上电自生成密钥哈希 → 控制台弹"发现新设备，是否信任"→ 主人一键批准后签发 MQTT 凭据。信任决策回到主人手里，QR/token 环节消失。需改配对协议（V2）+ 设备状态机 + 控制台审批 UI，与小程序侧对齐后再动 |
| 3 | ✅ internal/config 两个 Windows 失败测试（2026-09-18 修复） | `TestLoad_ValidOverride` / `TestLoad_CreatesAndAbsDataDir` 在 Windows 检出下必失败；原登记猜因为 CRLF，实际根因是 **`filepath.Join` 的反斜杠路径进 YAML 双引号串被当转义前缀**（`\U` 要求 8 位十六进制续接 → 解析报 "did not find expected hexdecimal number"） | 夹具路径经 `filepath.ToSlash` 转正斜杠进 YAML（Go 路径 API 在 Windows 接受正斜杠）；真因已回写测试注释防再误记 |
| 4 | ✅ 发布链可移植性三修（2026-09-18，随 v1.2.0） | build-releases.sh 由 macOS 单端编写，Windows/Linux 必炸三处：① VERSION 检出 CRLF 未剥 `\r` 污染 ldflags；② 版本自证步骤直接执行 darwin 二进制；③ `sed -i ''` 是 BSD 语法 | ①②脚本内 `tr -d ' \r\n'`（build-firmware.sh 同修）；②自证改为优先执行本机可跑产物、Linux CI 降级 `go version -m` 软校验；③改临时文件替换 |
| 5 | config.api_key 是死配置 | `Config.APIKey` 加载后无任何消费者（实际用 apikey.Store 首启生成 + initial-api-key.txt 交付）；openapi 旧文案曾据此失实描述 | 低优先：删除该字段或接回显式配置语义，随下次 config 面改动一并处理 |

## v1.2.0 收口版本（2026-09-18）

一次性交付（不再小步零发）：观测性（日志双路落盘 + panic 防护 + 退出码 0/1/2）、
config 测试 Windows 修复（真因更正为反斜杠 YAML 转义）、发布链可移植性三修
（CRLF / 宿主自证 / sed -i）、openapi 版本与回环免鉴权对齐、官网三处脱节
（版本角标 / 两处 SHA256SUMS 404）、文档事实对齐（led_manager 已验证、
配网 E14 闭环、发布链从未跑过 tag 的事实）。**v1.2.0 是 release.yml
tag 驱动发布的首次真实执行。**

| 遗留 | 处置 |
|---|---|
| ControlHub Windows exe 曾一次进程退出（2026-08-20 联调期），当时无日志落盘无线索 | 观测已补齐；待复现归因，不主动追查 |
| 官网 downloads 资产与 GitHub Release 的同步依赖 tag 发布流水线 | v1.2.0 起由 release.yml 自动产出，不再手工拷贝 |
