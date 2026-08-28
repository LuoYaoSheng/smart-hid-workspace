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
	"strings"

	"fyne.io/systray"
)

//go:embed assets/icon.png
var iconOnline []byte

//go:embed assets/icon-offline.png
var iconOffline []byte

// Controller 由 app.App 实现，tray 通过它驱动业务动作。
type Controller interface {
	HTTPPort() int                 // 用于"打开控制台" URL
	Stop()                         // "退出"菜单触发
	RotateAPIKey() (string, error) // "重置 API Key"菜单触发
	InitialAPIKey() (string, bool) // "复制 API Key"菜单取明文（bool=文件存在）
	LANModeEnabled() bool          // LAN 模式 checkbox 当前状态
	SetLANMode(bool) error         // LAN 模式 toggle（持久化，下次启动生效）
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

	mCopyKey := systray.AddMenuItem("复制 API Key",
		"Copy the initial API key to clipboard (from initial-api-key.txt)")

	systray.AddSeparator()

	mRotate := systray.AddMenuItem("重置 API Key", "Rotate API key (will invalidate current key)")
	mLAN := systray.AddMenuItemCheckbox("LAN 模式",
		"Allow LAN access to HTTP API (restart required)",
		c.LANModeEnabled())

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
			case <-mCopyKey.ClickedCh:
				raw, ok := c.InitialAPIKey()
				if !ok {
					systray.SetTooltip("Key 文件不存在（可能已删除）。请用「重置 API Key」生成新的。")
					continue
				}
				if err := copyToClipboard(raw); err != nil {
					log.Warn("copy api key failed", "err", err)
					systray.SetTooltip("复制失败：" + err.Error())
					continue
				}
				log.Info("api key copied to clipboard (user-initiated)")
				systray.SetTooltip("API Key 已复制到剪贴板。")
			case <-mRotate.ClickedCh:
				raw, err := c.RotateAPIKey()
				if err != nil {
					log.Error("rotate api key from tray", "err", err)
					systray.SetTooltip("Rotate failed: " + err.Error())
					continue
				}
				// 轮换后 app 层已回写 initial-api-key.txt，"复制 API Key"取到的是新 Key
				log.Info("api key rotated from tray", "key_prefix", raw[:12]+"...")
				systray.SetTooltip("API key 已轮换。菜单「复制 API Key」可取新 Key。")
			case <-mLAN.ClickedCh:
				newState := !c.LANModeEnabled()
				if err := c.SetLANMode(newState); err != nil {
					log.Error("set lan mode from tray", "err", err)
					systray.SetTooltip("LAN toggle failed: " + err.Error())
					continue
				}
				if newState {
					mLAN.Check()
					systray.SetTooltip("LAN mode ON (restart to apply). HTTP API will be 0.0.0.0.")
				} else {
					mLAN.Uncheck()
					systray.SetTooltip("LAN mode OFF (restart to apply). HTTP API will be localhost-only.")
				}
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

// copyToClipboard 跨平台复制文本，零新增依赖：
// Windows 用系统自带 clip.exe，macOS 用 pbcopy，Linux 用 xclip/xsel。
func copyToClipboard(text string) error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("cmd", "/c", "clip")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	case "linux":
		for _, c := range [][]string{
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		} {
			cmd := exec.Command(c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		return fmt.Errorf("no clipboard tool (install xclip or xsel)")
	}
	return fmt.Errorf("unsupported GOOS: %s", runtime.GOOS)
}
