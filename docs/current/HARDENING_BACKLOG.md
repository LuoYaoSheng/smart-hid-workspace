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
| 4 | BLE Provision 全链路未实现 | ✅ 固件侧完成：NimBLE GATT 服务（分帧 + 加密写 + status notify）、`hub_pairing` HTTP 客户端、provisioning 状态机（含崩溃边界 boot promote）；canonical 协议 `protocols/ble/PROVISIONING_V1.md`。小程序侧（smart-ble 仓）待按 canonical 对齐；**真机未验** |
| 5 | 设备凭据仅单行旋转 | 记录：维持单行旋转 + security_events 留痕设计不变（G3 未改语义；重配对覆盖旧凭据是有意行为） |
| 6 | WS CheckOrigin 全放行 | 记录：LAN 多端访问的有意取舍，维持现状（ticket 化认证的收益不抵复杂度，如需再启 gate） |

### G3 期间新发现（Deferred）

| 发现 | 归属 |
|---|---|
| BLE Just Works 配对无 MITM 抗性（无 IO 能力设备的天花板；设备身份根待 Production Security） | M2-G3 Production Security |
| `runtime_config_clear()` 已具备但无安全用户触发方式（物理按键 / 出厂流程） | M2-G1 硬件验收登记 |
| 小程序（smart-ble）客户端需按 PROVISIONING_V1.md 对齐（分帧/UUID/状态） | smart-ble 仓任务 |
| 分区表从 3×1M 扩到 3×1536K（NimBLE + HTTP 组件使固件超 1M）；设备未烧录过，无迁移成本 | 已落地（无需后续） |
| mock-device 经环回 peer 拿到 127.0.0.1 advertise（合法本机例外），真机场景不受影响 | 记录 |

## M1-G4 CI / Release Engineering

| # | 问题 | 证据 | 说明 |
|---|---|---|---|
| 1 | 版本默认值仍是 scaffold | smart-hid-web/downloads/build-releases.sh:15 `v0.1.0-scaffold` | 忘传 VERSION 即产出 scaffold 版本号 |
| 2 | 固件包非 clean build | build-releases.sh:24-31 | 已有 build/ 就直接复用，可能携带过期产物；固件无版本嵌入 |
| 3 | SHA256SUMS 自包含 | build-releases.sh:22, 33 | 二次运行时 glob `*` 把旧 SUMS 也算进新 SUMS |
| 4 | 无 git 精确版本 / 构建清单 | 全脚本 | 二进制不记录 commit，无法追溯 |
| 5 | 无 CI | .github/ 未建（本地 deploy-landing.yml 未跟踪） | fmt / vet / unit / race / schema / OpenAPI / ESP-IDF build / shellcheck 全缺 |
| 6 | openapi 投影拷贝可能漂移 | build-releases.sh:36-37 | web/api/openapi.yaml 是拷贝，无一致性检查 |

## M2-G1 Hardware Acceptance（独立任务，只列不排期）

ESP32-S3 flash ／ USB 枚举 ／ keyboard ／ mouse ／ hotkey ／ lease ／
release_all ／ Wi-Fi 重连 ／ MQTT 重连 ／ ControlHub 重启 ／ ESP 重启 ／
Windows ／ macOS ／ Linux ／ BIOS ／ 登录界面 ／ soak。

## M2-G2～G4（占位）

OTA / Recovery；Production Security（Secure Boot / Flash Encryption /
固件签名）；Diagnostics / Supportability。
