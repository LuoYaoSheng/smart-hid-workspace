# Smart BLE × Smart HID 通用工具/Profile 集成实施记录

> 本文保留原始架构计划和批次设计，并记录 2026-08-21 的实际实施结果；当前事实状态以 `docs/current/` 为准。

**目标：**保持 `smart-ble` 的开源通用 BLE 工具定位，同时把 Smart HID 建成内置、可拔除、边界清楚的第一方设备 Profile，并提供完整专属产品体验。

**架构：**由唯一 BLE Runtime 统一持有 `uni.onBLE*` 全局回调；通用层负责扫描、连接、GATT、通知路由和分帧机制；Smart HID 层负责 UUID、candidate、QR、状态语义、恢复策略和专属页面。两个产品版本独立，通过 canonical commit、契约摘要和兼容锁文件建立跨仓一致性。

**技术栈：**Vue 3、uni-app、Pinia、JavaScript/TypeScript、Node.js、Go、ESP-IDF C、Python 标准库、GitHub Actions。

---

## 0. 文档状态

- 状态：`IMPLEMENTED IN SOURCE / HARDWARE VALIDATION PENDING`
- 日期：2026-08-21
- 主仓：`Smart-HID-Workspace`，分支 `main`
- 客户端仓：`smart-ble`，分支 `main`
- 主仓实施提交：`ef45067`（machine contract）、`8fcf3cc`（落地页与当前说明同步）
- smart-ble 审计起始基线：`dd5eb8d`
- smart-ble 实施结果：唯一 BLE Runtime、严格 Profile contract、Smart HID V1 适配、contract lock、统一首页 Profile 入口、三阶段配网与自动门禁均已落地
- 硬件状态：本计划不授权 flash、monitor、esptool、eFuse、Secure Boot 或任何真机操作

实施过程中按用户评审取消了独立 HID Tab 和 Solutions 二次入口：Smart HID 由首页统一扫描识别，设备卡片直接提供 Profile 任务；配网收敛为“自动连接 → 单页填写 Wi-Fi/ControlHub → 查看状态”。协议语义未改变。

## 1. 架构决策

采用以下产品定位：

> `smart-ble` 是开源通用 BLE 工具；Smart HID 是随工具内置的第一方设备 Profile/解决方案，不属于通用 BLE 核心。

目标依赖方向：

```text
uni BLE 平台 API
        ↓
唯一 BLE Runtime / Session Manager
        ├─ 通用扫描与 GATT 调试器
        ├─ 通用 Provisioning Transport
        └─ Profile Runtime
              ↓
        Smart HID Profile
        ├─ Descriptor
        ├─ Codec
        ├─ Workflow Adapter
        └─ Smart HID 专属页面/Store
```

本计划固定以下原则：

1. Smart HID 默认随 smart-ble 内置并保持清晰入口。
2. 移除 Smart HID Profile 注册后，通用扫描、连接、读写、notify、广播仍可使用。
3. 通用层不得出现 Smart HID UUID、ControlHub、MQTT、pairing token 或 HID 状态机语义。
4. 允许 Profile 拥有专属 UI；不把所有产品流程强行做成大型 JSON 动态表单。
5. 所有 `uni.onBLE*` 回调只有一个所有者，页面和 Profile 通过内部事件总线订阅。
6. smart-ble 与 Smart HID 产品版本独立演进。
7. 兼容性通过 canonical commit、contract SHA-256、tested product version 表达。
8. 本计划不修改 MQTT topic/命令语义、BLE UUID/帧/candidate/错误码或配对 API。

## 2. 当前结合方式评估

### 2.1 已经合理的部分

