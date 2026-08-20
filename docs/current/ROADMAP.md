---
status: CURRENT
authority: canonical
---

# ROADMAP — 当前路线

> 旧的 Phase 0～7 模型（docs/archive/09）已废弃：其中 Phase 5 Trial、Phase 6
> Commercial、Phase 7 的一部分属于已移除的商业化路线，不会再做。
> 当前路线以 Milestone / Gate 推进，一次只做一个 Gate。

## 当前位置

```text
M1-G1 ✅ DONE（2026-08-20，治理基线）
M1-G2 ✅ DONE（2026-08-20，核心正确性：幂等/原子配对/并发安全/ACK 边界/深度校验）
M1-G3 ✅ DONE（2026-08-20，网络/配网：bind/advertise 拆分 + NVS 运行时配置 + BLE Provision）
M1-G4 ✅ DONE（2026-08-20，CI/Release：VERSION 唯一源 + 质量门 workflows + 干净发布链）
```

## M1 — Product Foundation Hardening（产品化加固）

### G1 Truth / Governance Baseline ✅

唯一事实入口（根 README → docs/current/）、历史资料降级（docs/archive/）、
开发规则、验收清单重建、hardening backlog、治理守卫脚本。

### G2 Core Correctness ✅（2026-08-20 完成）

request_id 并发去重与服务端幂等、waiter join、DB 错误处理、配对 token 原子消费、
RealtimeHub 并发修正、ACK 三方绑定校验、payload 深度校验、`go test -race` 固化。
明细见 [HARDENING_BACKLOG](HARDENING_BACKLOG.md)。

### G3 Network / Provisioning Foundation ✅（2026-08-20 完成）

mqtt bind/advertise 拆分（legacy 兼容迁移）、LAN IP 请求级解析（多网卡明确失败）、
内部 MQTT 凭据每启动随机化、固件 NVS 运行时配置（active/pending + 崩溃恢复）、
BLE Provision 全链路源码（NimBLE + canonical 协议）、配网状态机与 RECOVERY。
明细见 [HARDENING_BACKLOG](HARDENING_BACKLOG.md)。固件侧**未经真机验证**。

### G4 CI / Release Engineering ✅（2026-08-20 完成）

唯一版本源（根 VERSION → ControlHub ldflags / 固件 PROJECT_VER）、CI 质量门
（go fmt/vet/test/race + 协议/OpenAPI 门 + 治理 + shellcheck + 固件双配置
构建）、tag 驱动 Release（dirty 拒绝、fullclean 构建、显式 SHA256、
manifest.json provenance、README_RELEASE 诚实状态）。明细见
[HARDENING_BACKLOG](HARDENING_BACKLOG.md)。

## M2 — Hardware & Delivery（硬件与交付）

### G1 Hardware Acceptance

> **独立任务，不在 M1 内执行。** 真机烧写、USB 枚举、三操作系统、BIOS /
> 登录界面、断连 soak 等全部硬件验证。没有真实设备结果不得标 PASS。

### G2 OTA / Recovery
### G3 Production Security（Secure Boot / Flash Encryption / 固件签名）
### G4 Diagnostics / Supportability

## Gate 纪律

- 一次只做当前 Gate 范围内的事；发现别的问题 → 记入 HARDENING_BACKLOG，不顺手修
- 每个 Gate 完成后停下汇报，等用户明确指示再进下一个
- 涉及硬件的工作（烧写 / 真机验证）永远走 M2-G1 独立任务
