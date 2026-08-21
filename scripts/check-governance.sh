#!/usr/bin/env bash
# check-governance.sh — 仓库治理守卫（M1-G1 建立；M1-G4 扩展）
#
# 目的：防止历史商业设计结论被写回当前事实文档。只做静态文本检查，
#       不运行项目。保持简单、透明、易维护。
#
# 规则：
#   1. docs/current/*.md 必须带 status: CURRENT / authority: canonical 头
#   2. docs/archive/*.md 必须带 status: SUPERSEDED 头
#   3. docs/ 根下不允许再出现编号资料包文件（0X_*.md）
#   4. docs/current/ 内不得出现商业词，除非同一行带豁免标记
#      （仅限说明“已移除/禁止复活”的上下文）
#   5. 根 README 必须链接 CURRENT_STATE / ROADMAP / archive
#   6. [M1-G4] 版本与发布卫生：
#      a. 根 VERSION 文件存在且 x.y.z 形态（唯一版本事实源）
#      b. 全仓禁止 v0.1.0-scaffold（旧脚手架默认值，archive 除外）
#      c. downloads/manifest.json 存在且 version 与 VERSION 文件一致
#      d. 固件源码不得再硬编码 FIRMWARE_VERSION（版本来自 VERSION→PROJECT_VER）
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
fails=0
note() { printf '  %s %s\n' "$1" "$2"; }
fail() { note "✗" "$1"; fails=$((fails+1)); }
pass() { note "✓" "$1"; }

# --- 1. current 头 ---
for f in "$ROOT"/docs/current/*.md; do
  [ -e "$f" ] || { fail "docs/current/ 为空"; break; }
  head -5 "$f" | grep -q "status: CURRENT" && head -5 "$f" | grep -q "authority: canonical" \
    && pass "current 头: $(basename "$f")" || fail "current 头缺失: $(basename "$f")"
done

# --- 2. archive 头 ---
arch_n=0
for f in "$ROOT"/docs/archive/*.md; do
  [ -e "$f" ] || break
  arch_n=$((arch_n+1))
  head -5 "$f" | grep -q "status: SUPERSEDED" \
    || fail "archive SUPERSEDED 头缺失: $(basename "$f")"
done
[ "$arch_n" -gt 0 ] && pass "archive SUPERSEDED 头: $arch_n 个文件" || fail "docs/archive/ 为空"

# --- 3. docs 根不得残留编号资料 ---
stray=$(ls "$ROOT"/docs/[0-9]*.md 2>/dev/null | wc -l | tr -d ' ')
[ "$stray" = "0" ] && pass "docs/ 根无编号资料残留" || fail "docs/ 根残留 $stray 个编号文件（应移入 archive/）"

# --- 4. current 文档商业词检查（行级豁免） ---
FORBIDDEN='smart hid cloud|smart-hid-cloud|[Tt]rial|[Ll]icen[cs]e|[Pp]ayment|[Cc]ommercial|[Ee]ntitlement|订单|付费|下单'
ALLOW='REMOVED|SUPERSEDED|DO NOT IMPLEMENT|已移除|已删除|已废弃|移除|删除|废弃|禁止|历史|archive|复活'
while IFS= read -r line; do
  f="${line%%:*}"; n="${line#*:}"; n="${n%%:*}"
  fail "商业词越界 $(basename "$f"):$n → ${line#*:*:}"
done < <(grep -rniE "$FORBIDDEN" "$ROOT"/docs/current/ 2>/dev/null | grep -viE "$ALLOW")
[ "$fails" -eq 0 ] && pass "current 文档无未豁免商业词" || true

# --- 5. README 链接 ---
for target in "docs/current/CURRENT_STATE.md" "docs/current/ROADMAP.md" "docs/archive/"; do
  grep -q "$target" "$ROOT/README.md" && pass "README 链接 $target" || fail "README 缺少链接 $target"
done

# --- 6a. VERSION 文件（唯一版本事实源） ---
if [ -f "$ROOT/VERSION" ]; then
  VER="$(tr -d ' \r\n' < "$ROOT/VERSION" | sed 's/^v//')"   # \r：Windows CRLF 检出兼容
  case "$VER" in
    [0-9]*.[0-9]*.[0-9]*) pass "VERSION 文件存在且形态合法（${VER}）" ;;
    *) fail "VERSION 内容「${VER}」不是 x.y.z" ;;
  esac
else
  VER=""
  fail "根 VERSION 文件缺失（唯一版本事实源，M1-G4）"
fi

# --- 6b. 禁止脚手架版本号残留（archive 历史资料除外） ---
scaffold_n=$(grep -rn 'v0\.1\.0-scaffold' "$ROOT" \
  --include='*.sh' --include='*.md' --include='*.go' --include='*.py' --include='*.yml' --include='*.yaml' 2>/dev/null \
  | grep -v '/docs/archive/' | grep -v '/build/' \
  | grep -v 'check-governance.sh' \
  | grep -v 'docs/current/HARDENING_BACKLOG.md' \
  | grep -v 'build-releases.sh:5:' \
  | wc -l | tr -d ' ')
[ "$scaffold_n" = "0" ] && pass "无 v0.1.0-scaffold 残留" || fail "v0.1.0-scaffold 残留 $scaffold_n 处（archive 外）"

# --- 6c. manifest 与 VERSION 一致 ---
MANIFEST="$ROOT/smart-hid-web/downloads/manifest.json"
if [ -f "$MANIFEST" ]; then
  if command -v python3 >/dev/null; then
    # Windows Git Bash 下 python3 是原生 Windows Python，打不开 MSYS 风格路径
    # （/e/...），需转换为盘符路径；用 -m（正斜杠 E:/...）避免反斜杠在
    # Python 字符串里被当作转义（\x \f 等）。Linux/macOS 无 cygpath 原样传递。
    MANIFEST_PY="$MANIFEST"
    command -v cygpath >/dev/null && MANIFEST_PY="$(cygpath -m "$MANIFEST")"
    mv="$(python3 -c 'import json;print(json.load(open("'"$MANIFEST_PY"'"))["version"])' 2>/dev/null || echo ERR)"
    mv="${mv%%$'\r'}"   # Windows python print 尾随 CR 会让字符串比较恒不等
    if [ "$mv" = "$VER" ]; then
      pass "manifest.json version 与 VERSION 一致（${mv}）"
    else
      fail "manifest.json version=$mv ≠ VERSION=${VER}（重跑 build-releases.sh）"
    fi
  else
    note "-" "跳过 manifest 校验（无 python3）"
  fi
else
  fail "downloads/manifest.json 缺失（release 产物必须带 manifest，M1-G4）"
fi

# --- 6d. 固件不得硬编码版本 ---
fwver_n=$(grep -rn 'FIRMWARE_VERSION' "$ROOT/smart-hid-firmware/components" "$ROOT/smart-hid-firmware/main" 2>/dev/null | wc -l | tr -d ' ')
[ "$fwver_n" = "0" ] && pass "固件无硬编码 FIRMWARE_VERSION（版本经 PROJECT_VER 注入）" \
  || fail "固件仍硬编码 FIRMWARE_VERSION $fwver_n 处"

echo
if [ "$fails" -gt 0 ]; then
  echo "GOVERNANCE CHECK: FAIL（$fails 项）"
  exit 1
fi
echo "GOVERNANCE CHECK: PASS"
