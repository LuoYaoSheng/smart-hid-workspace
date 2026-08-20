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
M1-G1 ✅ DONE（2026-08-20，本文档所在轮次）
M1-G2 ⏳ NEXT —— 等待用户明确说「继续 M1-G2」后才能开始
```

## M1 — Product Foundation Hardening（产品化加固）

### G1 Truth / Governance Baseline ✅

唯一事实入口（根 README → docs/current/）、历史资料降级（docs/archive/）、
开发规则、验收清单重建、hardening backlog、治理守卫脚本。

### G2 Core Correctness（下一个 Gate，未开始）

request_id 并发去重、waiter 合并、DB 错误处理、配对 token 原子消费、
RealtimeHub 并发修正、ACK 校验、payload 深度校验、`go test -race`。
明细见 [HARDENING_BACKLOG](HARDENING_BACKLOG.md)。

### G3 Network / Provisioning Foundation（未开始）

mqtt bind/advertise 拆分、LAN IP 选择、BLE Provision、NVS 运行时配置、
重置 / 重配网、凭据持久化。明细见 HARDENING_BACKLOG。

### G4 CI / Release Engineering（未开始）

GitHub Actions（fmt / vet / unit / race / schema / OpenAPI / ESP-IDF clean
build / shellcheck）、干净 Release 流水线、精确版本嵌入、构建清单、SHA256、
Release 自动发布。

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
