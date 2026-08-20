---
status: CURRENT
authority: canonical
---

# DEVELOPMENT_RULES — 开发规则（人类贡献者与 AI Agent 通用）

## 1. Fact Source Rule（事实源顺序）

修改代码或文档前，按以下顺序确认事实，**不得以 archive 文档作为当前实现依据**：

```text
README.md
→ docs/current/
→ protocols/ 与 openapi.yaml
→ 实现（main 分支代码）
```

## 2. No Resurrection Rule（禁止复活）

禁止因历史文档存在而重新实现以下已移除能力（DO NOT IMPLEMENT）：Trial / License / Cloud / Commercial / Order / Payment / Entitlement / Usage Gate。除非用户未来明确重新立项。

## 3. No Feature Creep Rule（不越 Gate）

Gate 内只处理 Gate 范围。发现其他问题：记录 → HARDENING_BACKLOG（Deferred），
不能顺手实现。

## 4. Contract First Rule（契约先行）

涉及协议时，四份必须同步改，不能只改其中一份：

```text
Schema / OpenAPI → 实现 → 测试 → 文档
```

## 5. Honest Status Rule（诚实状态）

禁止把 mock tested ／ compiled ／ unit tested 写成 hardware verified ／
production ready。状态值只用 CURRENT_STATE 定义的五种明确值。

## 6. Hardware Isolation Rule（硬件隔离）

以下工作独立成任务（M2-G1），没有真实设备结果不得标 PASS：

```text
flash / 烧写 / USB 真机枚举 / BIOS / Windows 登录界面 / 真实 HID /
板级 Wi-Fi / ESP32 断电 / 硬件 soak
```

## 附：给 AI Agent 的入口指令

新会话进入本仓库开发时，只需要被告知：**先阅读根 README.md**。README 会把
你引导到本目录。若你发现任何文档与代码冲突：以代码为准，改文档，并在
提交说明里指出冲突来源。
