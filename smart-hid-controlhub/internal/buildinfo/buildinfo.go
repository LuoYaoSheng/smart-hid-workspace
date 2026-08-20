// Package buildinfo 承载构建期注入的版本元数据（M1-G4 唯一版本源 = 根 VERSION）。
//
// ldflags 注入（见 smart-hid-web/downloads/build-releases.sh 与 release.yml）：
//
//	-X smart-hid-controlhub/internal/buildinfo.Version=<VERSION 文件>
//	-X smart-hid-controlhub/internal/buildinfo.Commit=<git sha>
//	-X smart-hid-controlhub/internal/buildinfo.Date=<RFC3339>
//	-X smart-hid-controlhub/internal/buildinfo.Dirty=<true|false>
//
// 未注入时（go test / 开发构建）保持 "dev" 占位，不 panic、不影响测试。
package buildinfo

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
	Dirty   = "false"
)

// Summary 返回一行可读的版本串（启动横幅 / 日志 / -version 输出共用）。
func Summary() string {
	return fmt.Sprintf("Smart HID ControlHub version=%s commit=%s date=%s dirty=%s",
		Version, Commit, Date, Dirty)
}
