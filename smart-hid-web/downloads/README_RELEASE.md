# Smart HID Release v1.1.0

| 项 | 值 |
|---|---|
| version | 1.1.0 |
| commit | 118c6eb |
| build time (UTC) | 2026-08-20T09:23:43Z |
| dirty build | false |

## 内容

- `controlhub/`：ControlHub 桌面程序（macOS arm64 / Windows amd64），`-version` 可查版本
- `firmware/`：ESP32-S3 固件烧录包（`flash.sh` 一键烧录；esptool 需要）
- 校验：`shasum -c controlhub/controlhub-SHA256SUMS`、`shasum -c firmware/firmware-SHA256SUMS`，
  或核对 `manifest.json` 中每个 artifact 的 sha256

## 平台

- ControlHub：macOS (Apple Silicon) / Windows x64
- 固件：ESP32-S3（8MB flash）

## 硬件状态（诚实边界）

```text
Firmware BUILD VERIFIED（ESP-IDF 干净构建 + 36 项宿主单测）
ControlHub TEST VERIFIED（go test/-race + 28 项真二进制 mock e2e）
Hardware NOT VERIFIED —— 未在任何真实 ESP32-S3 上烧录/验证
```

USB HID 实效、BLE 配网真机链路、BIOS/登录界面均未做硬件验收（M2-G1 独立任务）。
