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

## 管理后台（CL-5，`admin.html` + `admin/`）

独立入口（不经落地页 nav 暴露，直接访问 `admin.html`），独立 admin token。运营可在浏览器里：

- **概览**：用户数 / 订单数 / 收入 / License 数 / 未用激活码
- **用户**：全用户列表（角色标识；V1 只读，不做禁用）
- **订单**：全订单列表 + 退款（paid → refunded）
- **License**：全列表 + 禁用（DISABLED，可恢复）/ 吊销（REVOKED，不可逆）
- **套餐**：列表 + 新建/编辑（upsert）+ 上下架
- **激活码**：生成（user+device+plan 预建 UNUSED License + 12 字符码）+ 列表 + 作废

> License 吊销的离线限制：DISABLED/REVOKED 是 cloud 侧状态标记，阻止重新下载/续费；**已导入 ControlHub 的 License 离线仍有效**（本地优先设计）。真正的实时吊销（CRL）属 Phase 7 生产安全。
> 激活码消费端（ControlHub 输入码换 License）待 ControlHub 后续支持；当前 ControlHub 已有 `.license` 文件导入这条离线路径。

## 关键约束

- 支付状态以服务端回调为准，不以前端状态判定。
- 不伪造实时设备在线状态（设备实时状态属于 ControlHub 本地，云侧只持授权关系）。
- License 激活绑定 Device；UNUSED License 才可激活。

## 目录结构

```text
smart-hid-web/
├── index.html              # 产品落地页（静态）
├── app.html                # 用户门户 SPA 入口（hash 路由，零构建）
├── admin.html              # 管理后台 SPA 入口（CL-5，侧边栏布局，独立 admin token）
├── style.css               # 落地页样式（单文件，零构建，门户复用其 :root token）
├── app.js                  # 落地页交互（移动端菜单 / 滚动渐现）
├── portal/                 # 用户门户（CL-4，原生 ES Modules，零构建）
│   ├── portal.css          # 门户专用样式（表/表单/卡片/徽章/Toast/modal）
│   ├── config.js           # API_BASE（同源 /api/v1，可 localStorage 覆盖）
│   ├── api.js              # fetch 封装（token 注入 / 401 登出 / blob 下载）
│   ├── store.js            # token/user 持久化 + 订阅
│   ├── router.js           # hash 路由 + 守卫
│   ├── ui.js               # Toast / DOM helper / 状态徽章 / saveBlob
│   ├── app.js              # bootstrap
│   └── views/              # login / dashboard / plans / devices / orders / licenses / account
├── admin/                  # 管理后台（CL-5，独立 admin token，复用 portal/ui.js）
│   ├── admin.css           # 侧边栏布局 + 统计卡片
│   ├── config/store/api/router/app.js  # 与 portal 对称的精简基建
│   └── views/              # login / stats / users / orders / licenses / plans / activation-codes
├── api-docs.html           # API 文档（Swagger UI，加载 api/openapi.yaml，可在线 Try it out）
├── video.html              # 演示视频中心（配置驱动：B 站嵌入 + 多平台跳转，见页内 VIDEO_CONFIG）
├── license.html            # 授权与套餐（试用 + 设备授权 + 激活流程 + License 载荷 + FAQ）
├── assets/                 # 落地页图片（ChatGPT2API 生成）
│   ├── hero.png            # 硬件主视觉（1536×1024）
│   └── concept.png         # 控制流概念图（1536×1024）
├── api/                    # 自包含 API 契约副本（事实源在 controlhub/docs/openapi.yaml）
│   └── openapi.yaml
├── docs/                   # 文档与教程（自包含，落地页可独立部署）
│   ├── quick-start.html    # 快速开始（烧录→起 ControlHub→首条命令）
│   ├── prd.html            # 产品 PRD（marked.js 运行时渲染 *.md）
│   ├── architecture.html   # 系统架构
│   ├── roadmap.html        # 开发路线
│   ├── protocols.html      # 协议 Schema（Command/Ack/Status，内联 JSON）
│   ├── markdown-loader.js  # 通用 Markdown 加载器（fetch + marked + 自动 TOC）
│   ├── docs.css / docs.js  # 文档页样式 + scrollspy（可重入，供 loader 重建）
│   └── *.md                # PRD/架构/路线 markdown 源（落地页自包含副本）
├── downloads/              # 发布资产（有意提交；build-releases.sh 重建）
│   ├── build-releases.sh   # 一键重建全部资产（ControlHub 双平台 + 固件包 + API 契约）
│   ├── controlhub/         # ControlHub 预构建二进制（Windows/macOS）+ SHA256
│   └── firmware/           # ESP32-S3 烧录包（4 bins + flash.sh）+ SHA256
└── README.md
```

