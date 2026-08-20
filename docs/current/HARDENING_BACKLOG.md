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

## M1-G3 Network / Provisioning

| # | 问题 | 证据 | 说明 |
|---|---|---|---|
| 1 | mqtt.host 一字段三用 | app.go:111（broker bind）、115（内部 client 连接）、120（广播给设备） | 同一值同时承担 bind / internal connect / advertised host；默认 127.0.0.1 时设备根本连不上 broker，配 0.0.0.0 时广播地址错误。需拆 `bind_host` / `advertise_host` |
| 2 | LAN IP 选择单薄 | pairing/qr.go（GuessLANIP 单值） | 多网卡 / Docker 网卡场景选错无提示 |
| 3 | 固件无运行时网络配置 | wifi_manager.c:54-55、mqtt_manager.c:123-127 | Kconfig 编译期固定 Wi-Fi/MQTT；需 NVS 运行时配置 + 重配网入口 |
| 4 | BLE Provision 全链路未实现 | 固件无 BLE 组件；协议文档在 docs/archive/03 | 小程序侧（smart-ble 仓）+ 固件 Provision Mode 双端都要做 |
| 5 | 设备凭据仅单行旋转 | pairing/manager.go:210-221 | device_credentials PK=device_id，重配对即覆盖旧凭据（有意设计，确认是否需要历史留痕） |
| 6 | WS CheckOrigin 全放行 | api/realtime.go:31 | LAN 多端访问的有意取舍；G3 评估是否收紧 |

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
