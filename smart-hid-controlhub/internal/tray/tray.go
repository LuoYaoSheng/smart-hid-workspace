// Package tray 封装系统托盘 UI。
//
// CH-P3：macOS 开发可验，Windows 交叉编译需 mingw（CH-P8）。
//
// 底层 fyne.io/systray 要求 systray.Run 在主线程调用
// （macOS NSApplication + Windows GUI subsystem 限制）。
// 因此调用方（cmd/controlhub/main.go）必须在主 goroutine 触发。
//
// 依赖注入：Controller interface，避免循环依赖 app ↔ tray。
package tray

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"

	"fyne.io/systray"
)

//go:embed assets/icon.png
var iconOnline []byte

//go:embed assets/icon-offline.png
var iconOffline []byte

// Controller 由 app.App 实现，tray 通过它驱动业务动作。
// 后续里程碑（CH-P4 LAN 模式）会扩展此接口。
type Controller interface {
	HTTPPort() int                 // 用于"打开控制台" URL
	Stop()                         // "退出"菜单触发
	RotateAPIKey() (string, error) // "重置 API Key"菜单触发
}

// Run 启动托盘，阻塞直到用户选"退出"。
// 必须在主线程调用。
func Run(c Controller, log *slog.Logger) {
	log.Info("tray starting")
	systray.Run(func() { onReady(c, log) }, func() { onExit(c, log) })
}

func onReady(c Controller, log *slog.Logger) {
	systray.SetIcon(iconOnline)
	systray.SetTitle("")
	systray.SetTooltip("Smart HID ControlHub")

	mStatus := systray.AddMenuItem("Smart HID 在线", "Status indicator")
	mStatus.Disable()

	systray.AddSeparator()

	mOpen := systray.AddMenuItem("打开控制台", "Open web console in browser")

	systray.AddSeparator()

	mRotate := systray.AddMenuItem("重置 API Key", "Rotate API key (will invalidate current key)")

	systray.AddSeparator()

	mAbout := systray.AddMenuItem("关于 Smart HID", "About")
	mQuit := systray.AddMenuItem("退出", "Quit ControlHub")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				url := fmt.Sprintf("http://127.0.0.1:%d", c.HTTPPort())
				if err := openBrowser(url); err != nil {
					log.Warn("open browser failed", "err", err, "url", url)
				}
			case <-mRotate.ClickedCh:
				raw, err := c.RotateAPIKey()
				if err != nil {
					log.Error("rotate api key from tray", "err", err)
					systray.SetTooltip("Rotate failed: " + err.Error())
					continue
				}
				// 不在托盘显示明文，避免肩窥泄漏；用户需到控制台或 initial-api-key.txt 取新 key
				log.Info("api key rotated from tray", "key_prefix", raw[:12]+"...")
				systray.SetTooltip("API key rotated. Open console with the new key.")
			case <-mAbout.ClickedCh:
				_ = openBrowser("https://smart-hid.local/about")
			case <-mQuit.ClickedCh:
				log.Info("tray quit requested")
				systray.Quit()
				return
			}
		}
	}()
}

func onExit(c Controller, log *slog.Logger) {
	log.Info("tray exiting, stopping app")
	c.Stop()
}

// openBrowser 跨平台打开默认浏览器。
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	}
	return fmt.Errorf("unsupported GOOS: %s", runtime.GOOS)
}
