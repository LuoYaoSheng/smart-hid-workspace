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

## 目录结构

```text
smart-hid-web/
├── index.html          # 产品落地页（静态）
├── style.css           # 落地页样式（单文件，零构建）
├── app.js              # 落地页交互（移动端菜单 / 滚动渐现）
├── assets/             # 落地页图片（ChatGPT2API 生成）
│   ├── hero.png        # 硬件主视觉（1536×1024）
│   └── concept.png     # 控制流概念图（1536×1024）
└── README.md
```

> 落地页是**静态展示页**，与上方的「用户门户 / 管理后台」（未来云端 Web 应用）是两件事。
> 门户/后台属后续 Phase，仍为占位。

## 预览

直接用浏览器打开 `index.html` 即可，无需构建、无需后端：

```bash
open index.html                 # macOS
# 或本地起一个静态服务器
python3 -m http.server 8080     # 访问 http://localhost:8080
```

## 当前状态

- ✅ **产品落地页**（静态）：已完成。Hero / 工作原理 / 键鼠能力 / 系统组成 / 设计原则 / CTA / 页脚，响应式。
- ⚠️ **用户门户 / 管理后台**：脚手架阶段，未实现功能代码。详见 `../docs/07_SMART_HID_WEB_PRD_V1.0.md`。

## 图片资产说明

`assets/` 下的图片由自托管生图服务（`c2a.i2kai.top`，ChatGPT2API，模型 `gpt-image-2`）生成，
作为占位产品视觉。后续替换为真实产品摄影时，保持文件名与尺寸即可，无需改动 HTML。

## 相关

- 验收清单：`../docs/10_ACCEPTANCE_CHECKLIST.md` §F
