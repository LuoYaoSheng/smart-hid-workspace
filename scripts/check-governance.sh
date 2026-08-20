#!/usr/bin/env bash
# check-governance.sh — 仓库治理守卫（M1-G1 建立）
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

echo
if [ "$fails" -gt 0 ]; then
  echo "GOVERNANCE CHECK: FAIL（$fails 项）"
  exit 1
fi
echo "GOVERNANCE CHECK: PASS"