- `[BLE]/core/ble-core/provisioning/framing.js` 是纯字节/分帧逻辑，不依赖 Smart HID、Pinia 或 `uni.*`。
- `[BLE]/apps/uniapp/services/provisioning/transport.js` 已尝试把 GATT 原语与设备业务分开。
- `[BLE]/core/protocols/hid-provisioning-protocol.ts` 集中持有 Smart HID UUID、candidate、QR、状态和错误码。
- `[BLE]/apps/uniapp/services/smart-hid/index.js` 依赖通用 provisioning 模块，通用模块没有反向依赖 Smart HID。
- `[BLE]/apps/uniapp/pages/hid/` 保留专属配网向导和诊断体验，没有污染通用 GATT 页面。

### 2.2 仍不够合理的部分

1. 通用设备详情页仍直接注册 `uni.onBLECharacteristicValueChange`、`uni.onBLEConnectionStateChange`。
2. Provisioning transport 又注册另一套全局回调，两条 GATT 路径会互相覆盖或串扰。
3. 通知 listener 只按 characteristic UUID 路由，没有同时绑定 device/service。
4. Smart HID Profile 依赖 import 副作用完成注册，装配关系不显式。
5. 重复 Profile ID 会静默覆盖。
6. Profile 当前只像 UUID 配置表，没有正式 codec/workflow 能力契约。
7. 固定 HID Tab 无法自然扩展第二、第三个第一方设备方案。
8. 没有 Fake Profile 证明通用层真的能接另一种设备。
9. Smart HID TS 镜像依靠人工同步另一个仓库。
10. smart-ble CI 没有运行 uni-app provisioning/Profile/Runtime 测试及全量 Vue 编译。

## 3. 分层边界

### 3.1 smart-ble 通用层负责

- 蓝牙适配器与权限状态
- 扫描开始/停止、发现事件流、设备去重、RSSI 更新
- 连接/session 创建与释放
- service/characteristic discovery
- MTU 协商
- read/write/notify 原语
- 全局 BLE 事件路由
- timeout、cancel、disconnect、retry 基础能力
- 通用 framing strategy 接口
- Profile 注册与 capability 校验
- 广播强/弱匹配
- 通用错误分类
- 日志敏感字段过滤钩子
- 可注入的平台适配器，供 Node 单测使用

### 3.2 Smart HID Profile 负责

- `9f1d1001`～`9f1d1004` UUID
- `SHID-` 广播名规则
- `shid://pair` QR
- pairing token 形态
- `wifi_ssid/wifi_password/hub_host/hub_port`
- Device Info、Provision Status 编解码
- state/step/error 解释
- ControlHub、pairing、MQTT、USB HID 术语
- Wi-Fi + 扫码 + 配对 + READY 向导
- Smart HID 恢复动作和诊断映射
- Smart HID 专属 Pinia 状态

### 3.3 边界能力

- 分帧算法属于通用层，Smart HID Profile 只声明选择哪种 strategy。
- 状态展示组件可以通用，具体状态含义由 Profile 解释。
- 名称前缀只能算弱匹配；连接后必须通过 service 与 Device Info 确认身份。
- Profile 可拥有专属页面，但不能绕过通用 Runtime 直接操作 `uni BLE`。

## 4. 目标文件结构

尽量沿用现有目录，不为“看起来整齐”做大范围搬迁：

```text
[BLE]/core/
├─ ble-core/
│  └─ provisioning/
│     ├─ framing.js                 # 现有纯分帧逻辑
│     └─ profile-contract.js        # 新增：Profile 校验/契约
└─ protocols/
   ├─ hid-provisioning-protocol.ts  # Smart HID 镜像/codec
   └─ smart-hid-contract.lock.json  # 跨仓兼容锁

[BLE]/apps/uniapp/
├─ services/
│  ├─ ble-runtime/
│  │  ├─ index.js                   # uni.onBLE* 唯一所有者
│  │  └─ platform.js                # 可注入 uni API adapter
│  ├─ provisioning/
│  │  ├─ transport.js               # 基于 BLE Runtime
│  │  ├─ profiles.js                # 严格 registry
│  │  └─ builtins.js                # 显式注册第一方 Profiles
│  └─ smart-hid/
│     ├─ profile.js                 # descriptor + codec/workflow 引用
│     ├─ workflow.js                # 状态/恢复/诊断语义
│     └─ index.js                   # Smart HID 应用服务
├─ pages/
│  ├─ device/                       # 通用 GATT 调试，同样使用 Runtime
│  ├─ solutions/                    # 后续“方案”入口
│  └─ hid/                          # Smart HID 专属 UI
└─ store/
   ├─ ble.js                        # 通用扫描/session 状态
   └─ hid.js                        # Smart HID 业务状态
```

