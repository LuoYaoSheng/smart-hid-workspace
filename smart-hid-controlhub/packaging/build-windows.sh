#!/usr/bin/env bash
# Smart HID ControlHub Windows 构建脚本（CH-P8）。
#
# 用途：在 macOS/Linux 上交叉编译 ControlHub.exe + 用 NSIS 生成 Setup 安装包。
# 前置依赖（macOS）：
#   brew install mingw-w64 makensis
# 前置依赖（Linux）：
#   apt install gcc-mingw-w64-x86-64 nsis
#
# 用法：./packaging/build-windows.sh
# 输出：packaging/ControlHub_Setup.exe
#
# 注意：
#   - 本脚本在 macOS 上无法"运行"生成的 .exe 验证，仅产生构建产物。
#   - fyne.io/systray 在 Windows 需要 GUI subsystem；用 -H windowsgui 链接 flag
#     实现"无 CMD 黑窗"（docs/05 §1 / 验收 A2）。
#   - 真正的 Windows 运行时验证留待 Windows 测试机。

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

OUT_DIR="$ROOT/packaging"
mkdir -p "$OUT_DIR"

echo "==> 1/3 交叉编译 ControlHub.exe (windows/amd64, CGO for systray)"
CGO_ENABLED=1 \
GOOS=windows \
GOARCH=amd64 \
CC=x86_64-w64-mingw32-gcc \
CXX=x86_64-w64-mingw32-g++ \
  go build \
    -ldflags "-H windowsgui -s -w" \
    -o "$OUT_DIR/ControlHub.exe" \
    ./cmd/controlhub

echo "==> 2/3 复制 manifest"
cp "$ROOT/cmd/controlhub/ControlHub.exe.manifest" "$OUT_DIR/ControlHub.exe.manifest"

echo "==> 3/3 NSIS 打包"
if ! command -v makensis >/dev/null 2>&1; then
  echo "WARNING: makensis not found; skipping installer."
  echo "产物：$OUT_DIR/ControlHub.exe + .manifest（可手动分发或用其他打包工具）"
  exit 0
fi

( cd "$OUT_DIR" && makensis controlhub.nsi )

echo ""
echo "==> 完成"
echo "安装包：$OUT_DIR/ControlHub_Setup.exe"
echo ""
echo "后续（Windows 测试机）："
echo "  - 双击 ControlHub_Setup.exe 安装"
echo "  - 安装目录 %LOCALAPPDATA%\\SmartHID\\ControlHub"
echo "  - 验证：托盘图标 + http://127.0.0.1:17890 控制台"
