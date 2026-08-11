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
├── index.html              # 产品落地页（静态）
├── style.css               # 落地页样式（单文件，零构建）
├── app.js                  # 落地页交互（移动端菜单 / 滚动渐现）
├── api-docs.html           # API 文档（Swagger UI，加载 api/openapi.yaml，可在线 Try it out）
├── assets/                 # 落地页图片（ChatGPT2API 生成）
│   ├── hero.png            # 硬件主视觉（1536×1024）
│   └── concept.png         # 控制流概念图（1536×1024）
├── api/                    # 自包含 API 契约副本（事实源在 controlhub/docs/openapi.yaml）
│   └── openapi.yaml
├── docs/                   # 教程
│   ├── quick-start.html    # 快速开始（烧录→起 ControlHub→首条命令）
│   ├── docs.css            # 文档页样式（侧边目录 / 代码块 / callout）
│   └── docs.js             # 文档页 scrollspy
├── downloads/              # 发布资产（有意提交；build-releases.sh 重建）
│   ├── build-releases.sh   # 一键重建全部资产（ControlHub 双平台 + 固件包 + API 契约）
│   ├── controlhub/         # ControlHub 预构建二进制（Windows/macOS）+ SHA256
│   └── firmware/           # ESP32-S3 烧录包（4 bins + flash.sh）+ SHA256
└── README.md
```

> 落地页是**静态展示页**，与上方的「用户门户 / 管理后台」（未来云端 Web 应用）是两件事。
> 门户/后台属后续 Phase，仍为占位。

## 预览

直接用浏览器打开 `index.html` 即可，无需构建、无需后端：

```bash
open index.html                 # macOS
# 或本地起一个静态服务器（推荐，相对链接更顺）
python3 -m http.server 8080     # 访问 http://localhost:8080
```

API 文档页（`api-docs.html`）的 Swagger UI 渲染器走 CDN，需联网；规范文件本身在 `api/openapi.yaml`，可离线查看或导入任意 OpenAPI 编辑器。

## 当前状态

- ✅ **产品落地页**（静态）：Hero / 工作原理 / 键鼠能力 / 系统组成 / 设计原则 / 文档 / 下载 / CTA / 页脚，响应式。
- ✅ **API 文档**：Swagger UI 交互式，对齐 `smart-hid-controlhub/docs/openapi.yaml`。
- ✅ **教程**：快速开始端到端指南。
- ✅ **下载**：ControlHub 双平台二进制 + 固件烧录包（开发构建 `v0.1.0-scaffold`，附 SHA256）。
- ⚠️ **用户门户 / 管理后台**：脚手架阶段，未实现功能代码。详见 `../docs/07_SMART_HID_WEB_PRD_V1.0.md`。

## 图片资产说明

`assets/` 下的图片由自托管生图服务（`c2a.i2kai.top`，ChatGPT2API，模型 `gpt-image-2`）生成，
作为占位产品视觉。后续替换为真实产品摄影时，保持文件名与尺寸即可，无需改动 HTML。

## 重建发布资产

改了 ControlHub 源码或固件后，重建下载区的二进制：

```bash
./downloads/build-releases.sh     # 重建 ControlHub + 固件包 + 同步 openapi.yaml 副本
```

## 相关

- 验收清单：`../docs/10_ACCEPTANCE_CHECKLIST.md` §F