文档中的 `[MAIN]` 指 `Smart-HID-Workspace/`，`[BLE]` 指独立兄弟仓 `smart-ble/`。

## 5. Profile 契约

### 5.1 最小接口

新增 `[BLE]/core/ble-core/provisioning/profile-contract.js`，逻辑形态如下：

```js
export const PROFILE_MATCH = Object.freeze({
  NONE: 0,
  WEAK: 1,
  STRONG: 2
});

export function defineProvisioningProfile(input) {
  // 校验后返回不可变 descriptor。
}

// Profile 逻辑形态：
// {
//   id: 'smart-hid',
//   version: '1',
//   displayName: 'Smart HID',
//   matchAdvertisement(device): PROFILE_MATCH.STRONG|WEAK|NONE,
//   gatt: {
//     serviceUuid: '...',
//     characteristics: { info: '...', input: '...', status: '...' },
//     required: ['info', 'input', 'status'],
//     notify: ['info', 'status'],
//     preferredMtu: 247
//   },
//   transport: { framing: 'framed-v1' },
//   codec: {
//     parseDeviceInfo(bytes) {},
//     parseStatus(bytes) {},
//     buildCandidate(input) {}
//   },
//   workflow: {
//     classifyStatus(status) {},
//     recoveryAction(status) {}
//   }
// }
```

校验规则：

- Profile ID 必填且唯一。
- UUID 统一规范化为小写。
- required alias 必须存在于 characteristics。
- notify alias 必须引用已声明特征。
- 匹配结果只能是 NONE/WEAK/STRONG。
- codec 函数必须可调用。
- 注册后 descriptor 冻结。
- 重复 ID 默认抛错，仅测试 reset API 可清注册表。
- descriptor 不保存任何 secret。

### 5.2 匹配强度

```text
STRONG = 广播中 Service UUID 命中
WEAK   = 只有设备名前缀命中
NONE   = 无 Profile 证据
```

弱匹配只能显示“可能是 Smart HID”；service discovery 与 Device Info 校验通过后才能标为已确认。

## 6. BLE Runtime 契约

新增 `[BLE]/apps/uniapp/services/ble-runtime/index.js`，作为以下接口的唯一所有者：

- `uni.onBluetoothDeviceFound`
- `uni.onBLEConnectionStateChange`
- `uni.onBLECharacteristicValueChange`

内部模型：

```text
sessions: Map<deviceId, Session>

Session
├─ deviceId
├─ connection state
├─ discovered services/chars
├─ MTU
├─ listeners(deviceId/serviceId/charId)
├─ pending one-shot reads
└─ close/dispose
```

必须满足：

1. Runtime callback 只注册一次。
2. 通知按 device + service + characteristic 完整路由。
3. 同一特征允许多个订阅者。
4. 关闭设备 A 不清设备 B listener。
5. read timeout 会移除一次性 listener。
6. disconnect 只拒绝对应 session 的 pending 操作。
7. 旧 session 的迟到断链事件不能清除新 session。
8. Runtime 不含任何 Profile 业务状态。
9. 单测通过 fake `uni` adapter 运行，不依赖微信工具。

## 7. 批次依赖

```text
批次 5A：通用 BLE Runtime + 严格 Profile contract
   ↓
批次 5B：Smart HID 迁移为第一方 Profile + 配网正确性修复
   ↓
批次 6：主仓机器可读 canonical contract
   ↓
批次 7：smart-ble contract lock + CI
   ↓
批次 9：Solutions UI + i18n/样式/图标
```

