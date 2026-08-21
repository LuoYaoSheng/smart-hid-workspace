---
status: CURRENT
authority: canonical
---

# CURRENT_STATE — 当前事实状态

> 本文是 Smart HID「现在是什么 / 不是什么 / 完成到哪里」的**唯一权威事实源**。
> 与代码冲突时，以 main 分支代码为准，并立即更新本文。
> 历史设计资料在 `docs/archive/`，不得作为当前实现依据。

最后核对：2026-08-20，M1-G4 完成后（基线见 git log M1-G4*）。

## 产品定位

主链路（已实现）：

```text
第三方程序
  → HTTP
  → ControlHub（本机，Go）
  → MQTT（内嵌 broker，仅局域网）
  → ESP32-S3
  → USB HID
  → 目标电脑
```

辅助链路（固件源码已实现，**未经真机验证**）：

```text
BLE Toolkit+ 微信小程序（独立仓库 smart-ble）
  → BLE Provision（NimBLE，协议 protocols/ble/PROVISIONING_V1.md）
  → ESP32-S3（配网 / 诊断）
  → ControlHub Pairing（:17892）→ per-device MQTT 凭据
  → NVS 持久化（active/pending 双配置）→ MQTT → READY
```

诚实状态：固件侧 BLE Provision / NVS 运行时配置 / 配网状态机源码已完成并
通过 ESP-IDF v5.4.4 编译与 host 状态机单测（36 项）；**从未在真实 ESP32-S3
上烧录验证**。小程序侧（smart-ble 仓）客户端适配仍待对齐 canonical 协议。

## 当前不是什么（REMOVED — 禁止恢复）

以下能力已于 2026-08（OS-1 开源瘦身）从代码库**移除**，状态 SUPERSEDED /
DO NOT IMPLEMENT：

| 已移除 | 处置 |
|---|---|
| smart-hid-cloud | 云端整目录已删除（账号 / 订单 / 支付 / License 签发） |
| Trial / License / Entitlement 门禁 | ControlHub 内部模块、402 路径、用量统计全部删除 |
| Usage / License 查询 API | openapi 从 17 path 缩减到 11（已移除） |
| Web Portal / Admin 商业前端 | 页面已删除，官网转纯静态作品集 |
| 小程序商业中心 | 早期设计已废弃（见 archive/11） |

保留途径：git 历史 + 本地 `commercial-backup` 分支（未推送）。除非用户明确重新立项，
**任何情况下不得恢复或重新实现**（见 [DEVELOPMENT_RULES](DEVELOPMENT_RULES.md) §2）。

## 组件状态

状态值语义：`IMPLEMENTED` 已实现且验证 ／ `PARTIAL` 部分实现 ／ `PLANNED` 仅规划
无代码 ／ `NOT VERIFIED ON HARDWARE` 源码完成但未经真机 ／ `SUPERSEDED` 已废弃。
禁止「差不多完成 / 基本完成」等模糊表述。

| 组件 | 状态 | 说明 |
|---|---|---|
| ControlHub（Go） | IMPLEMENTED | HTTP API + 内嵌 MQTT（bind/advertise 拆分）+ 配对（请求级地址解析）+ API Key + SQLite + 托盘 + Web 三页面；`go test` 全绿 + mock e2e |
| 固件（C，ESP-IDF v5.4.4） | IMPLEMENTED ＋ 真机基本链路已验证 | USB HID / 命令引擎 / 可靠性语义 / **BLE Provision + NVS 运行时配置 + 配网状态机**源码完成、编译通过、host 单测；**2026-08-20 真机 bring-up 通过**（ESP32-S3-WROOM-1，实测 16MB flash + 8MB PSRAM，Windows 宿主）：启动 / Wi-Fi / MQTT / USB HID 枚举 / 键鼠命令 executed（期间修复 5 个真机专属 bug，见下）。BLE 配网真机实测、BIOS / 三 OS / soak 未验证 |
| mock-device（cmd/mock-device） | IMPLEMENTED | Go 参考实现 = 虚拟 ESP32，测试替身；连非默认端口需 `--mqtt-port` |
| 官网（smart-hid-web） | IMPLEMENTED | 纯静态落地页 / 文档站 / 下载中心，GitHub Pages 托管 |
| protocols/ JSON Schema | IMPLEMENTED | command / ack / status 三 schema + 示例；**ble/PROVISIONING_V1.md（canonical）** |
| BLE 配网（固件侧） | IMPLEMENTED IN SOURCE ＋ BUILD VERIFIED ＋ NOT VERIFIED ON HARDWARE | NimBLE GATT 服务 + 分帧协议 + 状态机（M1-G3）；真机未验 |
| BLE 配网（小程序侧） | PARTIAL（外部独立仓 smart-ble） | 不在本仓库；需按 protocols/ble/PROVISIONING_V1.md 对齐客户端 |
| OTA | PLANNED | 分区表已扩至双 1536K OTA（M1-G3） |
| Secure Boot / Flash Encryption / 固件签名 | PLANNED | M2-G3 |
| 真机烧写与硬件验收 | IN PROGRESS | 2026-08-20 首次烧写 + 基本链路验收通过（Windows 单机）；完整验收（BIOS / 登录界面 / 三 OS / 断连 soak）未执行 |

