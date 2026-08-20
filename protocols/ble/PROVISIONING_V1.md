# Smart HID BLE Provisioning Protocol V1

> `status: CURRENT` · `authority: canonical`（固件 `components/ble_provision/` 与
> 小程序客户端共同遵守；历史推演见 `docs/archive/03`，SUPERSEDED，仅溯源）
>
> BLE 只负责：设备发现 / 配网 / 状态查询。**HID 实时控制不走 BLE**（走
> ControlHub HTTP → MQTT）。

## 1. 概览

```text
BLE Toolkit+（小程序，独立仓 smart-ble）
  → 扫描 Provisioning Service UUID
  → 连接（Just Works 配对加密）
  → 读 Device Info
  → 扫 ControlHub 动态 Pairing QR（shid://pair?token=…&host=…&port=…）
  → 分帧写入 Provision Input（Wi-Fi + hub + token）
  → 订阅 Provision Status
  → 设备：连 Wi-Fi → POST hub pairing → 拿 MQTT 凭据 → NVS commit → MQTT → READY
```

配对 QR 载荷（ControlHub `POST /api/v1/pairing/sessions` 的 `qr_payload`）：

```text
shid://pair?token=<32hex>&host=<hub-lan-ip>&port=<17892>
```

## 2. GATT 结构

| 项 | UUID | 权限 |
|---|---|---|
| Provisioning Service | `9f1d1001-e73b-4c8f-9d2a-6f0b5e8a1c04` | — |
| Device Info 特征 | `9f1d1002-e73b-4c8f-9d2a-6f0b5e8a1c04` | read + notify |
| Provision Input 特征 | `9f1d1003-e73b-4c8f-9d2a-6f0b5e8a1c04` | **write（要求加密链路）** |
| Provision Status 特征 | `9f1d1004-e73b-4c8f-9d2a-6f0b5e8a1c04` | read + notify |

广播：ADV 包含 128-bit Service UUID（可据此过滤扫描）；Scan Response 携带
设备名 `SHID-XXXXXXXX`（= device_id 的 8 位尾码）。

## 3. 分帧（Provision Input）

Wi-Fi 密码 + token + host 的 JSON 超过单个默认 ATT payload，**必须分帧**：

```text
帧 = [seq:u8][total:u8][len:u8][payload:len 字节]
```

- `seq` 从 0 起，顺序递增；`total` = 总块数（1–64）
- 每块 payload ≤ **128 字节**；客户端按协商 MTU 切块（MTU 23 时每块 ≤17B
  同样合法——设备端不假设单包成功）
- 收到 `seq=0` 即视为新传输开始（客户端出错后可直接从 0 重发）
- 乱序/跳号 → 设备丢弃并报错，客户端从 `seq=0` 重发
- 组装上限 1024 字节，超限报错

推荐客户端 `writeMTU ≥ 185`（微信小程序 `setMTU`）后按 `MTU-3-3` 切块。

## 4. Provision Input（组装后的 JSON）

```json
{
  "v": 1,
  "wifi_ssid": "home-net",
  "wifi_password": "pass1234",
  "hub_host": "192.168.1.8",
  "hub_port": 17892,
  "token": "0123456789abcdef0123456789abcdef"
}
```

- `v`：协议版本，当前仅 `1`（其他值设备拒绝）
- `hub_port` 可省略，默认 `17892`
- `token`：ControlHub pairing session 一次性 token（5 分钟）
- 设备不持久化 token 到 active 配置（一次性数据）

## 5. Device Info（读 / notify）

```json
{
  "product": "smart-hid",
  "protocol": "1.0",
  "device_id": "HID-ABCD1234",
  "firmware": "1.1.0",
  "state": "provisioning",
  "provisioned": false
}
```

`state` 取值见 §6；`provisioned` = 存在完整 active 运行时配置。

## 6. Provision Status（读 / notify）

```json
{ "state": "connecting_wifi", "step": "connecting_wifi", "error": null }
{ "state": "provisioning", "step": "wifi_failed", "error": "wifi_failed" }
```

状态机（`state`）：

```text
boot / load_config / unprovisioned / provisioning / connecting_wifi /
pairing / mqtt_connecting / ready / recovery / error
```

过渡步骤（`step`）：

```text
received / connecting_wifi / wifi_connected / pairing / pairing_success /
mqtt_connecting / ready
```

错误码（`error`，**稳定勿改**）：

| 错误码 | 含义 | 客户端提示 |
|---|---|---|
| `invalid_payload` | candidate JSON/字段非法 | 检查输入 |
| `wifi_failed` | Wi-Fi 连不上 | 检查 SSID/密码 |
| `controlhub_unreachable` | pairing 端点不可达（含 5xx/503） | 检查 hub 地址 / ControlHub 是否在运行 |
| `pairing_invalid` | HTTP 404 token 不存在 | 重新扫码 |
| `pairing_expired` | HTTP 410 token 过期 | 重新扫码 |
| `pairing_used` | HTTP 409 token 已消费 | 重新扫码 |
| `mqtt_invalid` | MQTT 连接失败（凭据/端点） | 进入诊断 |
| `storage_failed` | NVS 写失败 / 配置版本未知 | 重试或联系支持 |

恢复原则（与 archive/03 一致，仍有效）：Wi-Fi 失败只退回 Wi-Fi；token 失效只
重新扫码；MQTT 失败进诊断不重做 Wi-Fi；小程序切后台恢复后重连并读 Status。

## 7. 配网安全模型（如实声明）

- Provision Input 特征要求**加密链路**：bonding + LE Secure Connections，
  Just Works（设备无显示/键盘，IO capability = NoInputNoOutput）
- **Just Works ≠ MITM 抗性**：配对瞬间在场的攻击者理论上可介入。这是 V1 的
  已知取舍，不虚构 production secure
- 真正的设备身份根（出厂 Setup Code / QR 制造身份 / Secure Boot / Flash
  Encryption / NVS Encryption）属于后续 Production Security（M2-G3）
- 配网完成后建议关闭 BLE 广播（设备 READY 即停广播）

## 8. 设备侧状态机与崩溃恢复

状态机与 Active/Pending 配置模型的事实源：
`smart-hid-firmware/components/provisioning/`（host 单测
`test/host/test_provisioning.c` 覆盖崩溃边界）。

要点（详见组件头文件注释）：

- candidate 先 stage 到 NVS `pending`，**成功才 promote 为 active**——配网
  失败绝不破坏旧配置
- pairing 成功后凭据**先持久化**（pending.complete=1）再连 MQTT：掉电重启
  时 complete pending 自动 promote（token 已在服务端消费，凭据已落盘，
  这是唯一无死状态的恢复路径）
- active 持续连不上（Wi-Fi 改密 / hub 迁移）→ RECOVERY：BLE Provision 重新
  可见，允许重配；**只有新 pairing 成功才替换 active**

## 9. 与 ControlHub 的关系

- 设备**不自己猜** MQTT broker 地址：pairing 响应的 `mqtt_host` 是唯一来源
  （ControlHub 按 G3 网络模型解析的 advertised 地址，见
  `docs/current/ARCHITECTURE.md`）
- 配对 API 契约：`smart-hid-controlhub/docs/openapi.yaml`
  （`POST /api/v1/pairing/device`）
