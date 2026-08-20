---
status: SUPERSEDED
authority: historical-only
do_not_implement: true
current_source: docs/current/CURRENT_STATE.md
---

> ⚠️ **历史资料（SUPERSEDED）**：本文件属 2026-08-11 设计资料包快照，其中 Cloud / Trial / License / 商业化等设计已于 2026-08 从产品移除。本文仅作设计推演的历史记录保留，**不得作为当前实现依据**；当前事实见 `docs/current/CURRENT_STATE.md`。

# ControlHub 产品与技术详细设计 v1.0

## 1. 产品形态

正式交付：

```text
ControlHub_Setup.exe
```

安装后：

```text
ControlHub.exe
```

用户体验：
- 双击运行
- 无黑色 CMD 窗口
- 系统托盘
- 本地浏览器控制台
- 可选开机启动

V1 不做 Windows Service。

## 2. 内部模块

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

## 3. 推荐技术

- Go
- `net/http`
- Embedded MQTT Broker
- SQLite
- `go:embed`
- Windows tray
- Windows DPAPI
- `crypto/ed25519`

## 4. 网络

建议端口：

```text
17890 Local HTTP / Web
17891 MQTT
17892 Device Pairing HTTP
```

### HTTP
默认 `127.0.0.1`

### MQTT
LAN 可访问，但必须认证 + ACL。

### Pairing
LAN 可访问，只接受 Active Pairing Session。

## 5. Pairing

ControlHub：

```text
添加设备
→ 创建 Pairing Session
→ token
→ 显示二维码
```

ESP32：

```text
POST /api/v1/pairing/device
```

成功后获得：
- mqtt_host
- mqtt_port
- mqtt_username
- mqtt_credential

## 6. Command Engine

顺序：

```text
API Auth
→ Schema
→ Device
→ Device Ready
→ Entitlement
→ Rate Limit
→ Idempotency
→ Queue
→ MQTT
→ ACK
→ Trial Update
→ Response
```

## 7. Trial

以 Device ID 为核心。

不按 Command 数计费。

Session：
- 第一条 executed command 开始
- inactivity timeout 结束
- 内存累计 + 周期 flush
- 程序退出 flush

## 8. License

- 本地 signed artifact
- Ed25519 verify
- 只内置 Public Key
- Cloud 持有 Private Key
- 支持在线刷新
- 支持离线导入

## 9. SQLite

建议表：

```text
schema_migrations
app_meta
devices
device_credentials
pairing_sessions
licenses
trial_usage
trial_sessions
api_keys
settings
command_history
security_events
```

## 10. 数据目录

```text
%LOCALAPPDATA%\SmartHID\ControlHub\
```

包含：
- controlhub.db
- license.dat
- logs/
- cache/
- backup/

## 11. 开发里程碑

### CH-01
App / Config / Logging

### CH-02
SQLite / Migration

### CH-03
HTTP / Web

### CH-04
MQTT

### CH-05
Device Manager

### CH-06
Pairing

### CH-07
Command HTTP → MQTT → ACK

### CH-08
Tray / Single Instance

### CH-09
Trial

### CH-10
License

### CH-11
Installer / Firewall / Signing