## ControlHub 能力清单（按代码核对）

- HTTP API：11 path，事实源 `smart-hid-controlhub/docs/openapi.yaml`
- 内嵌 MQTT broker（mochi-mqtt）+ PerDeviceHook：hub 账号 + 每设备随机凭据 + per-device topic ACL
- 命令闭环：POST → publish（QoS1，严禁 retain）→ 同步等 ACK；TTL 内未终态返回 202
- **request_id 服务端幂等（M1-G2）**：并发同命令 join（publish 恰一次）；同 id 异命令 409 request_id_conflict；终态后重放直接返回既有结果（不再执行 HID）；指纹 = device/type/action/canonical payload（键序无关）
- **payload 服务端深度校验（M1-G2）**：键名（镜像固件 keymap）、hotkey ≤8 键、lease 范围、dx/dy ≤4096、wheel 硬限 [-127,127]（固件单次 int8 强转）、count 1-10、button 枚举
- 配对：Web 建 session → 动态 QR（`shid://pair?...`）→ 设备 POST `:17892` 换 MQTT 凭据（一次性 token，5 分钟，**单事务原子消费**——并发恰一次成功，失败可重试）
- **MQTT 网络模型（M1-G3）**：`mqtt.bind_host`（默认 0.0.0.0，LAN 设备可达）/ `advertise_host`（空 = 按设备请求路径解析，绝不返回环回/通配）/ 内部连接地址（由 bind 推导）；多网卡歧义明确报错并列出候选；legacy `mqtt.host` 自动迁移 + 一次性 deprecated 警告
- **pairing endpoint 先解析后消费 token（M1-G3）**：advertise 解析失败 → 503，token 保持 pending；QR host 与设备路径同一 resolver
- **内部 MQTT 凭据随机化（M1-G3）**：不再有固定默认密码——留空 = 每启动随机（仅内存，不持久化不进日志）；显式成对配置仍支持（如 e2e）
- **ACK 三方绑定（M1-G2）**：topic 设备 == ack.device_id == 在途请求期望设备；非法 ACK 记 warning 丢弃
- API Key：SHA-256 入库、明文不落库、可轮换；HTTP 用 Bearer，WebSocket 用 query key；**首启明文只落 0600 文件，不进日志（M1-G2）**
- Web 三页面：控制台 `/`、模拟键鼠演示台 `/demo.html`（可视化键盘 / 触控板 / 实体键盘直通 / 文本连打 / 多设备广播）、实时事件通道 `/api/v1/realtime`
- 配置面：`http.lan_mode` / `http.enable_api`、`mqtt.*`、`pairing.enabled` / `pairing.port`、`web.console` / `web.demo` / `web.realtime`；缺省 = 全开内置默认
- SQLite 持久化：migrations 0001 / 0002 / 0004（commands.fingerprint）
- 系统托盘常驻（`-tray`）与 headless 双生命周期

## 固件能力清单（按代码核对）

