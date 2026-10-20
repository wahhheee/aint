// Package buildinfo 提供程序版本信息。
//
// 发布构建通过 -ldflags 注入：
//
//	-X github.com/wahhheee/yanyi/internal/buildinfo.Version=1.0.0
//	-X github.com/wahhheee/yanyi/internal/buildinfo.Commit=abc1234
//
// 未注入时（go run / 开发构建）从 debug.ReadBuildInfo 的 VCS 信息兜底。
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"sync"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// Info 汇总版本信息，用于 `yanyi version`、/api/v1/system/info 与日志。
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
	Modified  bool   `json:"modified"` // 工作区有未提交改动时构建
}

var (
	once sync.Once
	info Info
)

// Get 返回版本信息（首次调用时计算并缓存）。
func Get() Info {
	once.Do(func() {
		info = Info{
			Version:   Version,
			Commit:    Commit,
			Date:      Date,
			GoVersion: runtime.Version(),
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		}
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = s.Value
				}
			case "vcs.time":
				if info.Date == "" {
					info.Date = s.Value
				}
			case "vcs.modified":
				info.Modified = s.Value == "true"
			}
		}
		if len(info.Commit) > 12 {
			info.Commit = info.Commit[:12]
		}
	})
	return info
}