编号与《Smart HID 全面审计》的总体批次保持一致；批次 1～4、8 属于固件、ControlHub 和发布链，不在本文重复展开。

## 8. 批次 5A：通用 BLE Runtime 与 Profile Contract

**仓库：**`[BLE] smart-ble`

**提交：**仅一次，前缀 `refactor:`

**协议影响：**无

**文件：**

- Create: `core/ble-core/provisioning/profile-contract.js`
- Create: `apps/uniapp/services/ble-runtime/index.js`
- Create: `apps/uniapp/services/ble-runtime/platform.js`
- Modify: `apps/uniapp/services/provisioning/profiles.js`
- Modify: `apps/uniapp/services/provisioning/transport.js`
- Modify: `apps/uniapp/store/ble.js`
- Modify: `apps/uniapp/pages/device/detail.vue`
- Test: `tests/unit/profile-contract.test.mjs`
- Test: `tests/unit/ble-runtime.test.mjs`
- Test: `tests/unit/provisioning-transport.test.mjs`

### 5A-0 前置检查

1. 两仓运行 `git status --short --branch`。
2. 主仓 `git log` 确认 `577e1e6` 仍在。
3. 先确认 smart-ble 当前未提交 UI/style 变更的归属。
4. 未获用户指示不得 reset、checkout、stash 或覆盖这些变更。
5. 不碰 smart-ble 的 master、AI、develop、refactor 分支。

### 5A-1 先写失败测试

覆盖：

- 重复 Profile ID 被拒绝；
- required characteristic 缺失被拒绝；
- UUID 规范化；
- Fake Profile 注册与匹配；
- 两台设备使用相同 characteristic UUID 时通知不串线；
- 设备 A 断开不影响设备 B；
- read 超时 listener 被清理；
- 旧 disconnect 回调不能清除新 session。

运行：

```bash
node tests/unit/profile-contract.test.mjs
node tests/unit/ble-runtime.test.mjs
node tests/unit/provisioning-transport.test.mjs
```

实现前预期：针对尚不存在 contract/runtime 或当前非 device-scoped 路由出现明确 FAIL。

### 5A-2 实现最小 Profile Contract

- 按第 5 节校验接口。
- 返回冻结 descriptor。
- 模块不得 import `uni`、Vue、Pinia、i18n、Smart HID。
- `services/provisioning/profiles.js` 使用该校验器。
- 删除重复 ID 静默覆盖行为。

### 5A-3 实现 BLE Runtime

- 用可注入 platform adapter 包装 `uni`。
- 全局 callback 只注册一次。
- session 按 deviceId 管理。
- 通知按完整 identity tuple 路由。
- 暴露现有页面需要的 scan/connect/discover/MTU/read/write/notify/disconnect。

### 5A-4 迁移 Provisioning Transport

- `transport.js` 不再直接拥有全局 callback。
- 改为消费 Runtime session。
- “开启 notify”与“注册 callback”分离，禁止把 `undefined` 加入 listener 集合。
- 尽量保持公开函数签名，控制 5B diff。

### 5A-5 迁移通用 Store 与设备详情页

- `store/ble.js` 消费 Runtime scan stream。
- `pages/device/detail.vue` 使用 Runtime read/write/notify。
- 没注册任何 Profile 时，通用 GATT 调试仍能工作。

### 5A-6 Fake Profile 证明通用性

Fake Profile 必须：

- 使用与 Smart HID 不同的 UUID；
- 复用同一 Runtime；
- 选择 raw 或 framed transport；
- 不 import Smart HID。

验收：加入 Fake Profile 不修改 Runtime 或 transport 源码。

### 5A-7 全门禁与一次提交

全部门禁绿后：

```bash
git commit -m "refactor: centralize BLE runtime and profile contract"
```

不 push。

## 9. 批次 5B：Smart HID 第一方 Profile 迁移

**仓库：**`[BLE] smart-ble`

**提交：**仅一次，前缀 `fix:`

