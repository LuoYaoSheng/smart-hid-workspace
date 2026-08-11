#!/usr/bin/env bash
# Smart HID ControlHub Phase 1 端到端验证脚本
#
# 验收标准（docs/09_LOCAL_DEVELOPMENT_ROADMAP.md Phase 1）：
#   curl → ControlHub HTTP → MQTT → ESP32(mock) → USB HID(模拟) → ACK
#
# 用法：
#   ./scripts/test-loop.sh
#
# 退出码：0 全部通过；非 0 某步失败。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CONTROLHUB="${ROOT}/bin/controlhub"
MOCK_DEVICE="${ROOT}/bin/mock-device"
API_BASE="http://127.0.0.1:17890/api/v1"
DEVICE_ID="HID-00000001"

# 清理：确保退出时杀掉后台进程
CONTROLHUB_PID=""
MOCK_PID=""
cleanup() {
  [ -n "$MOCK_PID" ] && kill "$MOCK_PID" 2>/dev/null || true
  [ -n "$CONTROLHUB_PID" ] && kill "$CONTROLHUB_PID" 2>/dev/null || true
  sleep 1
}
trap cleanup EXIT

echo "=== Phase 1 端到端验证 ==="
echo ""

# 0. 确保二进制存在
if [ ! -x "$CONTROLHUB" ] || [ ! -x "$MOCK_DEVICE" ]; then
  echo "[build] 二进制缺失，执行 go build..."
  go build -o bin/controlhub ./cmd/controlhub
  go build -o bin/mock-device ./cmd/mock-device
fi

# 1. 启动 ControlHub（后台）
echo "[1/6] 启动 ControlHub..."
rm -rf "${ROOT}/data"  # 干净开始
"$CONTROLHUB" > /tmp/controlhub.log 2>&1 &
CONTROLHUB_PID=$!
sleep 2

# 检查进程存活
if ! kill -0 "$CONTROLHUB_PID" 2>/dev/null; then
  echo "  ❌ ControlHub 启动失败，日志："
  cat /tmp/controlhub.log
  exit 1
fi

# 从日志提取 API Key
API_KEY=$(grep -oE 'chk_[a-f0-9]+' /tmp/controlhub.log | head -1)
if [ -z "$API_KEY" ]; then
  echo "  ❌ 未能从日志提取 API Key"
  cat /tmp/controlhub.log
  exit 1
fi
echo "  ✓ ControlHub 运行中 (PID=$CONTROLHUB_PID, API_KEY=${API_KEY:0:12}...)"

# 2. 健康检查（无鉴权）
echo "[2/6] 健康检查 GET /health..."
HEALTH=$(curl -s -w "\n%{http_code}" "${API_BASE}/health")
HEALTH_BODY=$(echo "$HEALTH" | head -1)
HEALTH_CODE=$(echo "$HEALTH" | tail -1)
if [ "$HEALTH_CODE" != "200" ]; then
  echo "  ❌ 健康检查失败 HTTP $HEALTH_CODE: $HEALTH_BODY"
  exit 1
fi
echo "  ✓ HTTP 200: $HEALTH_BODY"

# 3. 启动 mock-device（后台）
echo "[3/6] 启动 mock-device..."
"$MOCK_DEVICE" --device-id "$DEVICE_ID" > /tmp/mock-device.log 2>&1 &
MOCK_PID=$!
sleep 2

if ! kill -0 "$MOCK_PID" 2>/dev/null; then
  echo "  ❌ mock-device 启动失败，日志："
  cat /tmp/mock-device.log
  exit 1
fi
echo "  ✓ mock-device 运行中 (PID=$MOCK_PID, device_id=$DEVICE_ID)"

# 等待设备上线（poll /devices 直到 device 出现且 online）
echo "  等待设备上线..."
for i in $(seq 1 10); do
  DEVS=$(curl -s -H "Authorization: Bearer $API_KEY" "${API_BASE}/devices")
  if echo "$DEVS" | grep -q "\"device_id\":\"${DEVICE_ID}\""; then
    echo "  ✓ 设备已注册: $DEVS"
    break
  fi
  sleep 1
  if [ $i -eq 10 ]; then
    echo "  ❌ 设备 10s 内未上线"
    echo "  mock 日志: $(tail -5 /tmp/mock-device.log)"
    echo "  controlhub 日志: $(tail -5 /tmp/controlhub.log)"
    exit 1
  fi
done

# 4. 从 device 列表拿 boot_id（命令要 target_boot_id 匹配）
BOOT_ID=$(echo "$DEVS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(next(x['boot_id'] for x in d['devices'] if x['device_id']=='${DEVICE_ID}'))")
echo "  设备 boot_id=$BOOT_ID"

# 5. 发送 ENTER 键命令（核心验收）
echo "[4/6] 发送 keyboard tap ENTER 命令..."
REQ_ID="test_$(date +%s)"
CMD_BODY=$(cat <<EOF
{"protocol":"1.0","request_id":"${REQ_ID}","device_id":"${DEVICE_ID}","target_boot_id":"${BOOT_ID}","type":"keyboard","action":"tap","ttl_ms":3000,"payload":{"key":"ENTER","hold_ms":40}}
EOF
)
RESP=$(curl -s -w "\n%{http_code}" -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "$CMD_BODY" \
  "${API_BASE}/devices/${DEVICE_ID}/commands")
RESP_BODY=$(echo "$RESP" | head -1)
RESP_CODE=$(echo "$RESP" | tail -1)
echo "  HTTP $RESP_CODE: $RESP_BODY"

if [ "$RESP_CODE" != "200" ]; then
  echo "  ❌ 期望 HTTP 200（executed），实际 $RESP_CODE"
  exit 1
fi

# 检查 status=executed
STATUS=$(echo "$RESP_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")
if [ "$STATUS" != "executed" ]; then
  echo "  ❌ 期望 status=executed，实际 $STATUS"
  exit 1
fi
echo "  ✓ 命令执行成功：status=executed"

# 6. 查询命令状态（GET /commands/{request_id}）
echo "[5/6] 查询命令状态 GET /commands/${REQ_ID}..."
QUERY=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer $API_KEY" \
  "${API_BASE}/commands/${REQ_ID}")
QUERY_BODY=$(echo "$QUERY" | head -1)
QUERY_CODE=$(echo "$QUERY" | tail -1)
if [ "$QUERY_CODE" != "200" ]; then
  echo "  ❌ 查询失败 HTTP $QUERY_CODE: $QUERY_BODY"
  exit 1
fi
echo "  ✓ HTTP 200: $QUERY_BODY"

# 7. 鉴权失败验证（错误 API Key 应 401）
echo "[6/6] 鉴权失败验证（错误 API Key → 401）..."
BAD=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer wrong_key" \
  "${API_BASE}/devices")
if [ "$BAD" != "401" ]; then
  echo "  ❌ 期望 401，实际 $BAD"
  exit 1
fi
echo "  ✓ 错误 Key 正确返回 401"

echo ""
echo "=== ✅ Phase 1 端到端验证全部通过 ==="
echo ""
echo "验收链路：curl → ControlHub HTTP → MQTT → mock-device → USB HID(模拟) → ACK"
echo ""
echo "ControlHub 日志（最后 10 行）："
tail -10 /tmp/controlhub.log
echo ""
echo "mock-device 日志（最后 10 行）："
tail -10 /tmp/mock-device.log
