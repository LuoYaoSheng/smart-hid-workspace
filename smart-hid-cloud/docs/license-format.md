# Smart HID License 格式 — 占位

> 事实源：本文件（待补全）。依据：`docs/01_PRODUCT_PRD.md` §7、`docs/05_CONTROLHUB_DETAIL_DESIGN_V1.0.md` §8。

## 状态

⚠️ 脚手架占位。Phase 6（Commercial）里程碑落地时补全完整字段定义、签名算法、验签流程。

## 设计要点（来自资料包）

- 云端 Ed25519 签发，ControlHub 本地验签
- ControlHub 只内置 Public Key，Cloud 持有 Private Key
- 主绑定对象：ESP32 Device ID
- 支持在线刷新与离线导入

## 推荐载荷字段（来自 PRD §7）

```text
license_id
account_id
plan_id
device_id        # 主绑定 ESP32 Device ID
issued_at
valid_from
expires_at
features
license_version
signature        # Ed25519
```

## 待补全

- [ ] 完整 JSON Schema
- [ ] 签名输入串构造规则（canonicalization）
- [ ] Public Key 分发与轮换策略
- [ ] 离线导入文件格式（.dat）
- [ ] 续费 / 刷新协议
