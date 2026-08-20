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
- [x] Web 三页面：控制台 / 演示台 / realtime（Playwright 实测）
- [x] 多设备广播与状态芯片条（双 mock e2e）
- [ ] Tray 长时间运行稳定性（未做 soak）
- [ ] Windows 安装器 / 防火墙引导（未做）

## C. Firmware Source（源码级 ✅；硬件 ❌）

- [x] ESP-IDF v5.4.4 编译通过
- [x] 命令引擎语义：tap / hotkey / key_down(up) / move / click / wheel /
      button_down(up) / release_all / lease / TTL / dedup / boot_id（28 项 mock e2e）
- [x] LWT 掉线释放全部按键（mock e2e：broker 重启触发 release_all）
- [x] 宿主单测：dedup_cache、hid_keymap
- [ ] Windows / macOS / Linux 识别键盘 + 鼠标 —— NOT EXECUTED（硬件）
- [ ] 队列满明确返回 —— 固件路径未在真机验证

## D. Protocol（契约一致性）

- [x] command / ack / status JSON Schema 与 Go、固件实现一致（28 项 e2e 覆盖）
- [x] openapi.yaml 11 path 与路由一致（含 /realtime 事件说明）
- [ ] Schema 自动比对进 CI —— M1-G4

## E. Provisioning（配网）

- [x] ControlHub 侧配对服务（session / QR / 设备侧换凭据）
- [ ] BLE Provision 全链路（小程序 ↔ 固件）—— PLANNED，M1-G3
- [ ] NVS 运行时配置 / 重配网 —— PLANNED，M1-G3

## F. Security（已知风险已登记，修复排 G2/G3）

- [x] 控制链路不出局域网，无云端组件
- [x] API Key / 设备凭据明文不落库（SHA-256）
- [x] 配对端口只在配对链路使用，token 一次性 + 5 分钟过期
- [ ] 首次 API Key 明文进日志 —— 已登记，G2 处理
- [ ] MQTT 默认密码弱 + host 三用 —— 已登记，G3 处理
- [ ] WS CheckOrigin 全放行（LAN 场景有意为之）—— 已登记，随 G3 评估

## G. Release（已知问题已登记，修复排 G4）

- [x] 双平台二进制 + 固件包 + SHA256 人工发布流程可用（v1.0.0～v1.0.3）
- [ ] build-releases.sh 版本默认值 / clean build / SUMS 自包含 —— 已登记，G4
- [ ] CI 自动构建与发布 —— M1-G4

## H. Hardware（硬件验收）

```text
NOT EXECUTED — SEPARATE TASK（M2-G1）
```

真机烧写、USB 枚举、三操作系统、BIOS / 登录界面、断连 soak：全部未执行，
独立任务处理，本清单不为它们打勾。
