# smart-hid-cloud

Smart HID 云端。账号 / 套餐 / 订单 / 支付 / License 签发。

## 角色定位

```text
Smart HID Web ──HTTPS──▶ Smart HID Cloud
ControlHub    ──激活/刷新──▶ Smart HID Cloud
```

> **Cloud 不在实时 HID 控制链路。** 实时控制本地优先，无云环境已有有效 License 可继续工作。

## 当前状态

✅ **CL-1 ~ CL-2c 已实现**（2026-08-12）。Phase 6 第一批：完整 License 签发闭环。

| 步骤 | 内容 |
|---|---|
| CL-1 | `pkg/license`（Ed25519 + 载荷 + 签发 + 验签）+ Go workspace |
| CL-2a | Cloud skeleton（config + storage migration + JWT + main）|
| CL-2b | 6 业务域 store + 11 endpoints + License 签发 e2e 全通 |
| CL-2c | e2e 测试套件（handlers_test）+ license-format spec 完整化 |

后续（Phase 6 后续批次，独立做）：
- CL-3 ControlHub License 集成（验签 + 激活 + 导入 + Entitlement 升级）
- CL-4 smart-hid-web 用户门户重建

## 模块

```
smart-hid-cloud/
├── pkg/license/             # 共享 License 包（Cloud 签发，ControlHub 验签）
├── cmd/cloud/               # HTTP server 入口
├── cmd/gen-keys/            # Ed25519 keypair 生成工具
├── internal/
│   ├── config/              # YAML 配置加载
│   ├── logging/             # slog JSON 日志器
│   ├── storage/             # SQLite + 版本化 migration（7 张表）
│   ├── auth/                # HS256 JWT 自实现
│   ├── api/                 # HTTP handlers + 中间件
│   └── store/               # 业务域 CRUD（User/Plan/Device/Order/License）
├── docs/
│   └── license-format.md    # License 格式事实源（完整 spec）
├── keys/                    # Ed25519 私钥（git ignored）
└── scripts/gen-keys.sh      # keypair 生成脚本
```

## 快速开始

```bash
cd smart-hid-cloud

# 1. 生成 Ed25519 keypair（仅首次；已存在不覆盖）
./scripts/gen-keys.sh
# → keys/private.key (git ignored)
# → keys/public.hex  (复制 hex 到 ControlHub internal/license/publickey.go，CL-3a 用)

# 2. 启动 Cloud
go run ./cmd/cloud -config config.example.yaml
# 默认监听 127.0.0.1:17880
# 自动跑 migration + seed 默认套餐（plan_basic_monthly + plan_basic_yearly）

# 3. 端到端验证（用 curl 或 ControlHub API Playground）
EMAIL="me@example.com"
# 注册
curl -X POST http://127.0.0.1:17880/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"mypassword\"}"
# → {user_id, token}

# 列套餐
curl http://127.0.0.1:17880/api/v1/plans

# 注册设备 / 创建订单 / Mock 支付 / 激活 / 下载 .license
# （详见 docs/license-format.md 第 7-9 节）
```

## API 端点

公开（无需鉴权）：
- `POST /api/v1/auth/register` 注册 → `{user_id, token}`
- `POST /api/v1/auth/login` 登录 → `{user_id, token}`
- `GET /api/v1/health` 健康检查
- `GET /api/v1/plans` 套餐列表

JWT 保护：
- `GET /api/v1/users/me` 当前用户信息
- `GET/POST /api/v1/devices` 列出 / 注册设备
- `GET/POST /api/v1/orders` 列出 / 创建订单
- `POST /api/v1/orders/{id}/pay-callback` Mock 支付回调（V1 无真实网关验证）
- `GET /api/v1/licenses` License 列表
- `GET /api/v1/licenses/{id}` License 详情
- `POST /api/v1/licenses/{id}/activate` 激活（绑设备 + Ed25519 签发）
- `GET /api/v1/licenses/{id}/download` 下载 `.license` 文件

## License

- 云端 Ed25519 签发，ControlHub 本地验签（只内置 Public Key）
- Private Key 不下发
- 主绑定对象：ESP32 Device ID
- 支持在线激活与离线导入

完整 spec：`./docs/license-format.md`

## 购买与激活流程

```text
注册账号 → 选套餐 → 创建订单 →（V1 mock 支付）→ 创建 UNUSED License
→ 注册 ESP32 Device → 激活 License（绑 device + 签发）→ 下载 .license
→ ControlHub 导入 → 本地验签 → 解锁 Entitlement
```

购买与绑定分离（购买得 UNUSED，激活时才绑定 device）。

## 测试

```bash
go test ./... -count=1
# pkg/license: 14 用例（Ed25519 签发/验签全覆盖）
# internal/auth: 5 用例（JWT HS256）
# internal/api: e2e 完整闭环 + 鉴权 + 跨用户隔离
```

## 相关

- License 格式事实源：`./docs/license-format.md`
- 验收清单：`../docs/10_ACCEPTANCE_CHECKLIST.md` §E
- ControlHub 验签集成：`../smart-hid-controlhub/internal/license/`（CL-3a）
- 顶层 Go workspace：`../go.work`
