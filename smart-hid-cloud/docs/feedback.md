# 需求与反馈（Feedback）设计 — FB-1

> 状态：**已实现（FB-1a 后端 / FB-1b 前端 / FB-1c 路线图可见）**
> 关联：PRD §F（运营支撑）；落地页"需求与反馈"区块 / admin"反馈"视图 / docs 路线图"来自社区的需求"。

## 1. 目标与边界

给产品补上"用户声音"通道，最小可信闭环：

```
用户（匿名）→ 落地页提交 → Cloud 落库(status=new)
                             ↓ admin 后台分诊
                    planned / shipped → 公开路线图可见（"我提的被采纳了"）
                    rejected（写原因）→ 不对外
```

**不做**（V1 边界）：投票/点赞（等真实用户量）、邮件通知、Portal 内嵌表单、Turnstile 验证码。

## 2. 数据模型

迁移 `0004_feedback.up.sql`：

| 字段 | 说明 |
|---|---|
| `feedback_id` | `fb_<22hex>`，与 lic_/acc_/ord_ 同族 |
| `user_id` | **预留恒空**——匿名提交；不设 FK，与账号体系解耦 |
| `category` | `feature \| bug \| other`（白名单） |
| `title` / `body` | ≤80 / 5–2000（rune 计数，服务端权威） |
| `contact` | 可选联系方式 ≤120 |
| `client_ip` / `user_agent` | 审计字段，**仅 admin 可见**，UA 截断 250 |
| `status` | `new \| planned \| shipped \| rejected` |
| `admin_note` | admin 备注 ≤500；planned/shipped 状态下**对外可见**（路线图展示） |

状态机：admin **自由流转**（无转移限制，含重开 rejected→new）。简单优先——admin 是可信操作者，流程约束价值低于灵活性。

## 3. 端点契约

| 端点 | 方法 | 鉴权 | 说明 |
|---|---|---|---|
| `/api/v1/feedback` | POST | 公开 | `{category, title, body, contact?, website}`；201 `{feedback_id, status:"new"}` |
| `/api/v1/feedback/roadmap` | GET | 公开 | `{items, total}`，仅 planned/shipped，按 updated_at 倒序，≤100 |
| `/api/v1/admin/feedback?status=` | GET | admin | `{items, total}` 完整字段（含审计），created_at 倒序，≤500 |
| `/api/v1/admin/feedback/{id}/status` | POST | admin | `{status, admin_note?}` → `{feedback_id, status}` |

## 4. 反垃圾与威胁模型

匿名公开写端点必然被 bot 盯上，三板斧（成本递增，前两个已实装）：

1. **Honeypot**（已实装）：表单隐藏域 `website`，正常用户永远为空。命中 → 返与真实成功**完全同形**的 201（假 `fb_` id）但不落库，不暴露检测。低成本挡掉绝大多数无脑脚本。
2. **每 IP 限频**（已实装）：进程内滑动窗口 5 次/小时（`internal/api/ratelimit.go`）。超限 429 `rate_limited`。方法守卫后立即检查——**垃圾请求同样消耗配额**。
3. **长度硬上限**（已实装）：title/body/contact/admin_note 全部服务端截断或拒绝，防存储滥用。

**XFF 信任模型**（重要）：`clientIP()` 优先取 `X-Forwarded-For` 首值，否则 `RemoteAddr`。

- 直连 / 无反代：RemoteAddr 不可伪造，安全。
- 反代后：攻击者可自带伪造 XFF 头换 IP 绕过限频。**生产部署必须让反代覆盖该头**：
  ```nginx
  proxy_set_header X-Forwarded-For $remote_addr;   # 覆盖，而非追加
  ```
- Phase 7（生产安全）再上可信代理白名单配置。

**残余风险**：真人不带浏览器手动刷 5 条/小时——接受（admin 可批量 rejected，垃圾不进公开视图）。

**XSS**：title/body/contact 是本产品**首个公开写入、多端渲染**的用户输入。前端所有渲染点（admin 表格/详情、路线图列表）必须 HTML 转义；服务端长度上限做纵深兜底。

## 5. 限频器实现说明

- 进程内 `map[string][]int64` + mutex 滑动窗口；访问时惰性清理过期时间戳。
- 单实例部署（当前形态）够用；多实例/水平扩展时需换共享存储（Redis 等）——届时再议。
- 重启清零——可接受（攻击者也损失配额）。

## 6. 测试覆盖

`internal/api/feedback_test.go`：HappyPath / Honeypot（假成功+不落库）/ Validation（类目白名单+长度 rune 计数）/ RateLimit（第 6 次 429）/ AdminFlow（过滤→planned 带备注→roadmap 可见→rejected 不可见→非法 status 400→未知 id 404）/ AdminAuth（401/403）/ RoadmapEmpty（items=[] 非 null）。

## 7. 未来演进（不在 V1）

- 投票 / 点赞（需 IP 或账号去重策略，等用户量）
- 注册用户提交自动关联 user_id（列已预留）
- admin 回复邮件通知（需接邮件服务）
- Turnstile / hCaptcha（真实流量上来后评估）
- Cloud openapi.yaml（Cloud 全 API 文档化的独立任务）
