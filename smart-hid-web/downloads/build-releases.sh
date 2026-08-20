#!/usr/bin/env bash
# build-releases.sh — 重新生成 smart-hid-web/downloads/ 下的全部发布资产（M1-G4 重写）
#
# 与旧版的区别（旧版六病，见 docs/current/HARDENING_BACKLOG M1-G4）：
#   1. 版本事实源 = 仓库根 VERSION 文件（不再有 v0.1.0-scaffold 默认值）
#   2. dirty tree 默认拒绝发布（DEV_BUILD=true 显式放行开发构建）
#   3. 固件经 scripts/build-firmware.sh fullclean 重建（绝不复制本机旧 build/）
#   4. SHA256SUMS 显式文件清单（绝不 `shasum *` 自包含）+ shasum -c 自校验
#   5. 产出 manifest.json（version/commit/build_time/artifacts{sha256,size,type}）
#   6. openapi.yaml 投影后 diff 防漂移
#
# 产出：
#   downloads/controlhub/{controlhub-darwin-arm64, controlhub-windows-amd64.exe, controlhub-SHA256SUMS}
#   downloads/firmware/{bootloader,partition-table,ota_data_initial,smart-hid-firmware}.bin,
#                flash.sh, firmware-SHA256SUMS
#   downloads/manifest.json + README_RELEASE.md
#
# 前置：go、git、ESP-IDF 环境（idf.py 在 PATH）、shasum。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"   # Smart-HID-Workspace/
DL="$ROOT/smart-hid-web/downloads"
cd "$ROOT"

