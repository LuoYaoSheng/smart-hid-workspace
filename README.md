# smart-hid-web

Smart HID 官网：产品落地页 + 文档站 + 下载中心。

## 组成

```text
index.html        落地页（定位 / 工作原理 / 能力 / 下载 / 反馈入口）
video.html        演示视频中心
api-docs.html     ControlHub HTTP API 文档（Swagger UI，加载 api/openapi.yaml）
docs/             文档站（快速开始 / 架构 / PRD / 协议 / 开发路线）
downloads/        预编译发行物（ControlHub 双平台 + 固件烧录包）+ 构建脚本
assets/           图片
```

## 特性

- **零构建**：纯静态 HTML/CSS/JS（ES5），无任何依赖与打包步骤，克隆即用
- **零后端**：反馈与需求通过 GitHub / Gitee Issues 承接，页面无任何 API 依赖
- **全相对路径**：任意静态托管可用（GitHub Pages / 自托管）

## 本地预览

```bash
cd smart-hid-web && python3 -m http.server 8090
# 浏览器打开 http://127.0.0.1:8090
```

## 在线地址

- 官网：<https://luoyaosheng.github.io/smart-hid-workspace/>
- 源仓库：<https://github.com/LuoYaoSheng/smart-hid-workspace>（Gitee 同步镜像）

## 重新生成交付物

`downloads/build-releases.sh`：从工作区源码交叉编译 ControlHub 双平台二进制、复制固件 bin、生成 SHA256SUMS、并把 ControlHub openapi.yaml 投影到 `api/openapi.yaml`。

## 关键约束

- 落地页保持 100% 静态零构建，不引入任何前端框架或 CDN 运行时依赖
- `api/openapi.yaml` 是 ControlHub API 的投影副本，改契约先改 `smart-hid-controlhub/docs/openapi.yaml` 再跑构建脚本