**协议影响：**无，仅恢复既定 V1 行为

**文件：**

- Create: `apps/uniapp/services/smart-hid/profile.js`
- Create: `apps/uniapp/services/smart-hid/workflow.js`
- Create: `apps/uniapp/services/provisioning/builtins.js`
- Modify: `apps/uniapp/services/smart-hid/index.js`
- Modify: `apps/uniapp/store/hid.js`
- Modify: `apps/uniapp/pages/hid/add.vue`
- Modify: `core/protocols/hid-provisioning-protocol.ts`
- Test: `tests/unit/smart-hid-profile.test.mjs`
- Test: `tests/unit/smart-hid-service.test.mjs`
- Test: `tests/unit/provisioning.test.mjs`

### 5B-1 显式 Smart HID Descriptor

- 从 service import 副作用中移出注册。
- Descriptor 包含 UUID、matcher、required/notify aliases、MTU、framing、codec。
- `builtins.js` 在应用装配入口显式注册 `smartHidProfile`。
- ControlHub/MQTT 术语不得进入通用 registry。

### 5B-2 身份确认

- 广播 Service UUID 命中为强匹配。
- `SHID-` 名称命中仅为弱匹配。
- 连接后校验 product、protocol、device_id，成功前禁止 candidate write。

### 5B-3 修复扫描实时性

- Smart HID 观察通用 scan stream。
- 不再在 `startScan()` 返回后立刻读取一次快照。
- 匹配设备到达时实时更新 UI。
- 扫描到期或用户停止后再完成扫描 Promise。

### 5B-4 修复 Status waiter 竞态

- 只清当前 attempt 状态。
- 写 candidate 最后一帧前先注册 result waiter。
- status 绑定本地 session/attempt generation。
- `invalid_payload` 等快速响应不得丢失。

### 5B-5 分离 state 与 step

- progress 同时读取 `status.state` 和 `status.step`。
- 不重命名任何 canonical 值。
- 未知值进入诊断显示，不伪装为已知状态。
- 固件额外 step 漂移只登记，不在本批修改协议语义。

### 5B-6 QR 防御解析

- percent decode 整体 try/catch。
- 畸形编码返回 null。
- scheme/token/host/port 语义不变。
- 增加坏 `%`、重复参数、非法 host、端口边界测试。

### 5B-7 全门禁与一次提交

专项验收：

- 原 20 项 provisioning tests 保持全绿；
- 新 Profile/service tests 全绿；
- 全量 Vue SFC 编译全绿；
- BLE Runtime 之外不再注册 `uni.onBLECharacteristicValueChange` 或 `uni.onBLEConnectionStateChange`。

提交：

```bash
git commit -m "fix: integrate Smart HID as a first-party BLE profile"
```

不 push。

## 10. 批次 6：主仓机器可读 Canonical Contract

**仓库：**`[MAIN] Smart-HID-Workspace`

**提交：**仅一次，前缀 `test:`

**协议影响：**无；JSON 只镜像当前已部署 V1

**文件：**

- Create: `protocols/contracts/smart-hid-v1.json`
- Create: `protocols/contracts/README.md`
- Modify: `scripts/validate-protocols.py`
- Modify: `.github/workflows/ci.yml`
- Modify: `smart-hid-web/downloads/build-releases.sh`
- Modify: release manifest generation
- Test: `protocols/examples/` fixtures

机器契约包含：

- MQTT topic、QoS、retain 规则；
- command protocol 版本与限制；
- BLE UUID/权限；
- frame header/limits；
- candidate 字段、必填/可选、默认端口；
- Device Info 字段；
- 已批准的 state/step/error 列表；
- QR scheme/token 形态；
- source metadata 与 contract version。

验证：

- 对 canonical JSON bytes 计算确定性 SHA-256。
- 对照 C header、Go 常量、JSON Schema、OpenAPI 示例、canonical Markdown。
- 镜像改变但 contract 未更新时直接 FAIL。
- Release manifest 增加 `contract_sha256`。
- Markdown 不再充当机器解析的主数据源。

