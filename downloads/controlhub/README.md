# Smart HID ControlHub 二进制

Windows / macOS 预构建（已 strip 符号，未签名）。版本/commit/校验和见上级目录 `manifest.json`；`./controlhub-darwin-arm64 -version` 可查版本。

| 文件 | 平台 | 大小 |
|---|---|---|
| `controlhub-windows-amd64.exe` | Windows x86_64 | ~12 MB |
| `controlhub-darwin-arm64` | macOS Apple Silicon | ~11 MB |

> Linux / Windows ARM 等暂未提供，可用 `build-releases.sh` 自行交叉编译。

## 运行

```bash
# Windows (PowerShell / CMD)
.\controlhub-windows-amd64.exe
.\controlhub-windows-amd64.exe -config config.yaml

# macOS
chmod +x controlhub-darwin-arm64
./controlhub-darwin-arm64
```

启动后：
- HTTP API：`http://localhost:17890`
- 内嵌 MQTT Broker：`localhost:17891`
- Web 控制台：浏览器打开 `http://localhost:17890/`
- 系统托盘：双击运行即驻留（V1 不做 Windows Service）

## 鉴权

首次运行会在 `data/` 生成 API Key（`chk_` 前缀）。查 `config.yaml` 或启动日志取 Key，所有 `/api/v1/*` 请求带：

```
Authorization: Bearer chk_xxxxxxxxxxxxxxxx
```

## 校验

```bash
shasum -a 256 -c SHA256SUMS
```

## 从源码构建

```bash
cd smart-hid-controlhub
go build ./cmd/controlhub                      # 本机
GOOS=windows GOARCH=amd64 go build ./cmd/controlhub   # 交叉编译
```

或用统一发布脚本：`../build-releases.sh`

> ⚠️ macOS 首次运行可能提示"无法验证开发者"。这是未签名构建，右键→打开 即可。生产版将做代码签名。
