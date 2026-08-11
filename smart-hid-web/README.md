# smart-hid-web

Smart HID 用户门户与管理后台。

## 角色定位

```text
Smart HID Web ──HTTPS──▶ Smart HID Cloud
```

承载账号、套餐、订单、License、设备授权、离线 License 下载。

## 用户门户

- Dashboard
- Plans（套餐）
- Devices（设备）
- Licenses（授权）
- Orders（订单）
- Downloads（下载中心）
- Account（账户）

## 管理后台

- Users
- Devices
- Plans
- Orders
- Licenses
- Activations

## 关键约束

- 支付状态以服务端回调为准，不以前端状态判定。
- 不伪造实时设备在线状态（设备实时状态属于 ControlHub 本地，云侧只持授权关系）。
- License 激活绑定 Device；UNUSED License 才可激活。

## 当前状态

⚠️ **脚手架阶段**。仅有 README 占位，未实现任何功能代码。详见 `../docs/07_SMART_HID_WEB_PRD_V1.0.md`。

## 相关

- 验收清单：`../docs/10_ACCEPTANCE_CHECKLIST.md` §F
