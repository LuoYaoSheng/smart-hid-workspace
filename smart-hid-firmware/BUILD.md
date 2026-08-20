# Smart HID Firmware — 编译烧录指南

> ✅ **本机已验证编译通过**（2026-08-11）：ESP-IDF v5.4.4 / macOS arm64 / Python 3.12。
> 产出 `smart-hid-firmware.bin`（921KB）。烧录需真实 ESP32-S3 硬件。

## 1. 环境要求（已验证）

- **ESP-IDF v5.4.4**（推荐 v5.4.x；v5.0+ 亦可，需 esp_tinyusb ≥ 2.0）
- **Python 3.9–3.12**（3.13+ 暂不受 esp-idf-kconfig 完整支持）
- 目标芯片：**ESP32-S3**（建议模组 ESP32-S3-N8R2 / N16R8，带 PSRAM）
- USB 数据线（接 ESP32-S3 的 USB-OTG 口，用于烧录 + USB HID 复用）
- macOS arm64 / x86_64 / Linux / Windows 均可

## 2. 安装 ESP-IDF（如未装）

```bash
mkdir -p ~/esp && cd ~/esp
git clone --recurse-submodules -b v5.4.4 https://github.com/espressif/esp-idf.git
cd esp-idf && ./install.sh esp32s3
. ./export.sh   # 每个新终端都要 source
```

> 若 GitHub 不通，设 `export IDF_GITHUB_ASSETS=dl.espressif.com/github_assets` 让下载走 Espressif CDN。

## 3. 配置

```bash
cd smart-hid-firmware
idf.py set-target esp32s3
idf.py menuconfig
```

`Smart HID` 菜单关键字段：

| 配置 | 默认 | 说明 |
|------|------|------|
| `SMART_HID_DEVICE_ID` | `HID-00000001` | 首次烧录用，之后存 NVS |
| `SMART_HID_MQTT_BROKER_HOST` | `192.168.1.100` | ControlHub LAN IP |
| `SMART_HID_MQTT_BROKER_PORT` | `17891` | |
| `SMART_HID_MQTT_USERNAME` | `controlhub` | |
| `SMART_HID_MQTT_PASSWORD` | `change-me-in-production` | **生产必改** |
| `SMART_HID_WIFI_SSID` | (空) | F1/F2 固定 |
| `SMART_HID_WIFI_PASSWORD` | (空) | F1/F2 固定 |

> F3+ 阶段 Wi-Fi/MQTT 改由 BLE 配网写入 NVS，Kconfig 仅作首次启动兜底。

## 4. 编译

```bash
idf.py build
```

产物：`build/smart-hid-firmware.bin`、`build/partition_table/partition-table.bin`、`build/bootloader/bootloader.bin`。

## 5. 烧录

```bash
# 接好 USB，查看端口
idf.py -p /dev/cu.usbmodem* flash monitor
```

`monitor` 会实时显示串口日志（JSON 格式），看到 `Smart HID Firmware ready` 即启动完成。

## 6. 验证（与 ControlHub 闭环）

1. 在另一终端启动 ControlHub（见 `../smart-hid-controlhub/`）。
2. 等 ESP32 上电 + 连 Wi-Fi + 连 MQTT。
3. ControlHub 应识别到设备（`GET /api/v1/devices` 返回 online=true）。
4. 测试命令：
   ```bash
   curl -X POST -H "Authorization: Bearer $API_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"protocol":"1.0","request_id":"t01","device_id":"HID-00000001",
          "target_boot_id":"B-XXXXXX","type":"keyboard","action":"tap",
          "ttl_ms":3000,"payload":{"key":"ENTER"}}' \
     http://127.0.0.1:17890/api/v1/devices/HID-00000001/commands
   ```
5. 目标电脑光标聚焦到文本框 → ENTER 应被按下。

> 真实 boot_id 从 `GET /devices` 响应取。

## 7. USB Composite HID 验收

烧录后接 Windows / macOS：
- 设备管理器 / 系统信息应识别为"Keyboard + Mouse"组合设备，名为 "Smart HID"。
- §B 全部 keyboard / mouse action 用 ControlHub 逐项发命令测试。

## 8. 量产安全（F5 才做）

**不要在功能闭环之前锁 eFuse。**

F5 启用：
- Secure Boot v2
- Flash Encryption (AES-XTS)
- NVS Encryption
- Firmware Signing

见 `../docs/archive/06_ESP32_FIRMWARE_DETAIL_DESIGN_V1.0.md` §14 / §15。

## 9. F1→F2→F3 演进

| 里程碑 | 范围 | 当前 |
|--------|------|------|
| F1 | USB Keyboard/Mouse + 固定 Wi-Fi/MQTT + ControlHub | ✅ C 代码完成 + `idf.py build` 通过（未硬件验证） |
| F2 | ACK / request_id / dedup / boot_id / TTL / lease / release_all / queue | ✅ C 代码完成 + 编译通过 + Go 参考验证 28/28 |
| F3 | BLE Provision + Pairing | ⏳ 待开始 |
| F4 | NVS Security + Trial Anchor + OTA Foundation | ⏳ |
| F5 | Production Security + Factory Tool | ⏳ |

## 10. 故障排查

| 现象 | 排查 |
|------|------|
| 编译报 `tusb.h` not found | 检查 `CONFIG_TINYUSB_HID_ENABLED=y`（sdkconfig.defaults 已设） |
| 烧录失败 | 确认 USB 接的是 OTG 口（不是 UART 口）；按住 BOOT + 按 RST 进下载模式 |
| 设备 online=false | 检查 Wi-Fi SSID/密码；串口看 `got ip` 日志 |
| Command 一直 timeout | 确认 MQTT broker IP/端口正确；检查防火墙放行 17891 |
| STALE_DEVICE_SESSION | `target_boot_id` 与设备当前 `boot_id` 不符，从 `/devices` 重新取 |
