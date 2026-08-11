# smart-hid-controlhub

Windows 本地控制程序。接收第三方 HTTP API 指令，通过 MQTT 下发给 ESP32-S3，并管理 Trial / License / 设备配对。

## 角色定位

```text
第三方程序 ──HTTP──▶ ControlHub ──MQTT──▶ ESP32-S3 ──USB HID──▶ 目标电脑
```

实时控制不依赖互联网。License 云端签发、本地离线验签（Ed25519）。

## 技术栈（推荐）

- Go
- `net/http`（HTTP API + Local Web UI）
- Embedded MQTT Broker（端口 17891）
- SQLite（持久化）
- `go:embed`（嵌入 Web 资源）
- Windows tray
- Windows DPAPI（敏感数据保护）
- `crypto/ed25519`（License 验签，只内置 Public Key）

## 建议模块结构

来自资料包 `starter/controlhub`：

```text
cmd/controlhub
internal/app
internal/config
internal/api
internal/web
internal/tray
internal/pairing
internal/mqtt
internal/device
internal/command
internal/trial
internal/license
internal/entitlement
internal/storage
internal/securestore
internal/logging
docs/openapi.yaml
```

## 网络（建议端口）

```text
17890  Local HTTP / Web          （默认 127.0.0.1）
17891  MQTT                       （LAN 可达，需认证 + ACL）
17892  Device Pairing HTTP        （LAN 可达，只接受 Active Pairing Session）
```

LAN API 必须用户显式开启；除 `/health` 外，控制 API 使用 Bearer API Key。

## 内部模块

```text
ControlHub.exe
├── Tray
├── Local Web UI
├── HTTP API
├── Pairing HTTP
├── Embedded MQTT Broker
├── Device Manager
├── Command Engine
├── Trial Manager
├── License Manager
├── Entitlement Manager
├── Secure Store
├── SQLite
└── Logging
```

## Command Engine 处理顺序

```text
API Auth → Schema → Device → Device Ready → Entitlement → Rate Limit
→ Idempotency → Queue → MQTT → ACK → Trial Update → Response
```

Entitlement 必须在 Publish MQTT 前完成。

## 开发里程碑

| 里程碑 | 内容 |
|--------|------|
| CH-01 | App / Config / Logging |
| CH-02 | SQLite / Migration |
| CH-03 | HTTP / Web |
| CH-04 | MQTT |
| CH-05 | Device Manager |
| CH-06 | Pairing |
| CH-07 | Command HTTP → MQTT → ACK |
| CH-08 | Tray / Single Instance |
| CH-09 | Trial |
| CH-10 | License |
| CH-11 | Installer / Firewall / Signing |

第一里程碑：HTTP → MQTT → ESP32 → ACK。

## 数据目录（Windows）

```text
%LOCALAPPDATA%\SmartHID\ControlHub\
├── controlhub.db
├── license.dat
├── logs/
├── cache/
└── backup/
```

## 当前状态

✅ **Phase 1 已实现并通过端到端验证**（2026-08-11）。

实现里程碑：CH-01 App/Config/Logging、CH-02 SQLite、CH-03 HTTP、CH-04 MQTT、CH-05 Device Manager、CH-07 Command HTTP→MQTT→ACK。
跳过：CH-06 Pairing（Phase 4）、CH-08 Tray、CH-09 Trial、CH-10 License、CH-11 Installer。

✅ **F2 可靠性语义参考实现 + 验证**（2026-08-11）。

`cmd/mock-device` 已升级为 ESP32-S3 固件 F1+F2 阶段的 Go 语义参考实现，与 `../smart-hid-firmware/components/` 下的 C 代码一一对照：
- dedup（环形 256）/ boot_id 校验 / TTL 过期 / queue(32) 满 / lease 超时自动释放 / system.release_all / MQTT 断开 release_all
- 验证脚本 `scripts/test-loop-f2.sh` 28/28 全过

✅ **Web 管理界面（CH-03 Web）+ 单元测试安全网**（2026-08-11）。

- `internal/web/` 内嵌单页 Web UI（设备列表 / 命令编辑器 / 命令查询），经 `go:embed` 打进二进制，零构建步骤
- `internal/command/` + `internal/device/` 单元测试覆盖校验边界、设备生命周期、命令闭环全路径（`-race` 无竞争，覆盖率 command 89% / device 93%）

### 运行（本地）

```bash
cd smart-hid-controlhub

# 构建
go build -o bin/controlhub ./cmd/controlhub
go build -o bin/mock-device ./cmd/mock-device

# Phase 1 端到端验证（启 ControlHub + mock-device + curl ENTER）
./scripts/test-loop.sh

# F2 可靠性语义验证（dedup/boot_id/TTL/lease/release_all/queue_full/MQTT disconnect）
./scripts/test-loop-f2.sh

# 单元测试（validator / device / engine，含 -race）
go test ./internal/command/ ./internal/device/ -race -count=1
```

验证通过的链路：`curl → ControlHub HTTP → MQTT → mock-device → USB HID(模拟) → ACK`
- `POST /api/v1/devices/HID-00000001/commands` 发 keyboard tap ENTER → HTTP 200 status=executed
- `GET /api/v1/commands/{request_id}` 查询命令状态
- 错误 API Key → 401；错误 boot_id → 422 status=rejected（STALE_DEVICE_SESSION）
- 同 request_id 重发 → status=duplicate；TTL 越界 → 400
- key_down + lease_ms + release_all → 清空 pressed keys

### 配置

- 默认端口：HTTP 17890 / MQTT 17891
- API Key：`config.yaml` 未指定时启动随机生成并打印到日志
- 示例配置：`config.example.yaml`

### Web 管理界面（CH-03）

ControlHub 内嵌一个单页 Web 管理界面（`internal/web/`，经 `go:embed` 打进二进制，零额外文件、零构建步骤）：

```bash
./bin/controlhub
# 浏览器打开 http://127.0.0.1:17890/
```

1. 从 ControlHub 启动日志复制 API Key（`chk_...`），粘贴到顶栏输入框并保存（存在浏览器 localStorage）
2. 设备列表自动轮询；启动 `mock-device` 或真实 ESP32-S3 后设备自动出现
3. 点设备行的「发送命令」打开命令编辑器：选类型（键盘/鼠标/系统）+ 动作，payload 表单按动作动态生成
4. 发送后实时显示 ACK 结果（executed / duplicate / rejected / expired / accepted-未终态）
5. 「查询命令状态」按 request_id 查询任意命令的 ACK 终态

静态资源（`/`、`/app.js`、`/style.css`）本身不鉴权；真正的控制调用由前端携带 Bearer Key 请求 `/api/v1/*`。

详见 `../docs/05_CONTROLHUB_DETAIL_DESIGN_V1.0.md` 与 `../docs/04_MQTT_AND_CONTROLHUB_API_PROTOCOL_V1.0.md`。

## 相关

- HTTP API 事实源：`./docs/openapi.yaml`（占位）
- MQTT 协议公开定义：`../../smart-ble/core/protocols/hid-command-schema.ts`
- 验收清单：`../docs/10_ACCEPTANCE_CHECKLIST.md` §A