- USB Composite HID：键盘 + 鼠标（TinyUSB）
- 板载状态 LED：led_manager（WS2812 / 单色 / 无，Kconfig 可配）轮询 Wi-Fi/MQTT/USB 映射闪烁语义 + EXECUTED 命令脉冲（2026-08-20 新增，待 `idf.py build` 与真机验证）
- 命令引擎：串行队列（32）／ request_id 去重（256）／ TTL ／ target_boot_id ／ lease 超时释放 ／ release_all
- Fail-safe：MQTT / Wi-Fi 断开 → 设备端自动释放全部按键（LWT 语义）
- **NVS 运行时配置（M1-G3）**：runtime_config 组件（active/pending 双 namespace、schema_version 守卫、generation、factory clear 底层能力）；Kconfig 网络参数仅 `SMART_HID_DEV_STATIC_CONFIG=y` 时作 DEV fallback（默认 OFF，绝不覆盖 NVS）
- **配网状态机（M1-G3）**：BOOT→LOAD_CONFIG→UNPROVISIONED/PROVISIONING/CONNECTING_WIFI/PAIRING/CONNECTING_MQTT/READY/RECOVERY/ERROR；candidate 先 stage pending、成功才 promote（配网失败不变砖）；崩溃边界（token 已消费后掉电）由 complete-pending boot promote 收敛
- **BLE Provision（M1-G3）**：NimBLE GATT（Provisioning Service 三特征 + 分帧写入 + 状态 notify；Just Works 加密如实声明非 MITM 抗性）；协议 canonical = `protocols/ble/PROVISIONING_V1.md`
- 宿主单测：dedup_cache、hid_keymap、**runtime_config / provisioning / ble_proto**（`smart-hid-firmware/test/host/`，36 suite）

## 验证状态（诚实边界）

- `go test ./...` 全绿（含并发压力用例）；`go test -race ./...` 全绿；并发/网络包高倍重复通过
- `test-loop-f2.sh` 28/28（真二进制 + mock-device 端到端，无需硬件；含幂等重放与日志零明文断言；**经 legacy mqtt.host 兼容路径驱动**）
- 固件 `idf.py fullclean + build` 双配置通过（默认 provisioning 模式 + DEV_STATIC_CONFIG）；host 单测 36/36（含配网状态机崩溃边界）
- 交付链（M1-G4）：根 VERSION 唯一版本源（二进制 `-version` 自证 + 固件 PROJECT_VER 注入）、
  build-releases 干净构建（dirty 拒绝/显式 SHA256/manifest/投影防漂移）、协议与 OpenAPI 门
  （validate-protocols.py）、shellcheck warning 级全过、CI/release/docs 三 workflow（tag 驱动发布）
- 配置面 e2e、WebSocket e2e、演示台 Playwright 真浏览器实测均通过（2026-08-20）
- **真机 bring-up（2026-08-20，ESP32-S3-WROOM-1 + Windows）已通过**：
  烧写（UART/CH343）→ 启动 → Wi-Fi（STA）→ MQTT（hub 账号 + PerDeviceHook）→
  USB OTG 接入后宿主机枚举为 HID Keyboard + Mouse → HTTP API 下发
  keyboard tap 与 mouse move 均返回 `executed`，**CapsLock 状态翻转与光标移动均有
  客观测量证据**（PowerShell CapsLock 查询 / Cursor::Position 前后对比）；
  板载 LED 状态机（led_manager）五态与命令脉冲真机表现正确；
  ControlHub 对 usb_hid_ready=false 的设备正确拒绝命令。
  期间修复五个仅真机可暴露的问题：
  ① TinyUSB task xCoreID=-1 断言崩溃；
  ② 双接口复合描述符 Windows 鼠标集合收不到输入（改单接口复合）；
  ③ **espressif TinyUSB 0.21 鼠标模板含 AC_PAN 水平轮 → 输入报告须 5 字节**，
    发 4 字节被 Windows 静默丢弃而设备端返回成功（最隐蔽，短报告无任何报错）；
  ④ 键名结构体 key[8] 截断 8+ 字符键名（CAPSLOCK → CAPSLOC 被拒）；
  ⑤ 稳定性双修：Wi-Fi 默认省电（WIFI_PS_MIN_MODEM）致 MQTT 周期断连（弱信号下
    每 ~10s）→ WIFI_PS_NONE 后连发 8 条 8/8 executed；多段鼠标报告段间隔与
    bInterval 同相位竞态致前段丢失 → 15ms 错相后两轴完整移动。
- Wi-Fi 省电已关闭（延迟/稳定性优先）；弱信号（RSSI≈-77）下仍可能偶发断连，
  设备自动重连（~10s 内恢复），期间命令 accepted_not_acked，重试即可
- ControlHub（Windows exe）联调中途发生过一次进程退出，原因未查（重启后正常），
  已登记待查
- 真机**未**验证：BLE 实连配网、BIOS / 登录界面（描述符无 boot protocol，已知缺口）、
  macOS / Linux、断连 soak、长时间稳定性
