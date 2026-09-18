#!/usr/bin/env bash
# 同步 smart-hid-web/ → gh-pages 分支，GitHub Pages（Source = Deploy from a
# branch: gh-pages）随之自动重建。
#
# 背景（2026-09-18）：deploy-pages 路线在分支模式下 0 步骤秒败（docs.yml 两次
# 失败、无任何 step 启动），而线上实际服务的一直是 gh-pages 分支内容——改为
# 直接推送分支，与仓库设置自洽，无需人工切 Source。
#
# 用法：bash scripts/sync-gh-pages.sh [repo_root] [remote]
#   repo_root 缺省为脚本上上级目录
#   remote    缺省 origin（CI 检出即 GITHUB_TOKEN；本地双远端时传 github）
# 提交身份可用 GIT_DEPLOY_NAME / GIT_DEPLOY_EMAIL 覆盖，缺省 github-actions[bot]。
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
REMOTE="${2:-origin}"
SRC_DIR="smart-hid-web"

[ -d "$ROOT/$SRC_DIR" ] || { echo "ERROR: $ROOT/$SRC_DIR 不存在" >&2; exit 1; }

DEPLOY="$(mktemp -d)"
trap 'cd "$ROOT" && git worktree remove --force "$DEPLOY" 2>/dev/null || rm -rf "$DEPLOY"' EXIT

git -C "$ROOT" fetch "$REMOTE" gh-pages
git -C "$ROOT" worktree add --detach "$DEPLOY" FETCH_HEAD

# 整树替换（而非增量拷贝）：已下线的资产必须从站点消失，防目录漂移
find "$DEPLOY" -mindepth 1 -maxdepth 1 -not -name .git -exec rm -rf {} +
cp -a "$ROOT/$SRC_DIR/." "$DEPLOY/"
touch "$DEPLOY/.nojekyll"   # 分支模式默认过 Jekyll，关掉以免下划线目录被吞

git -C "$DEPLOY" config user.name  "${GIT_DEPLOY_NAME:-github-actions[bot]}"
git -C "$DEPLOY" config user.email "${GIT_DEPLOY_EMAIL:-41898282+github-actions[bot]@users.noreply.github.com}"
SRC_SHA="$(git -C "$ROOT" rev-parse --short HEAD)"

git -C "$DEPLOY" add -A
if git -C "$DEPLOY" diff --cached --quiet; then
  echo "gh-pages 与 $SRC_DIR 一致（源 $SRC_SHA），无需推送"
  exit 0
fi
git -C "$DEPLOY" commit -m "deploy: 官网同步 $SRC_SHA"
git -C "$DEPLOY" push "$REMOTE" HEAD:gh-pages
echo "gh-pages 已更新（源 $SRC_SHA）"
