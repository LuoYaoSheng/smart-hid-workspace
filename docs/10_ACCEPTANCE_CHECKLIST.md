# Smart HID V1 验收清单

## A. ControlHub

- [ ] 双击可运行
- [ ] 无 CMD 黑窗
- [ ] 托盘可打开控制台
- [ ] HTTP API 默认 localhost
- [ ] MQTT LAN 可达
- [ ] MQTT 需要认证
- [ ] 每设备 Topic ACL
- [ ] 设备在线/离线状态正确
- [ ] Device restart 能识别 boot_id 变化
- [ ] Command 同步 API 返回 executed/rejected/timeout
- [ ] LAN API 需要显式开启
- [ ] API Key 可重新生成

## B. Firmware

- [ ] Windows 能识别 Keyboard + Mouse
- [ ] keyboard tap
- [ ] hotkey
- [ ] key_down/up
- [ ] mouse move
- [ ] click
- [ ] button_down/up
- [ ] wheel
- [ ] lease 超时释放
- [ ] MQTT 断开 release_all
- [ ] request_id 去重
- [ ] target_boot_id 防旧命令
- [ ] queue full 可明确返回
- [ ] 离线不积压 Command

## C. BLE 小程序

- [ ] 原设备 Tab 不受影响
- [ ] 原广播不受影响
- [ ] 关于不受影响
- [ ] HID Tab 存在
- [ ] 不要求设备二维码
- [ ] 能搜索附近 Smart HID
- [ ] 能 BLE 连接并读取 Device Info
- [ ] 能扫描 ControlHub 动态二维码
- [ ] Wi-Fi 列表来自 ESP32
- [ ] Wi-Fi 错误只退回 Wi-Fi 步骤
- [ ] Pair Token 过期只重新获取 ControlHub QR
- [ ] 配网结果可诊断
- [ ] 小程序无会员/订单/License 页面

## D. Trial

- [ ] 安装 ControlHub 不立刻开始倒计时
- [ ] 第一条 executed command 才开始 Session
- [ ] 无操作不消耗
- [ ] 配置/状态不消耗
- [ ] Trial expired 后配置仍可用
- [ ] 简单重装 ControlHub 不应直接获得全新 Trial

## E. License

- [ ] ControlHub 只含 Public Key
- [ ] Cloud Private Key 不下发
- [ ] License 与 Device ID 匹配
- [ ] 过期正确拒绝
- [ ] 离线可导入
- [ ] 云不可用时已有有效 License 继续工作
- [ ] 续费后可重新刷新/导入新 License

## F. Web

- [ ] 登录
- [ ] 套餐
- [ ] 下单
- [ ] 支付状态以服务端回调为准
- [ ] UNUSED License
- [ ] 激活绑定 Device
- [ ] 下载离线 License
- [ ] 订单
- [ ] 下载中心
- [ ] 不伪造实时设备在线状态
