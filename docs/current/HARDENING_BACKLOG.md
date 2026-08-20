---
status: CURRENT
authority: canonical
---

# HARDENING_BACKLOG — 加固待办

> M1-G1 审计（2026-08-20，基线 5c2e5dc）逐项核实过的已知问题。
> 本文件只记录与排期，**不在 G1 修**；各项归属的 Gate 开工前再细化方案。

## M1-G2 Core Correctness

| # | 问题 | 证据（file:line） | 说明 |
|---|---|---|---|
| 1 | request_id 无服务端去重，客户端可任意传 | command/validator.go:30（仅查空与长度） | 同 id 并发 = 重复投递风险 |
| 2 | 并发同 request_id 覆盖 pending chan | command/engine.go:120-129 | 后到者覆盖 map 项；先到者 defer 提前 delete 对方项 |
| 3 | DB 写错误被丢弃 `_, _ =` | command/engine.go:81, 114 | INSERT 失败（如 UNIQUE 冲突）静默吞掉 |
| 4 | 配对 token 消费非原子（TOCTOU） | pairing/manager.go:144-169 | 读-查-写三步无事务；UPDATE 无 `WHERE status='pending'` 守卫，同 token 并发可双发凭据 |
| 5 | 配对凭据签发与 session 标记跨事务 | pairing/manager.go:156-169 | IssueDeviceCredentials 独立事务成功后，标记 session 失败会出现凭据已发但 session 仍 pending |
| 6 | RealtimeHub 锁外读 len(h.subs) | api/realtime.go:47 | Broadcast 早退路径与 subscribe/unsubscribe 并发时是数据竞争（-race 候选） |
| 7 | ACK 字段不做校验（信任边界） | command/engine.go:59-90 | execution_ms / device_id 与 topic 不比对；靠 broker ACL 兜底 |
| 8 | payload 深度校验缺失 | command/validator.go:22-23 注释自认 | type/action 合法即放行，字段级校验全在设备侧 |
| 9 | `go test -race ./...` 未纳入常态 | — | G1 只跑一次记录，修复与固化在 G2 |

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