# ----------------------------------------------------------
# 1) 前置检查：版本源 + dirty tree + 工具
# ----------------------------------------------------------
[ -f "$ROOT/VERSION" ] || { echo "ERROR: 缺少根 VERSION 文件（唯一版本事实源）" >&2; exit 1; }
VERSION="$(tr -d ' \n' < "$ROOT/VERSION" | sed 's/^v//')"
case "$VERSION" in
  [0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "ERROR: VERSION 文件内容「${VERSION}」不是 x.y.z 形态" >&2; exit 1 ;;
esac
echo "==> 版本：${VERSION}（来源 VERSION 文件）"

if [ "${DEV_BUILD:-false}" != "true" ]; then
  if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
    echo "ERROR: 工作区不干净（git status --porcelain 非空）；生产 release 必须 clean tree。" >&2
    echo "       开发构建请显式 DEV_BUILD=true（产物会标记 dirty）。" >&2
    git status --short >&2 || true
    exit 1
  fi
  DIRTY="false"
else
  DIRTY="true"
  echo "   ⚠ DEV_BUILD：跳过 dirty 检查，产物将标记 dirty=true"
fi

COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

command -v go >/dev/null || { echo "ERROR: go 不在 PATH" >&2; exit 1; }
# sha 工具：优先 shasum（macOS），回退 sha256sum（Linux/CI 容器）；两者输出格式一致
SHA="$(command -v shasum || command -v sha256sum || true)"
[ -n "$SHA" ] || { echo "ERROR: shasum / sha256sum 均不在 PATH" >&2; exit 1; }
echo "   sha 工具：$SHA"

# ----------------------------------------------------------
# 2) ControlHub：双平台构建（ldflags 注入版本元数据）
# ----------------------------------------------------------
PKG="smart-hid-controlhub/internal/buildinfo"
LDF="-s -w -X ${PKG}.Version=$VERSION -X ${PKG}.Commit=$COMMIT -X ${PKG}.Date=$BUILD_TIME -X ${PKG}.Dirty=$DIRTY"
echo "==> 构建 ControlHub 二进制（$VERSION @ ${COMMIT}）"
mkdir -p "$DL/controlhub"
GOOS=darwin  GOARCH=arm64 go build -ldflags "$LDF" -o "$DL/controlhub/controlhub-darwin-arm64"       ./smart-hid-controlhub/cmd/controlhub
GOOS=windows GOARCH=amd64 go build -ldflags "$LDF" -o "$DL/controlhub/controlhub-windows-amd64.exe" ./smart-hid-controlhub/cmd/controlhub

# 版本注入自证（本机可运行的平台）：防止 ldflags 拼写错误静默失效
INJ="$( "$DL/controlhub/controlhub-darwin-arm64" -version )"
echo "$INJ" | grep -q "version=$VERSION" || { echo "ERROR: 版本注入校验失败：$INJ" >&2; exit 1; }
echo "$INJ" | grep -q "commit=$COMMIT"   || { echo "ERROR: commit 注入校验失败：$INJ" >&2; exit 1; }
echo "   controlhub done（注入自证：${INJ}）"

# ----------------------------------------------------------
# 3) 固件：fullclean 干净重建（绝不复制本机旧 build/）
# ----------------------------------------------------------
STAGE="$(mktemp -d)/fw"
echo "==> 固件干净构建"
"$ROOT/scripts/build-firmware.sh" "$STAGE"
mkdir -p "$DL/firmware"
for f in bootloader.bin partition-table.bin ota_data_initial.bin smart-hid-firmware.bin; do
  cp "$STAGE/$f" "$DL/firmware/$f"
done
FW_META="$(cat "$STAGE/firmware-build.json")"
echo "   firmware done（${FW_META}）"

# ----------------------------------------------------------
# 4) openapi.yaml 投影 + 防漂移
# ----------------------------------------------------------
echo "==> 同步 API 契约到落地页（自包含）+ 防漂移检查"
if [ -f "$ROOT/smart-hid-web/api/openapi.yaml" ] && ! diff -q \
     "$ROOT/smart-hid-controlhub/docs/openapi.yaml" "$ROOT/smart-hid-web/api/openapi.yaml" >/dev/null; then
  echo "ERROR: smart-hid-web/api/openapi.yaml 与事实源已漂移，先手工核对差异" >&2
  diff "$ROOT/smart-hid-controlhub/docs/openapi.yaml" "$ROOT/smart-hid-web/api/openapi.yaml" | head -20 >&2 || true
  exit 1
fi
cp "$ROOT/smart-hid-controlhub/docs/openapi.yaml" "$ROOT/smart-hid-web/api/openapi.yaml"
echo "   openapi.yaml projected（一致）"

# ----------------------------------------------------------
# 5) SHA256SUMS：显式清单 + 自校验（SUMS 不含自己）
# ----------------------------------------------------------
cd "$DL/controlhub"
"$SHA" -a 256 \
  controlhub-darwin-arm64 \
  controlhub-windows-amd64.exe \
  > controlhub-SHA256SUMS
"$SHA" -c controlhub-SHA256SUMS

cd "$DL/firmware"
"$SHA" -a 256 \
  bootloader.bin \
  partition-table.bin \
  ota_data_initial.bin \
  smart-hid-firmware.bin \
  flash.sh \
  > firmware-SHA256SUMS
"$SHA" -c firmware-SHA256SUMS

# ----------------------------------------------------------
# 6) manifest.json（provenance：version/commit/time + 每 artifact 校验和）
# ----------------------------------------------------------
cd "$ROOT"
python3 - "$DL" "$VERSION" "$COMMIT" "$BUILD_TIME" "$DIRTY" "$FW_META" <<'PYEOF'
import hashlib, json, sys, os
dl, version, commit, build_time, dirty, fw_meta = sys.argv[1:7]
fw_meta = json.loads(fw_meta)

def entry(rel, typ):
    p = os.path.join(dl, rel)
    h = hashlib.sha256(open(p, "rb").read()).hexdigest()
    return {"name": rel, "sha256": h, "size": os.path.getsize(p), "type": typ}

manifest = {
    "product": "smart-hid",
    "version": version,
    "commit": commit,
    "build_time": build_time,
    "dirty": dirty == "true",
    "firmware": fw_meta,
    "artifacts": [
        entry("controlhub/controlhub-darwin-arm64", "controlhub"),
        entry("controlhub/controlhub-windows-amd64.exe", "controlhub"),
        entry("controlhub/controlhub-SHA256SUMS", "controlhub"),
        entry("firmware/bootloader.bin", "firmware"),
        entry("firmware/partition-table.bin", "firmware"),
        entry("firmware/ota_data_initial.bin", "firmware"),
        entry("firmware/smart-hid-firmware.bin", "firmware"),
        entry("firmware/flash.sh", "firmware"),
        entry("firmware/firmware-SHA256SUMS", "firmware"),
    ],
}
with open(os.path.join(dl, "manifest.json"), "w") as f:
    json.dump(manifest, f, indent=2, ensure_ascii=False)
    f.write("\n")
print("   manifest.json 写入 %d 个 artifact 条目" % len(manifest["artifacts"]))
PYEOF

# ----------------------------------------------------------
# 7) README_RELEASE.md（诚实状态：BUILD VERIFIED ≠ HARDWARE VERIFIED）
# ----------------------------------------------------------
cat > "$DL/README_RELEASE.md" <<'EOF_TMPL'
# Smart HID Release v@VERSION@

| 项 | 值 |
|---|---|
| version | @VERSION@ |
| commit | @COMMIT@ |
| build time (UTC) | @BUILD_TIME@ |
| dirty build | @DIRTY@ |

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
EOF_TMPL
# 引用 heredoc 保护反引号/``` 围栏；占位符在此替换
sed -i '' -e "s/@VERSION@/$VERSION/g" -e "s/@COMMIT@/$COMMIT/g" \
  -e "s/@BUILD_TIME@/$BUILD_TIME/g" -e "s/@DIRTY@/$DIRTY/g" "$DL/README_RELEASE.md"

echo ""
echo "完成。产物："
ls -lh "$DL/controlhub/"
ls -lh "$DL/firmware/"
echo "manifest → $DL/manifest.json"
