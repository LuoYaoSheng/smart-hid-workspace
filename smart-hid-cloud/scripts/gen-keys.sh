#!/usr/bin/env bash
# 生成 Smart HID Cloud License 签名 keypair（CL-1）。
#
# 输出：
#   keys/private.key  — Ed25519 私钥（hex，mode 0600，git ignored）
#   keys/public.hex   — 公钥 hex（嵌入 ControlHub binary 用）
#
# 已存在则不覆盖（避免意外轮换 key 让历史 license 失效）。

set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p keys

if [[ -f keys/private.key ]]; then
  echo "keys/private.key 已存在，跳过生成（避免轮换让历史 license 失效）。"
  echo "如确需重新生成，先备份旧的 keys/private.key 再删除。"
  exit 0
fi

go run ./cmd/gen-keys ./keys/private.key ./keys/public.hex

echo ""
echo "✔ keys/private.key（已在 .gitignore，勿提交）"
echo "✔ keys/public.hex（用于 ControlHub embed，可提交作参考）"
