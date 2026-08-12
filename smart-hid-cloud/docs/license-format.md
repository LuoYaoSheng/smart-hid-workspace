# Smart HID License 格式规范

> 事实源：本文件。实现：`smart-hid-cloud/pkg/license/`。
> 依据：`docs/01_PRODUCT_PRD.md` §7、`docs/05_CONTROLHUB_DETAIL_DESIGN_V1.0.md` §8、`docs/10_ACCEPTANCE_CHECKLIST.md` §E。

## 1. 概述

- 云端（smart-hid-cloud）用 **Ed25519** 私钥签发 License。
- ControlHub **只内置公钥**，本地离线验签。
- 私钥不下发；公钥 embed 进 ControlHub binary（`internal/license/publickey.go`）。
- 主绑定对象：**ESP32 Device ID**（`HID-XXXXXXXX`）。
- 支持在线激活、刷新、离线导入。

## 2. 载荷（Payload）

完整 License = `Payload` + `signature`。

```json
{
  "license_id":      "lic_<22hex>",
  "account_id":      "acc_<22hex>",
  "plan_id":         "plan_basic_yearly",
  "device_id":       "HID-AAAA1111",
  "issued_at":       1723440000,
  "valid_from":      1723440000,
  "expires_at":      1754976000,
  "features":        ["hid_control"],
  "license_version": 1
}
```

字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `license_id` | string | 唯一 ID，前缀 `lic_` + 22 hex（11 随机字节）。数据库 PK 与 payload 字段一致。 |
| `account_id` | string | 用户 ID（Cloud.users.user_id），前缀 `acc_`。 |
| `plan_id` | string | 套餐 ID（如 `plan_basic_yearly`），决定 `duration_days`。 |
| `device_id` | string | ESP32 device_id，正则 `^HID-[A-Z0-9]{8}$`。主绑定对象。 |
| `issued_at` | int64 | 签发 Unix 秒（激活时刻）。 |
| `valid_from` | int64 | 生效 Unix 秒（通常等于 issued_at）。 |
| `expires_at` | int64 | 过期 Unix 秒（issued_at + plan.duration_days * 86400）。 |
| `features` | string[] | 授权功能列表（来自 plan.features_json）。 |
| `license_version` | int | 载荷版本，当前固定 1。验签时必须匹配。 |

## 3. Canonicalization（签名输入构造）

签名输入 = Canonical(Payload)。

实现（`pkg/license/Canonical`）：
1. `json.Marshal(Payload)` → 得到 struct 顺序的 JSON
2. 解码为 `map[string]any`（`json.Decoder.UseNumber` 保留整数类型）
3. 重新 `json.Marshal(map)` —— Go 标准库对 map 输出**按 key 字典序**，结果稳定可重现
4. 输出为紧凑 JSON（无缩进空格）

属性：
- **可重现**：相同 Payload 永远产生相同 canonical bytes
- **跨实现兼容**：Python/Node 等用 `json.dumps(payload, sort_keys=True, separators=(',', ':'))` 产生等价输出
- **抗字段顺序攻击**：攻击者无法通过重排 JSON 字段绕过验签

## 4. 签名

- 算法：**Ed25519**（RFC 8032）
- 私钥：64 字节（seed 32 + pub 32），hex 编码存 `smart-hid-cloud/keys/private.key`（git ignored）
- 公钥：32 字节，hex 字符串 embed 进 ControlHub binary
- 签名输出：64 字节，`base64.StdEncoding.EncodeToString` 编码为字符串

## 5. License 文件格式

完整 License JSON：

```json
{
  "payload": { ... },
  "signature": "<base64 64-byte Ed25519>"
}
```

- 编码：`json.MarshalIndent(license, "", "  ")`（2 空格缩进，便于人读）
- 扩展名：`.license`
- Content-Type：`application/json`
- Content-Disposition（下载时）：`attachment; filename="<license_id>.license"`

## 6. 验签规则（ControlHub）

`license.VerifyFull(license, publicKey, expectedDeviceID, now)`：

