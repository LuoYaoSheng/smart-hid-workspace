#!/usr/bin/env bash
# build-releases.sh — 重新生成 smart-hid-web/downloads/ 下的全部发布资产
#
# 产出：
#   downloads/controlhub/controlhub-darwin-arm64        (go build, stripped)
#   downloads/controlhub/controlhub-windows-amd64.exe
#   downloads/firmware/{bootloader,partition-table,ota_data_initial,smart-hid-firmware}.bin
#   downloads/{controlhub,firmware}/SHA256SUMS
#
# 前置：go（任意平台）、ESP-IDF（仅固件重建需要，已存在 build/ 则跳过）。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"   # Smart-HID-Workspace/
DL="$ROOT/smart-hid-web/downloads"
cd "$ROOT"

VERSION="${VERSION:-v0.1.0-scaffold}"
LDF="-s -w -X main.version=$VERSION"
echo "==> 构建 ControlHub 二进制 ($VERSION)"

mkdir -p "$DL/controlhub"
GOOS=darwin  GOARCH=arm64 go build -ldflags "$LDF" -o "$DL/controlhub/controlhub-darwin-arm64"    ./smart-hid-controlhub/cmd/controlhub
GOOS=windows GOARCH=amd64 go build -ldflags "$LDF" -o "$DL/controlhub/controlhub-windows-amd64.exe" ./smart-hid-controlhub/cmd/controlhub
(cd "$DL/controlhub" && shasum -a 256 * > SHA256SUMS)
echo "   controlhub done"

echo "==> 同步固件烧录包"
FB="$ROOT/smart-hid-firmware/build"
if [ ! -f "$FB/smart-hid-firmware.bin" ]; then
  echo "   未找到固件 build 产物，跳过（如需重建：cd smart-hid-firmware && idf.py build）"
else
  mkdir -p "$DL/firmware"
  cp "$FB/bootloader/bootloader.bin"         "$DL/firmware/"
  cp "$FB/partition_table/partition-table.bin" "$DL/firmware/"
  cp "$FB/ota_data_initial.bin"              "$DL/firmware/"
  cp "$FB/smart-hid-firmware.bin"            "$DL/firmware/"
  (cd "$DL/firmware" && shasum -a 256 bootloader.bin partition-table.bin ota_data_initial.bin smart-hid-firmware.bin > SHA256SUMS)
  echo "   firmware done"
fi

echo "==> 同步 API 契约到落地页（自包含）"
cp "$ROOT/smart-hid-controlhub/docs/openapi.yaml" "$ROOT/smart-hid-web/api/openapi.yaml"
echo "   openapi.yaml projected"

echo ""
echo "完成。产物："
ls -lh "$DL/controlhub/" "$DL/firmware/"
