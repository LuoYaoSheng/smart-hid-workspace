# Smart HID 固件烧录包

ESP32-S3 Smart HID 固件烧录包。版本/commit/校验和见上级目录 `manifest.json` 与 `firmware-SHA256SUMS`。

## 内容

| 文件 | 烧录地址 | 说明 |
|---|---|---|
| `bootloader.bin` | `0x0` | ESP-IDF bootloader |
| `partition-table.bin` | `0x8000` | 分区表 |
| `ota_data_initial.bin` | `0x11000` | OTA 初始数据 |
| `smart-hid-firmware.bin` | `0x20000` | 主固件（应用） |
| `flash.sh` | — | 一键烧录脚本 |
| `SHA256SUMS` | — | 校验文件 |

闪存配置：ESP32-S3，**8MB**，`dio` / `80m`。

## 烧录

### 方式一：脚本（推荐）

```bash
pip install esptool          # 前置依赖
./flash.sh                   # 自动探测串口
./flash.sh /dev/ttyUSB0      # 或指定串口
```

### 方式二：手动

```bash
esptool.py --chip esp32s3 --port /dev/ttyUSB0 --baud 460800 \
  --flash_mode dio --flash_freq 80m --flash_size 8MB \
  write_flash \
  0x0 bootloader.bin 0x8000 partition-table.bin \
  0xd000 ota_data_initial.bin 0x10000 smart-hid-firmware.bin
```

### 方式三：ESP-IDF

若已装 ESP-IDF，把四个 `.bin` 放回 build 目录后：

```bash
idf.py -p /dev/ttyUSB0 flash
```

## 校验

```bash
shasum -a 256 -c SHA256SUMS     # macOS / Linux
certutil -hashfile smart-hid-firmware.bin SHA256   # Windows
```

## 烧录后

1. 设备重启，USB HID 就绪后状态指示亮起。
2. 用 **BLE Toolkit+ 微信小程序** 搜索附近 Smart HID → 配 Wi-Fi → 配对 ControlHub。
3. 详见根目录 `../../docs/quick-start.html`。

## 构建

本包源自 `smart-hid-firmware/` 的 `idf.py build` 产物。重新构建：

```bash
cd smart-hid-firmware && idf.py build
```

> ⚠️ 这是开发构建，未做 Secure Boot / Flash 加密（Phase 5 安全生产后续）。生产固件将单独发布。
