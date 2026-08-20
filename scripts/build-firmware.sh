#!/usr/bin/env bash
# build-firmware.sh — Smart HID 固件干净构建（M1-G4）
#
# 职责：fullclean → set-target → build（默认 provisioning 配置），
#       并把烧录所需 bin 复制到目标目录 + 生成构建元数据片段。
# 供本地 release（build-releases.sh）与 CI（release.yml）复用。
# 禁止 flash（硬件验收是 M2-G1 独立任务）。
#
# 用法：
#   scripts/build-firmware.sh <out_dir>          # 必须 source 过 ESP-IDF export
# 环境变量：
#   SKIP_FULLCLEAN=1  复用既有 build/（仅 CI 缓存场景；正式 release 禁用）
set -euo pipefail

OUT_DIR="${1:?usage: build-firmware.sh <out_dir>}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FW="$ROOT/smart-hid-firmware"

command -v idf.py >/dev/null || { echo "ERROR: idf.py 不在 PATH（先 source ESP-IDF export.sh）" >&2; exit 1; }

cd "$FW"
if [ "${SKIP_FULLCLEAN:-0}" != "1" ]; then
  echo "==> idf.py fullclean"
  idf.py fullclean
fi
rm -f sdkconfig   # 重新生成，保证 sdkconfig.defaults 全量生效（不残留本机旧值）
echo "==> idf.py set-target esp32s3 + build"
idf.py set-target esp32s3 >/dev/null
idf.py build

VERSION="$(tr -d ' \n' < "$ROOT/VERSION" | sed 's/^v//')"
mkdir -p "$OUT_DIR"
cp build/bootloader/bootloader.bin            "$OUT_DIR/"
cp build/partition_table/partition-table.bin  "$OUT_DIR/"
cp build/ota_data_initial.bin                 "$OUT_DIR/"
cp build/smart-hid-firmware.bin               "$OUT_DIR/"

# 构建元数据（进 manifest 由 build-releases.sh 合并；不伪造字段）
IDF_VER="$(idf.py --version 2>/dev/null | awk '{print $2}' | head -1)"
cat > "$OUT_DIR/firmware-build.json" <<EOF
{
  "version": "$VERSION",
  "esp_idf": "${IDF_VER:-unknown}",
  "app_bin_bytes": $(wc -c < "$OUT_DIR/smart-hid-firmware.bin" | tr -d ' '),
  "built_from": "scripts/build-firmware.sh (fullclean + set-target + build)"
}
EOF

echo "==> firmware artifacts → $OUT_DIR"
ls -l "$OUT_DIR"
