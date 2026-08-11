// Package web 内嵌 ControlHub 的 Web 管理界面（单页、原生 JS、无构建步骤）。
//
// 设计：
//   - static/ 目录经 go:embed 打进二进制，部署零额外文件
//   - 由 api.Server 在 "/" 挂载（http.FileServer），与 /api/v1/* 路由共存
//   - 静态资源本身不鉴权；真正的控制调用由前端携带 Bearer Key 请求 /api/v1/*
//   - API Key 由用户在界面输入并存在浏览器 localStorage（ControlHub 本身不存前端密钥）
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/index.html static/app.js static/style.css
var staticFiles embed.FS

// Handler 返回服务 Web 静态资源的 http.Handler。
func Handler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		// 仅在 embed 指令与目录不一致时发生，属编译期错误。
		panic("web: embedded static subdir missing: " + err.Error())
	}
	return http.FileServer(http.FS(sub))
}
