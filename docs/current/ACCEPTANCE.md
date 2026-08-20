---
status: CURRENT
authority: canonical
---

# ACCEPTANCE — 当前验收清单

> 只验收当前产品边界内的项。旧的 Trial / License / Web 验收项已随能力一起移除
> （历史版见 docs/archive/10，SUPERSEDED）。勾选标准：必须有可复现的证据
> （测试输出 / e2e 记录），禁止为凑整齐打勾。

## A. Governance（治理）

- [x] 根 README 是唯一 bootstrap 入口，链接 current / archive
- [x] docs/current/ 带 CURRENT 标识，docs/archive/ 全部带 SUPERSEDED 标识
- [x] 开发规则（含 No Resurrection）成文
- [x] 治理守卫脚本 `scripts/check-governance.sh` 存在且通过

## B. ControlHub（mock / 本机验证 ✅，真机相关 ❌）

- [x] HTTP API 默认只听 127.0.0.1；LAN 需显式开启（config + 控制台开关）
- [x] API Bearer 鉴权；WS query key；Key 可轮换（mock e2e）
- [x] 内嵌 MQTT 每设备凭据 + per-device ACL（PerDeviceHook）
- [x] 命令闭环：executed / rejected / TTL 超时 202（test-loop-f2 28/28，mock）
- [x] 设备在线 / 离线 / boot_id 变化识别（mock e2e）
- [x] 配对 session 一次性 token + QR 载荷端口跟随配置（e2e）
- [x] 配对 token 原子单次消费：50 并发恰 1 成功、失败可重试、无半状态（M1-G2，-race）
- [x] request_id 幂等：并发同命令 join（publish 恰一次）/ 异命令 409 / 终态重放不重发（M1-G2，-race）
- [x] wrong-device ACK 无法认领在途请求（三方绑定，M1-G2）
- [x] payload 服务端深度校验（键名/范围/枚举，合法 fixtures 全兼容，M1-G2）
- [x] Web 三页面：控制台 / 演示台 / realtime（Playwright 实测）
- [x] 多设备广播与状态芯片条（双 mock e2e）
- [ ] Tray 长时间运行稳定性（未做 soak）
- [ ] Windows 安装器 / 防火墙引导（未做）

## C. Firmware Source（源码级 ✅；硬件 ❌）

- [x] ESP-IDF v5.4.4 编译通过（默认 provisioning 模式 + DEV_STATIC_CONFIG 双配置；分区 3×1536K 余量 32%）
- [x] 命令引擎语义：tap / hotkey / key_down(up) / move / click / wheel /
      button_down(up) / release_all / lease / TTL / dedup / boot_id（28 项 mock e2e）
- [x] LWT 掉线释放全部按键（mock e2e：broker 重启触发 release_all）
- [x] 宿主单测：dedup_cache、hid_keymap、runtime_config、provisioning 状态机
      （含崩溃边界）、ble_proto（36/36）
- [ ] Windows / macOS / Linux 识别键盘 + 鼠标 —— NOT EXECUTED（硬件）
- [ ] 队列满明确返回 —— 固件路径未在真机验证
- [ ] BLE Provision 真机配网全链路 —— NOT EXECUTED（硬件）

## D. Protocol（契约一致性）

- [x] command / ack / status JSON Schema 与 Go、固件实现一致（28 项 e2e 覆盖）
- [x] openapi.yaml 11 path 与路由一致（含 /realtime 事件说明）
- [ ] Schema 自动比对进 CI —— M1-G4

## E. Provisioning（配网）

- [x] ControlHub 侧配对服务（session / QR / 设备侧换凭据）
- [x] MQTT 网络模型：bind/advertise 拆分、legacy 迁移、环回/通配绝不返回设备、
      多网卡 deterministic 单测（M1-G3，internal/netaddr）
- [x] pairing 先解析 endpoint 再消费 token（失败 503 + token 保持 pending，单测）
- [x] 固件 NVS 运行时配置（active/pending、schema 守卫、崩溃边界 boot promote，
      host 单测 36 suite，M1-G3）
- [x] BLE Provision 固件源码（NimBLE GATT + 分帧协议 + 状态机 + canonical 协议
      文档；DEV_STATIC_CONFIG 双配置编译通过）
- [ ] BLE Provision 真机全链路（小程序 ↔ 固件）—— NOT VERIFIED ON HARDWARE
- [ ] 小程序客户端按 canonical 协议对齐 —— smart-ble 仓任务

## F. Security（G2/G3 已修项打勾；其余排 M2）

- [x] 控制链路不出局域网，无云端组件
- [x] API Key / 设备凭据明文不落库（SHA-256）
- [x] 首次 API Key 明文不进日志（只落 0600 文件；结构性防回归 + e2e 断言日志零 chk_，M1-G2）
- [x] 配对端口只在配对链路使用，token 一次性 + 5 分钟过期 + 原子消费（M1-G2）
- [x] 固定默认 MQTT 密码移除（内部凭据每启动随机；显式配置需成对，M1-G3）
- [x] mqtt.host 三用拆分 + advertise 校验（环回/通配/localhost 拒绝，M1-G3）
- [x] 固件 secret 不进日志（wifi/mqtt 密码、token redact；host 单测断言，M1-G3）
- [x] WS CheckOrigin 维持 LAN 有意取舍（G3 评审结论：记录不收紧）

## G. Release（G4 已修项打勾）

- [x] 唯一版本源：根 VERSION 文件 → ControlHub ldflags 注入（`-version` 自证）+ 固件 PROJECT_VER（M1-G4）
- [x] build-releases.sh：dirty tree 拒绝（DEV_BUILD 显式放行）、固件 fullclean 重建、
      显式 SHA256 清单 + 自校验、openapi 投影防漂移（M1-G4）
- [x] manifest.json provenance（version/commit/build_time/dirty + 每 artifact sha256）+ README_RELEASE 诚实状态（M1-G4）
- [x] CI 质量门：go fmt/vet/test/race + 协议/OpenAPI 门 + 治理守卫 + shellcheck + 固件双配置构建（M1-G4，ci.yml）
- [x] tag 驱动 Release：tag↔VERSION 一致校验 → 干净构建 → 自动发布（M1-G4，release.yml）
- [x] Pages 部署 workflow 化（docs.yml，仅 smart-hid-web）
- [ ] 首次 tag 发布走完全链路 —— 待下一个版本 tag 时实证

## H. Hardware（硬件验收）

```text
NOT EXECUTED — SEPARATE TASK（M2-G1）
```

真机烧写、USB 枚举、三操作系统、BIOS / 登录界面、断连 soak：全部未执行，
独立任务处理，本清单不为它们打勾。
