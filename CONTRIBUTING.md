# 贡献指南

个人维护的开源项目，欢迎 Issue 与 PR。规模小，不套企业流程。

## 开始之前（重要）

1. 先读根 [README.md](README.md) 的「事实源与开发治理」区块
2. 当前事实以 [docs/current/](docs/current/) 为准；`docs/archive/` 是历史设计资料（SUPERSEDED），**不要**按它实现功能
3. 禁止重新实现已移除的商业能力（Trial / License / Cloud 等，见 [DEVELOPMENT_RULES §2](docs/current/DEVELOPMENT_RULES.md)）
4. 路线按 Gate 推进（[ROADMAP](docs/current/ROADMAP.md)）；范围外的问题请记入 Issue 而不是顺手改

## 提 Issue

- Bug：写清复现步骤、组件（ControlHub / 固件 / Web）、版本
- 功能：先看 [HARDENING_BACKLOG](docs/current/HARDENING_BACKLOG.md) 和路线，避免重复

## 提 PR

1. 改协议时四件同步：Schema / OpenAPI → 实现 → 测试 → 文档（Contract First）
2. ControlHub 改动：`cd smart-hid-controlhub && go vet ./... && go test ./...` 全绿
3. 文档治理改动：`bash scripts/check-governance.sh` 通过
4. 提交说明写清楚为什么；一个 PR 一件事
5. 状态描述诚实：mock 通过 ≠ 硬件验证（Honest Status）

## 硬件相关

真机烧写 / 硬件验收是独立任务（M2-G1）。PR 不要声称"hardware verified"，
除非附真实设备测试记录。
