#!/usr/bin/env python3
"""validate-protocols.py — Protocol / OpenAPI Gate（M1-G4）

职责（轻量、无重型工具链依赖，仅 jsonschema + pyyaml）：
  1. protocols/examples/*.json 逐一校验对应 protocols/schemas/*.schema.json
  2. smart-hid-controlhub/docs/openapi.yaml 结构校验：
     - 可被 YAML 解析
     - openapi/info(title,version)/paths 齐备，每个 path+method 有 responses
     - components.schemas 非空
     - openapi 里内嵌 JSON example 全部是合法 JSON
  3. openapi 投影一致性：smart-hid-web/api/openapi.yaml 与事实源逐字节一致

退出码 0 = 全部通过；非 0 = 有失败（CI gate）。
"""
import json
import pathlib
import sys

import jsonschema
import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
SCHEMAS = ROOT / "protocols" / "schemas"
EXAMPLES = ROOT / "protocols" / "examples"
OPENAPI = ROOT / "smart-hid-controlhub" / "docs" / "openapi.yaml"
OPENAPI_PROJECTION = ROOT / "smart-hid-web" / "api" / "openapi.yaml"

failures = []


def fail(msg: str) -> None:
    failures.append(msg)
    print(f"    FAIL: {msg}")


def ok(msg: str) -> None:
    print(f"  ✓ {msg}")


# ---------- 1) examples vs schemas ----------
# 文件名前缀 ↔ schema 映射（keyboard_tap.json → command.schema.json 等）
PREFIX_TO_SCHEMA = {
    "keyboard": "command.schema.json",
    "mouse": "command.schema.json",
    "ack": "ack.schema.json",
    "status": "status.schema.json",
}

schema_cache: dict = {}
checked = 0
for ex in sorted(EXAMPLES.glob("*.json")):
    prefix = ex.name.split("_")[0]
    schema_name = PREFIX_TO_SCHEMA.get(prefix)
    if schema_name is None:
        fail(f"{ex.name}: 无 schema 映射（请更新 PREFIX_TO_SCHEMA）")
        continue
    if schema_name not in schema_cache:
        schema_cache[schema_name] = json.loads((SCHEMAS / schema_name).read_text())
    schema = schema_cache[schema_name]
    try:
        instance = json.loads(ex.read_text())
        jsonschema.validate(instance=instance, schema=schema)
        checked += 1
    except json.JSONDecodeError as e:
        fail(f"{ex.name}: 非法 JSON（{e}）")
    except jsonschema.ValidationError as e:
        fail(f"{ex.name}: 不符合 {schema_name}（{e.message}）")
if checked:
    ok(f"examples×schema 校验 {checked} 个全过")

# ---------- 2) OpenAPI 结构 ----------
try:
    doc = yaml.safe_load(OPENAPI.read_text())
except Exception as e:  # noqa: BLE001
    print(f"    FAIL: openapi.yaml 解析失败：{e}")
    sys.exit(1)

if not isinstance(doc.get("openapi"), str):
    fail("openapi 字段缺失")
if not (doc.get("info", {}) or {}).get("title") or not doc["info"].get("version"):
    fail("info.title / info.version 缺失")
paths = doc.get("paths") or {}
if not paths:
    fail("paths 为空")
n_ops = 0
for path, item in paths.items():
    if not isinstance(item, dict):
        continue
    for method, op in item.items():
        if method not in ("get", "post", "put", "delete", "patch"):
            continue
        n_ops += 1
        if not (isinstance(op, dict) and op.get("responses")):
            fail(f"{method.upper()} {path}: 缺 responses")
if n_ops:
    ok(f"openapi 结构校验 {len(paths)} path / {n_ops} operation")
if not (doc.get("components", {}) or {}).get("schemas"):
    fail("components.schemas 为空")

# 内嵌 example 必须是合法 JSON（yaml 已把 {a: b} 当 map 解析，这里校验序列化回环）
def walk_examples(node, parent_key, where):
    if isinstance(node, dict):
        # 仅 request/response 的 media-type object（父键是 content）下的字符串
        # example 要求合法 JSON；parameter 等处的纯字符串 example 不误报。
        if parent_key == "content" and "example" in node and isinstance(node["example"], str):
            try:
                json.loads(node["example"])
            except json.JSONDecodeError as e:
                fail(f"{where}: example 非法 JSON（{e}）")
        for k, v in node.items():
            walk_examples(v, k, where)
    elif isinstance(node, list):
        for i, v in enumerate(node):
            walk_examples(v, parent_key, where)


walk_examples(doc, "", "openapi")

# ---------- 3) 投影一致性 ----------
if OPENAPI_PROJECTION.exists():
    if OPENAPI_PROJECTION.read_bytes() == OPENAPI.read_bytes():
        ok("openapi 投影一致（smart-hid-web/api/）")
    else:
        fail("openapi 投影漂移：smart-hid-web/api/openapi.yaml ≠ 事实源（跑 build-releases.sh 同步）")
else:
    print("  - 跳过投影检查（smart-hid-web/api/openapi.yaml 不存在）")

print()
if failures:
    print(f"PROTOCOL/OPENAPI GATE: FAIL（{len(failures)} 项）")
    sys.exit(1)
print("PROTOCOL/OPENAPI GATE: PASS")
