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
package main

import (
	"flag"
	"fmt"
	"os"

	"smart-hid-controlhub/internal/app"
)

func main() {
	cfgPath := flag.String("config", "", "path to config.yaml (default: built-in defaults)")
	trayMode := flag.Bool("tray", false, "run with system tray (CH-P3); default is headless")
	flag.Parse()

	var err error
	if *trayMode {
		err = app.RunWithTray(*cfgPath)
	} else {
		err = app.Run(*cfgPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "controlhub: %v\n", err)
		os.Exit(1)
	}
}
