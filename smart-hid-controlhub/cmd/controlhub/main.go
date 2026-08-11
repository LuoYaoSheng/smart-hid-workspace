// Command controlhub 启动 Smart HID ControlHub。
// 用法：
//   controlhub                    # 用默认配置
//   controlhub -config config.yaml
//   controlhub -h
package main

import (
	"flag"
	"fmt"
	"os"

	"smart-hid-controlhub/internal/app"
)

func main() {
	cfgPath := flag.String("config", "", "path to config.yaml (default: built-in defaults)")
	flag.Parse()

	if err := app.Run(*cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "controlhub: %v\n", err)
		os.Exit(1)
	}
}
