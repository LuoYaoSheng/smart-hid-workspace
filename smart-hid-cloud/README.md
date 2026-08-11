# smart-hid-cloud

Smart HID 云端。账号 / 套餐 / 订单 / 支付 / License 签发。

## 角色定位

```text
Smart HID Web ──HTTPS──▶ Smart HID Cloud
ControlHub    ──激活/刷新──▶ Smart HID Cloud
```

> **Cloud 不在实时 HID 控制链路。** 实时控制本地优先，无云环境已有有效 License 可继续工作。

## V1 模块

来自资料包 `starter/cloud`：

```text
auth
user
device
plan
order
payment
license
activation
signer
```

## License

- 云端 Ed25519 签发，ControlHub 本地验签（只内置 Public Key）
- Private Key 不下发
- 主绑定对象：ESP32 Device ID
- 支持在线刷新与离线导入

推荐 License 载荷字段：

```text
license_id
account_id
plan_id
device_id
issued_at
valid_from
expires_at
features
license_version
signature
```

## 购买与激活流程

```text
购买套餐 → 获得 UNUSED License → 选择 Device → 激活
→ 云端签发 → ControlHub 下载/导入 → 本地验签
```

购买与绑定分离。

## 当前状态

⚠️ **脚手架阶段**。仅有目录骨架与文档落位，未实现任何功能代码。

## 相关

- License 格式事实源：`./docs/license-format.md`（占位）
- 验收清单：`../docs/10_ACCEPTANCE_CHECKLIST.md` §E
