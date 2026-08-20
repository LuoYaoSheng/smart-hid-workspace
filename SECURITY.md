# Security Policy

## 支持版本

仅支持最新发布版本（见 [Releases](https://github.com/LuoYaoSheng/smart-hid-workspace/releases)）。

## 设计边界

Smart HID 是**纯本地**系统：控制链路不出局域网，没有云端组件。安全边界
设计见 [docs/current/ARCHITECTURE.md](docs/current/ARCHITECTURE.md)；已知
风险与加固排期见 [docs/current/HARDENING_BACKLOG.md](docs/current/HARDENING_BACKLOG.md)
（按 Gate 逐项修复，不隐藏问题）。

## 报告漏洞

请**不要**在公开 Issue 中提交安全漏洞细节。发送邮件至
[lys1988_cool@126.com](mailto:lys1988_cool@126.com)，包含：

- 受影响组件（ControlHub / 固件 / Web）
- 复现步骤或 PoC
- 影响评估

会在可行时间内回复。修复后将发布新版本并致谢（除非你希望匿名）。

## 部署注意

- ControlHub 控制 API 默认只监听 `127.0.0.1`；开启 LAN 模式前请确认网络环境
- `mqtt.password` 默认值仅供开发，正式使用务必修改
- 首次启动生成的 API Key 会写入 `data_dir/initial-api-key.txt` 并打印一次日志，保存后请删除该文件
