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

辅助（PLANNED，未实现）：BLE Toolkit+ 小程序（独立仓 smart-ble）经 BLE 为固件配网；
当前固件 Wi-Fi / MQTT 为 Kconfig 编译期固定值。

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
- 控制链路不出局域网；默认 `127.0.0.1`，LAN 各面（HTTP / broker）需显式开启
- 已知架构债务（mqtt.host 一字段三用等）见 HARDENING_BACKLOG M1-G3，此处不美化
