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

//go:embed static/*
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

// consolePaths / demoPaths 是可独立关闭的静态资源（config.web.*）。
var consolePaths = map[string]bool{
	"/": true, "/index.html": true, "/app.js": true, "/style.css": true,
	"/api-test.html": true, "/api-test.js": true,
}
var demoPaths = map[string]bool{"/demo.html": true, "/demo.js": true}

// Gated 在 Handler 外按 config.web 门禁：关闭的页面返回 404。
// console 关闭时控制台与其资源 404；demo 关闭时演示台 404；其余资源照常。
func Gated(console, demo bool) http.Handler {
	inner := Handler()
	if console && demo {
		return inner
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "" {
			p = "/"
		}
		if consolePaths[p] && !console {
			http.NotFound(w, r)
			return
		}
		if demoPaths[p] && !demo {
			http.NotFound(w, r)
			return
		}
		inner.ServeHTTP(w, r)
	})
}
