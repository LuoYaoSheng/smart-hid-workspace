# smart-hid-controlhub

Smart HID 本地控制程序：接收第三方程序的 HTTP 指令，经内嵌 MQTT Broker 下发给 ESP32-S3，由设备以真实 USB HID 键鼠输出到目标电脑。

```text
第三方程序 ──HTTP:17890──▶ ControlHub ──MQTT:17891──▶ ESP32-S3 ──USB HID──▶ 目标电脑
                              │ ▲
                              │ └─ 设备配对 :17892（仅配对会话期开放）
```

实时控制完全本地，无云、无账号、无授权门禁。技术栈：Go 标准库 net/http + [mochi-mqtt](https://github.com/mochi-mqtt/server)（内嵌 Broker，per-device 凭据 + ACL）+ SQLite（modernc 纯 Go 驱动）+ [gorilla/websocket](https://github.com/gorilla/websocket)（实时事件通道）。

## 快速开始

```bash
# 直接运行（无 config.yaml 时使用内置默认值；首次启动生成 API Key 打印到日志并落盘 data/initial-api-key.txt）
go run ./cmd/controlhub

# 指定配置
go run ./cmd/controlhub -config config.yaml

# 系统托盘模式（macOS/Windows；systray 需主线程）
go run ./cmd/controlhub -tray
```

预编译二进制（macOS arm64 / Windows amd64）见仓库 `smart-hid-web/downloads/controlhub/` 或 [Releases](https://github.com/LuoYaoSheng/smart-hid-workspace/releases)。

## 配置参考

复制 `config.example.yaml` 为 `config.yaml` 按需修改；全部默认开启，老配置文件无新字段时行为不变：

| 字段 | 默认 | 说明 |
|---|---|---|
| `http.host` / `http.port` | `127.0.0.1` / `17890` | 本地 HTTP 服务（API + 内置页面） |
| `http.lan_mode` | `false` | 启动即监听 `0.0.0.0`（控制台运行时开关持久化后优先） |
| `http.enable_api` | `true` | `false` = 不注册 `/api/v1`（纯静态模式） |
| `mqtt.port` | `17891` | 内嵌 MQTT Broker（hub 自用 + 设备接入，per-device 凭据） |
| `pairing.enabled` / `pairing.port` | `true` / `17892` | 设备侧配对服务（QR 载荷端口同步生效） |
| `web.console` / `web.demo` | `true` / `true` | 控制台 / 模拟键鼠演示台页面开关 |
| `web.realtime` | `true` | WebSocket 实时事件通道 |
| `api_key` | 空 | 留空则首启随机生成（`chk_` 前缀，哈希落库） |

安全基线：HTTP 默认只听回环；LAN 模式需显式开启；控制 API 全部 Bearer API Key 鉴权；API Key 可经 `POST /api/v1/api-keys/rotate` 或托盘菜单轮换。

## 内置 Web 界面

| 页面 | 路径 | 说明 |
|---|---|---|
| 控制台 | `/` | 设备列表 / 命令编辑器 / API Key / LAN 模式 / 配对（QR） |
| **模拟键鼠演示台** | `/demo.html` | 浏览器遥控另一台电脑：可视化键盘（修饰键锁定组合）、实体键盘直通、触控板（增量合流）、文本连打；**设备芯片条** 1-click 切换主控，**🎯 广播模式**把每次操作同时发给所有就绪设备 |
| 实时事件通道 | `/api/v1/realtime?key=` | WebSocket 只下行推送 `hello` / `device` / `ack` 事件——演示页的事件流面板即由它驱动，多端打开可实时观战 |

页面均零构建（go:embed 内嵌静态资源），静态本身不鉴权，控制调用由前端携带 Bearer Key 请求 `/api/v1/*`。

## 设备配对

ControlHub 生成配对会话（QR 载荷 `shid://pair?token=...&host=<lan-ip>&port=<pairing.port>`）→ 设备 Provision Mode 扫码接入 `:17892` → 签发**每设备独立 MQTT 凭据**（含 per-device ACL，设备只能收发自己的 topic）→ 上线。配对服务仅会话期开放。

## HTTP API

完整契约见 `docs/openapi.yaml`（11 paths：devices / commands / api-keys / settings / pairing / realtime）。示例：

```bash
KEY=chk_xxx
curl -H "Authorization: Bearer $KEY" http://127.0.0.1:17890/api/v1/devices

curl -X POST -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"protocol":"1.0","request_id":"t1","device_id":"HID-00000001","target_boot_id":"B-xxx","type":"keyboard","action":"tap","ttl_ms":2000,"payload":{"key":"ENTER","hold_ms":40}}' \
  http://127.0.0.1:17890/api/v1/devices/HID-00000001/commands
```

命令闭环：校验 → 设备就绪检查 → publish MQTT(QoS1) → 等终态 ACK（TTL 内）→ `200 executed` / `422 rejected` / `504 expired` / `202` 未回执。可靠性语义（request_id 去重、boot_id 防旧命令、TTL、lease 超时释放、断线 release_all）在设备侧保证，见仓库根 README。

## 测试与验证

```bash
go test ./...                 # 全模块单测（api/command/device/pairing/config/web/realtime...）

# 可靠性语义 28 项端到端（真二进制 + mock-device，无需硬件）
bash scripts/test-loop-f2.sh
```

`cmd/mock-device` 是固件的 Go 参考实现（含 dedup/boot_id/TTL/lease/queue 语义），也是零硬件开发的联调伙伴——配对后即成为一台"虚拟 ESP32"。

## 构建

```bash
go build ./cmd/controlhub                    # 本机
# 双平台交叉编译 + SHA256 + 固件打包（工作区根目录执行）
bash ../smart-hid-web/downloads/build-releases.sh
```

## 目录结构

```text
cmd/controlhub/        入口（headless / -tray 托盘模式）
cmd/mock-device/       设备模拟器（可靠性语义参考实现，联调用）
internal/app/          装配与生命周期（Build/Start/Wait/Stop）
internal/api/          HTTP API + 静态托管 + WebSocket 实时通道
internal/command/      命令引擎（校验/publish/等 ACK/持久化 + ACK 观察者）
internal/device/       设备注册表与状态
internal/mqtt/         内嵌 Broker（per-device 凭据 + ACL）+ hub 客户端
internal/pairing/      配对会话 + 设备侧 listener + QR 载荷
internal/apikey/       API Key（哈希存储/轮换/首启生成）
internal/settings/     运行时可改设置（LAN 模式）
internal/storage/      SQLite + 迁移
internal/web/          go:embed 静态资源（控制台/演示台）+ 页面门禁
internal/tray/         系统托盘
internal/protocol/     协议类型（Status 等；Command/Ack 事实源在 smart-ble）
```

协议事实源：MQTT Command/Ack Schema 的 TypeScript 定义在 [smart-ble](https://github.com/LuoYaoSheng/smart-ble)（`core/protocols/`），JSON Schema 镜像见工作区 `protocols/schemas/`。