提交：

```bash
git commit -m "test: enforce the Smart HID machine contract"
```

不 push。

## 11. 批次 7：smart-ble Contract Lock 与 CI

**仓库：**`[BLE] smart-ble`

**提交：**仅一次，前缀 `chore:`

**文件：**

- Create: `core/protocols/smart-hid-contract.lock.json`
- Create: `scripts/check-smart-hid-contract.mjs`
- Modify: `.github/workflows/ci.yml`
- Modify: Smart HID 公开文档的 authority 表述
- Modify: uni-app tests/package metadata（按需）

Lock 格式：

```json
{
  "canonical_repository": "LuoYaoSheng/smart-hid-workspace",
  "canonical_commit": "<full commit sha>",
  "contract_version": "1",
  "contract_sha256": "<sha256>",
  "tested_smart_hid_version": "1.1.1",
  "miniapp_version": "1.0.4"
}
```

CI：

1. 使用能执行 TS mirror tests 的 Node 版本。
2. 运行 provisioning/Profile/Runtime tests。
3. 编译所有 uni-app `.vue` template。
4. 把 canonical 仓锁定 commit checkout 到临时目录。
5. 计算 contract digest 并与 lock 比较。
6. 比对 Smart HID TS 常量与字段集合。
7. 禁止跟随对方移动中的 `main` 做比较。

smart-ble 文档统一表述：

```text
语义正典：Smart-HID-Workspace machine contract / PROVISIONING_V1
smart-ble TypeScript：客户端可执行镜像与 codec
```

提交：

```bash
git commit -m "chore: lock Smart HID contract compatibility"
```

不 push。

## 12. 批次 9：Solutions UI 与表现层收敛

**仓库：**`[BLE] smart-ble`

**依赖：**5A、5B、6、7

**提交：**仅一次，依据最终范围使用 `refactor:`、`perf:` 或 `fix:`

**预计文件：**

- Create: `apps/uniapp/pages/solutions/index.vue`
- Modify: `apps/uniapp/pages.json`
- Modify: `apps/uniapp/pages/hid/*.vue`
- Modify: `apps/uniapp/locale/zh-CN.json`
- Modify: `apps/uniapp/locale/en-US.json`
- Modify: `apps/uniapp/styles/design-system.css`
- Replace: `apps/uniapp/static/tabs/*.png`

推荐 Tab：

```text
设备 | 方案 | 广播 | 关于
```

“方案”页显示 Smart HID 第一方 Profile，并作为未来其他 Profile 的入口。通用扫描页确认 Smart HID 后，也可以显示“前往配置”的上下文操作。

要求：

- 保留 Smart HID 专属向导与诊断页。
- 不把向导改成通用 JSON 表单。
- 增加 `hid.*`、`solutions.*` i18n namespace。
- 通用 card/button/status 样式收敛到 design system。
- 替换正式 normal/active 图标。
- 历史设备必须明确标注“非实时在线”。
- 诊断页需为目标设备建立 BLE session，或提供明确重连流程。

提交：

```bash
git commit -m "refactor: present device profiles through the solutions hub"
```

不 push。

## 13. 每批必过门禁

### Smart-HID-Workspace

```bash
cd Smart-HID-Workspace/smart-hid-controlhub
go vet ./...
go test -race ./...
bash scripts/test-loop-f2.sh

cd ../smart-hid-firmware/test/host
./run.sh

cd ../../..
source ~/esp/export.sh
bash scripts/build-firmware.sh <validated-temp-output-dir>

bash scripts/check-governance.sh
python3 scripts/validate-protocols.py
```

临时输出目录必须来自已校验的 `mktemp -d`。禁止 flash。

### smart-ble

```bash
cd smart-ble
node tests/unit/provisioning.test.mjs
# 同时运行该批新增的 Runtime/Profile/Smart-HID tests。
```

并执行：

