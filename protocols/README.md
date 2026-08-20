# protocols

Smart HID 协议 Schema 与示例（MQTT Command / ACK / Status）。

## 目录

```text
protocols/
├── schemas/
│   ├── command.schema.json   # SmartHidCommand envelope
│   ├── ack.schema.json       # SmartHidAck
│   └── status.schema.json    # SmartHidStatus
└── examples/
    ├── keyboard_tap.json
    ├── keyboard_hotkey.json
    ├── mouse_click.json
    ├── mouse_move.json
    ├── ack_executed.json
    └── status_online.json
```

## 事实源说明

| 内容 | 事实源 |
|------|--------|
| MQTT Command Schema（公开 TypeScript 定义） | `smart-ble` 仓库 `core/protocols/hid-command-schema.ts` |
| 本目录 JSON Schema（本仓共享校验） | `./schemas/*.schema.json` |

TypeScript 定义（独立仓库 `smart-ble`）为权威事实源；本目录 JSON Schema 用于 ControlHub / Firmware 运行时校验，二者须保持一致。如需修订协议，先改 `smart-ble/core/protocols/` 的事实源，再同步本目录（见 docs/current/DEVELOPMENT_RULES §4）。

## 协议版本

当前 `protocol: "1.0"`。Topic 前缀 `smart-hid/v1/`。