1. **版本检查**：`license.payload.license_version == 1`，否则 `ErrVersion`
2. **签名验证**：`ed25519.Verify(publicKey, Canonical(payload), base64Decode(signature))`，失败 `ErrInvalidSig`
3. **设备绑定**：`payload.device_id == expectedDeviceID`（除非 `expectedDeviceID==""` 调试场景），否则 `ErrWrongDevice`
4. **生效检查**：`now >= payload.valid_from`，否则 `ErrFutureStart`
5. **过期检查**：`now <= payload.expires_at`，否则 `ErrExpired`

任一失败 → 拒绝；全部通过 → License 有效。

## 7. 离线导入（CL-3b ControlHub 实装）

ControlHub `POST /api/v1/license/import`（body = 完整 License JSON）：
1. Decode body → License
2. VerifyFull(license, embeddedPublicKey, thisDeviceID, now)
3. 通过 → 存入 ControlHub `licenses` 表 + 解锁 Entitlement
4. 失败 → 400 + 错误原因

适用场景：ControlHub 在离线机器上，用户从手机/其他电脑下载 .license 文件后拷贝导入。

## 8. 公钥分发与轮换策略（V1 简化）

- **V1**：单一公钥，embed 进 binary。无 key_id 字段。
- **轮换**：V1 不支持。若私钥泄漏，需发新版 ControlHub + 重签所有 License。
- **V2（未规划）**：加 `key_id` 字段，ControlHub 持有多组公钥，支持平滑轮换。

## 9. 续费 / 刷新协议（CL-6 实装）

两种 License 获取路径，均产出同一签名格式：

**A. 激活码在线激活（CL-6 新增）**
- admin 在后台生成激活码（预创建 UNUSED License + 12 字符 Crockford base32 码）
- ControlHub 输入码 → `POST /api/v1/activation/consume {code, device_id}`（PUBLIC，码即凭据）
  → Cloud 校验码（未用/未过期/设备绑定）→ 签发并激活 → 返回签名 License JSON
- ControlHub `Import`（Decode + VerifyFull + upsert）→ 本地生效
- 设备绑定双模式：码生成时 device_id 非空 → 消费必须匹配；为空（通用码）→ 消费时绑定

**B. 在线刷新（续期，CL-6 实装；原 §9 设计落地）**
- admin `POST /admin/licenses/{id}/extend {add_days}` → 同 license_id 重签延长 expires_at（不新建 id）
- ControlHub `POST /api/v1/license/refresh {license_id}`（PUBLIC，license_id 即凭据）
  → Cloud 返回最新签名 License JSON → ControlHub 覆盖导入
- ControlHub 后台自动刷新循环：启动后 + 每 6h best-effort 拉取全部本地 License；离线降级不中断
- 设备绑定由签名强制：返回的 License 内嵌 device_id，`VerifyFull` 校验，换设备无法冒用

**鉴权说明**：consume/refresh 是 PUBLIC endpoint（不走 JWT）。
- consume 凭 12 字符激活码（32^12 ≈ 10^18 空间，暴力不可行；类比产品密钥）
- refresh 凭不可猜测的 license_id（lic_+22hex）；只读返回已属权 License
- 设备绑定由签名强制，Phase 7 生产安全再上设备证书 / CRL 实时吊销

## 10. 安全注意事项

- ❌ 私钥绝不离开 smart-hid-cloud 服务器
- ❌ ControlHub binary 不含私钥
- ❌ License 文件不含私钥
- ✅ 公钥可公开（仅用于验签，泄露无害）
- ✅ License 文件可在用户间传递，但因绑定 device_id，他人拿到也无法用于自己的设备

## 11. 测试覆盖

`pkg/license/license_test.go` 14 用例覆盖：
HappyPath / TamperedPayload / WrongPublicKey / Expired / FutureStart /
WrongDevice / AllowAnyDevice / BadVersion / EncodeDecode / Base64 /
Canonical-Stable / Canonical-Distinct / SaveLoad / NewPayload / PublicFromPrivate。

`internal/api/handlers_test.go` 含完整 e2e：
注册 → 登录 → /me → 套餐 → 注册设备 → 订单 → Mock 支付 → 激活 →
下载 .license → pkg/license.VerifyFull 通过 → 错设备验签拒绝。
