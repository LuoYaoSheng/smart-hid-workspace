---
status: CURRENT
authority: canonical
---

# ARCHITECTURE — 当前架构

> 只描述 main 分支当前真实存在的架构。历史架构（含 Cloud / License 层）见
> `docs/archive/02`，与本图冲突之处一律以本图为准。

## 总体结构

```text
External Client（第三方程序 / 演示台页面）
   ↓ HTTP :17890（Bearer API Key；LAN 需显式开启）
ControlHub（Go，单进程）
   ├─ HTTP API（net/http，11 path）
   ├─ Command Engine（校验 → publish → 同步等 ACK）
   ├─ Device Manager（在线 / boot_id / USB ready）
   ├─ Pairing（session + 动态 QR；设备侧 listener :17892，可关）
   ├─ API Key / Settings Store
   ├─ SQLite（命令历史 / 设备 / 凭据 / 配置）
   ├─ Local Web（控制台 / 演示台 / WS realtime，go:embed 静态）
   └─ Embedded MQTT Broker（mochi-mqtt :17891，每设备凭据 + ACL）
            ↓ MQTT（QoS1，严禁 retained command）
        ESP32-S3（固件，C / ESP-IDF v5.4.4）
         ├─ 命令引擎（队列 32 / 去重 256 / TTL / lease / release_all）
         ├─ USB Composite HID（TinyUSB：键盘 + 鼠标）
         └─ Status / 心跳 / LWT（掉线自动释放全部按键）
            ↓ USB HID
        Target PC（零软件，BIOS 级通用）
```

辅助（固件源码已实现，未上真机）：BLE Toolkit+ 小程序（独立仓 smart-ble）经
BLE Provision 为固件配网（NimBLE，协议 `protocols/ble/PROVISIONING_V1.md`）。

## MQTT 网络模型（M1-G3 拆分）

一个 `mqtt.host` 承担三个语义的时代已结束；现在是三个明确概念：

```text
mqtt.bind_host（默认 0.0.0.0）
  = embedded broker 监听地址
  = LAN 设备可达（broker 有 per-device 凭据 + ACL 保护）

internal connect address（非用户配置）
  = ControlHub 自身连本机 broker
  = 由 bind_host 推导（通配 → 127.0.0.1；具体 IP → 原值）

mqtt.advertise_host（默认空 = 自动解析，netaddr.Resolver）
  = 返回给 ESP32 的 broker 地址（pairing 响应 / QR 载荷）
  解析优先级：
    1. 显式配置（Load 时校验：拒绝环回/通配/localhost/链路本地）
    2. 设备/浏览器请求实际到达的本机地址（http.LocalAddrContextKey）
    3. 向 peer 的 UDP 出口推导（不发包）
    4. 唯一可用 LAN IPv4（过滤 down/loopback/link-local/docker/veth）
    5. 0 个或多个候选 → 明确失败（列出候选，提示配置 advertise_host）
  唯一例外：peer 是环回（本机 mock/测试）→ 环回地址合法
  legacy mqtt.host 自动迁移（环回→bind 兼容；LAN IP→bind+advertise）+ 一次性警告
```

pairing 顺序约束：**先解析 advertise → 再原子消费 token**——endpoint 解析失败
返回 503 且 token 保持 pending，用户修复后原 token 可重试。

内部 MQTT 凭据：不再有固定默认密码；`mqtt.username/password` 成对显式配置
或留空由每次启动随机生成（仅内存，不持久化、不进日志）。设备凭据始终
per-device 随机 + ACL（PerDeviceHook），与内部凭据互不相干。

## 固件配网链（M1-G3）

```text
BOOT → LOAD_CONFIG（NVS rt_active/rt_pending；schema_version 守卫）
 ├─ valid active ──→ CONNECTING_WIFI → CONNECTING_MQTT → READY
 ├─ no config ─────→ UNPROVISIONED → BLE PROVISIONING
 ├─ DEV_STATIC（显式）→ Kconfig 开发配置（仅内存，绝不写 NVS）
 └─ 版本未知 ──────→ RECOVERY（BLE 开，active 只读）

BLE candidate（分帧写入）→ 校验 → stage pending → CONNECTING_WIFI
 → PAIRING（HTTP :17892）→ 凭据先落盘（pending.complete=1）
 → promote 为 active → CONNECTING_MQTT → READY（BLE 广播停）
失败：discard pending，active 不动；运行期持续失联 5 分钟 → RECOVERY
崩溃恢复：complete pending 在下次 boot 自动 promote（token 已消费、
凭据已持久化——唯一无死状态路径）
```

## 仓库布局

```text
smart-hid-workspace/
├── smart-hid-controlhub/   # Go：本地控制程序（HTTP API + 内嵌 MQTT + Web + 托盘）
│   ├── cmd/controlhub/     # 主入口（headless / -tray）
│   ├── cmd/mock-device/    # Go 参考实现 = 虚拟 ESP32（测试替身）
│   └── docs/openapi.yaml   # HTTP API 事实源
├── smart-hid-firmware/     # C：ESP32-S3 固件（未上真机）
├── smart-hid-web/          # 纯静态官网（Pages 托管）+ 下载中心
├── protocols/              # MQTT Command / ACK / Status JSON Schema + 示例
├── docs/current/           # ✅ 当前事实源（本目录）
├── docs/archive/           # ⛔ 历史设计资料（SUPERSEDED，禁止指导实现）
└── scripts/                # 仓库级检查脚本（check-governance.sh）
```

配套独立仓库 [smart-ble](https://github.com/LuoYaoSheng/smart-ble)：BLE Toolkit+
微信小程序；MQTT Command Schema 的 TypeScript 事实源在该仓库。

## 事实源关系

```text
HTTP API 契约        smart-hid-controlhub/docs/openapi.yaml
MQTT 消息契约        protocols/schemas/*.schema.json（TS 权威源在 smart-ble 仓）
固件协议实现         smart-hid-firmware/components/smart_hid_protocol/
当前状态 / 路线      docs/current/（本目录）
历史设计推演         docs/archive/（SUPERSEDED）
```

修改协议时四者必须同步（见 DEVELOPMENT_RULES §4 Contract First）。

## 关键设计决定（现行）

- 命令走 HTTP 同步闭环（调用方需要 ACK 结果），WebSocket 只下行事件，不做命令通道
- 严禁 retained command；QoS1 + request_id 去重保证幂等
- 设备不预烧凭证：配对时动态签发每设备 MQTT 凭据（SHA-256 入库，可撤销）
- 控制链路不出局域网；HTTP API 默认 `127.0.0.1` 需显式开 LAN，MQTT broker
  默认 `0.0.0.0`（设备接入是主场景，凭据 + ACL 兜底）
- 固件网络配置以 NVS 为正式事实源；设备不猜 MQTT 地址——pairing 响应的
  advertised endpoint 是唯一来源（protocols/ble/PROVISIONING_V1.md §9）
- 已知架构债务见 HARDENING_BACKLOG（G4 Release / M2 各节），此处不美化
