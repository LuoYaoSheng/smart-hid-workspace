// Command controlhub 启动 Smart HID ControlHub。
//
// 用法：
//
//	controlhub                    # headless 模式（信号循环），向后兼容
//	controlhub -tray              # 系统托盘模式（CH-P3，主线程跑 systray）
//	controlhub -config config.yaml
//	controlhub -h
//
// CH-P3：tray 模式下，主 goroutine 跑 fyne.io/systray 事件循环
// （macOS NSApplication + Windows GUI subsystem 要求）；headless 模式
// 保持原行为，便于服务化与单元测试。
//
// 退出码约定（1.2.0 观测性）：0 = 正常退出；1 = 启动/运行错误；
// 2 = panic（app 层已捕获落日志的 *PanicError，或此处兜底 recover）。
// 事件查看器 / 服务管理器凭退出码即可归因。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"smart-hid-controlhub/internal/app"
	"smart-hid-controlhub/internal/buildinfo"
)

func main() {
	// 兜底 recover：Build 阶段等 app.Run 尚未接管的 panic。
	// 运行期 panic 由 app.Run/RunWithTray 捕获（能落文件日志）。
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "controlhub: panic: %v\n%s", r, debug.Stack())
			os.Exit(2)
		}
	}()

	cfgPath := flag.String("config", "", "path to config.yaml (default: built-in defaults)")
	trayMode := flag.Bool("tray", false, "run with system tray (CH-P3); default is headless")
	showVer := flag.Bool("version", false, "print version/build info and exit")
	flag.Parse()

	if *showVer {
		fmt.Println(buildinfo.Summary())
		return
	}
	// 启动横幅：版本事实源 = 根 VERSION（M1-G4 ldflags 注入）
	fmt.Println(buildinfo.Summary())

	var err error
	if *trayMode {
		err = app.RunWithTray(*cfgPath)
	} else {
		err = app.Run(*cfgPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "controlhub: %v\n", err)
		var pe *app.PanicError
		if errors.As(err, &pe) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