> 落地页（`index.html`）是**静态展示页**；用户门户（`app.html` + `portal/`）与管理后台（`admin.html` + `admin/`）都是**零构建 SPA**，调用 Smart HID Cloud API。
> 门户走用户自助（JWT role=user），后台走运营管理（JWT role=admin，独立 token）。

## 预览

### 落地页

直接用浏览器打开 `index.html` 即可，无需构建、无需后端：

```bash
open index.html                 # macOS
# 或本地起一个静态服务器（推荐，相对链接更顺）
python3 -m http.server 8080     # 访问 http://localhost:8080
```

### 用户门户（需 Cloud 在跑）

门户通过 `portal/api.js` 调用 Smart HID Cloud 的 `/api/v1/*`。两种方式：

**方式 A：Cloud 同源托管（推荐，零 CORS）** —— Cloud 配置 `http.web_root` 指向本目录：

```bash
# smart-hid-cloud/config.yaml
http:
  port: 17880
  web_root: ../smart-hid-web    # 指向本目录

cd ../smart-hid-cloud && go run ./cmd/cloud -config config.yaml
# 打开 http://127.0.0.1:17880/app.html
```

**方式 B：门户单独静态服务 + Cloud 跨域** —— 在 Cloud 配置 `http.cors_origins` 放行门户来源；门户端在浏览器控制台执行 `localStorage.setItem('smarthid_cloud_base','http://127.0.0.1:17880/api/v1')` 指向 Cloud。

> 门户使用原生 ES Modules（`<script type="module">`），**不能直接 `file://` 打开**，必须经 HTTP 服务器（方式 A 或任意静态服务器均可）。

### 管理后台（需 Cloud + admin 账户）

后台与门户共用 Cloud 同源托管（方式 A）。第一个 admin 的引导：

```bash
# 1. 先在 app.html 用某邮箱注册（如 admin@example.com）
# 2. Cloud config 加 admin_email，重启 → 该账户被提升为 admin
# smart-hid-cloud/config.yaml
admin_email: admin@example.com

cd ../smart-hid-cloud && go run ./cmd/cloud -config config.yaml
# 3. 打开 http://127.0.0.1:17880/admin.html，用该账户登录
```

非 admin 账户登录 `admin.html` 会被拒绝（403，自动登出）。

API 文档页（`api-docs.html`）的 Swagger UI 渲染器走 CDN，需联网；规范文件本身在 `api/openapi.yaml`，可离线查看或导入任意 OpenAPI 编辑器。

## 当前状态

- ✅ **产品落地页**（静态）：Hero / 工作原理 / 键鼠能力 / 系统组成 / 设计原则 / 文档 / 下载 / CTA / 页脚，响应式。
- ✅ **API 文档**：Swagger UI 交互式，对齐 `smart-hid-controlhub/docs/openapi.yaml`。
- ✅ **教程**：快速开始端到端指南。
- ✅ **文档中心**：`docs/` 下 PRD / 系统架构 / 开发路线 / 协议 Schema，自包含（markdown 由 marked.js 运行时渲染，带自动侧边目录）。落地页**零跨目录依赖**，可独立部署到任意静态服务器。
- ✅ **下载**：ControlHub 双平台二进制 + 固件烧录包（开发构建 `v0.1.0-scaffold`，附 SHA256）。
- ✅ **演示视频中心**：`video.html`，配置驱动。视频首发 B 站，同步抖音 / YouTube；页面内 `VIDEO_CONFIG` 填入链接即自动渲染（B 站 bvid 填入即嵌入播放器）。当前为"制作中"占位态。
- ✅ **授权与套餐**：`license.html`，试用 + 设备授权 + 7 步激活流 + License 载荷 + FAQ。
- ✅ **用户门户**（CL-4，`app.html` + `portal/`）：零构建原生 ES Module SPA，hash 路由，7 个视图 —— 登录/注册、概览、套餐、设备、订单（含 V1 模拟支付）、License（激活 + 下载 `.license`）、账户。落地页导航已加入「控制台」入口。商业闭环用户侧打通：注册 → 设备 → 选套餐 → 支付 → 激活 → 下载 `.license` → ControlHub 导入。
- ✅ **管理后台**（CL-5，`admin.html` + `admin/`）：零构建 SPA，侧边栏布局，7 视图 —— 概览统计 / 用户 / 订单（退款）/ License（禁用·吊销·恢复）/ 套餐（新建·上下架）/ 激活码（生成·作废）。独立 admin token + AdminAuthMiddleware（JWT role=admin）。

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