- `@vue/compiler-sfc` 全量 `.vue` parse + compileTemplate；
- 所有变更的独立 JS 文件 `node --check`；
- 搜索确认 BLE Runtime 外无全局 BLE callback 注册；
- 搜索确认通用 core 无 Smart HID 产品术语；
- 搜索确认 wifi_password、pairing token、MQTT credential、API key 未进入新日志/URL/存储；
- commit 前再次确认两仓状态与 `577e1e6` 未丢失。

## 14. 完成定义

只有以下全部成立，才算结合架构完成：

1. 唯一 BLE Runtime 持有全部 `uni.onBLE*`。
2. 通用调试器与 Smart HID 使用同一个 session manager。
3. 通知不会跨设备串线。
4. Smart HID Profile 显式注册且可拔除。
5. 重复/非法 Profile 确定性失败。
6. Fake Profile 无需改 Runtime/transport 即可接入测试。
7. 禁用 Smart HID 后通用工具仍正常。
8. 通用模块无 Smart HID UUID、ControlHub、MQTT、token 逻辑。
9. 弱广播匹配经过 Device Info 二次确认。
10. 快速 Status 不会早于 waiter 丢失。
11. state 与 step 分开处理。
12. 两个产品版本独立，但发布同一 contract digest。
13. 两仓 CI 都能发现协议漂移。
14. 未修改任何已部署 V1 语义。
15. 全部门禁全绿。
16. 用户未执行真机/微信动态验证前，状态保持 `NOT VERIFIED`。

## 15. 失败模式与缓解

| 失败模式 | 缓解 |
|---|---|
| Runtime 逐渐变成 Smart HID 专用层 | 通用 core 禁止产品术语，并用 Fake Profile 证明复用 |
| 多页面争抢 BLE callback | 单一 event broker + 静态搜索门禁 |
| Profile 变成隐藏的大型应用框架 | 契约只保留 descriptor/codec/workflow hook，允许专属 UI |
| 名称伪装被当作设备身份 | 匹配强度 + Device Info 确认 |
| 双仓顺序提交导致 CI 随机红 | lock 固定 canonical commit，不比较移动 main |
| 两产品版本被错误绑定 | digest/compatibility lock，不要求版本相等 |
| 通用工具回归 | 禁用 Smart HID Profile 后跑通用 scan/GATT tests |
| 覆盖 smart-ble 当前外部改动 | 5A 前停下确认 dirty worktree 归属 |
| 契约清理误改部署语义 | 先做机器镜像；语义差异另立提案 |
| candidate 被通用日志记录 | Runtime redaction + Profile 敏感字段测试 |

## 16. 回滚策略

- 每批严格一个本地提交。
- 不 squash 或丢失主仓 `577e1e6`。
- 不自动 push。
- 审查不通过时只 revert 对应批次，不 reset 两仓。
- 5A 的兼容适配必须在 5B 有明确移除点。
- 5B 可独立回滚，前提是通用调试器保持全绿。
- 跨仓 lock 两批可独立回滚，产品版本不绑定。

## 17. 明确不做

- 不做固件 flash/真机操作
- 不改 MQTT topic 或命令语义
- 不改 BLE V1 UUID、framing、candidate、错误码
- 不改 pairing API
- 不实现 Secure Boot、Flash/NVS Encryption 或 TLS
- 不引入新重框架/插件依赖
- 不支持远程动态下载 Profile 或执行第三方代码
- 不碰 smart-ble 非 main 分支
- 不访问生产服务器或部署环境

## 18. 用户最终动态验证

自动门禁全部通过后，由用户执行：

- 微信开发者工具构建与页面导航
- Android/iOS BLE 权限、前后台、断线重连
- Smart HID 广播发现与身份确认
- 加密 candidate write
- Status notify 时序
- ControlHub pairing → MQTT → READY
- 通用 GATT 调试器回归
- Solutions/HID 页视觉验收

完成这些项目之前，只能标注 source-tested/build-tested，不能标注 hardware-verified 或 production-secure。
