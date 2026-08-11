#!/usr/bin/env bash
# flash.sh — 烧录 Smart HID 固件到 ESP32-S3
#
# 用法：
#   ./flash.sh                        # 自动探测串口
#   ./flash.sh /dev/ttyUSB0           # 指定串口
#   ./flash.sh /dev/cu.usbmodem1234   # macOS
#
# 前置：已安装 esptool（pip install esptool），ESP32-S3 通过 USB 连接。
# 烧录地址来自 ESP-IDF build/flash_args，勿改。
set -euo pipefail

PORT="${1:-}"
if [ -z "$PORT" ]; then
  # 自动探测：取第一个 cp210x/ch34x/usbmodem 串口
  PORT="$(ls /dev/ttyUSB* /dev/cu.usbmodem* /dev/cu.SLAB_USBtoUART* 2>/dev/null | head -1 || true)"
  if [ -z "$PORT" ]; then
    echo "未探测到串口。请传入：$0 /dev/ttyUSB0" >&2
    exit 1
  fi
  echo "自动探测到串口：$PORT"
fi

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

echo "==> 烧录 ESP32-S3 @ $PORT  (dio / 80m / 8MB)"
esptool.py --chip esp32s3 --port "$PORT" --baud 460800 \
  --flash_mode dio --flash_freq 80m --flash_size 8MB \
  write_flash \
  0x0     bootloader.bin \
  0x8000  partition-table.bin \
  0xd000  ota_data_initial.bin \
  0x10000 smart-hid-firmware.bin

echo ""
echo "==> 烧录完成。设备将重启，状态 LED 亮起。"
echo "==> 下一步：用 BLE Toolkit+ 小程序配网（Wi-Fi + ControlHub 配对）。"
