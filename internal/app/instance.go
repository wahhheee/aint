package app

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/wahhheee/aint/internal/pkg/fsx"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// instanceInfo 写在 <数据目录>/instance.json，用于"同一数据目录只运行一个实例"（SET-06）。
//
// 检测方式只用标准库、不依赖系统文件锁：读取文件 → 请求其中地址的 /api/v1/system/health →
// 返回相同实例 ID 说明程序已在运行（直接打开其页面后退出）；否则视为残留文件，覆盖后正常启动。
// 运行时"单一写者"的假设依赖于此。
type instanceInfo struct {
	InstanceID string `json:"instance_id"`
	PID        int    `json:"pid"`
	URL        string `json:"url"`
	StartedAt  int64  `json:"started_at"`
}

// runningInstance 返回该数据目录下正在运行的实例地址（没有则返回空）。
func runningInstance(ctx context.Context, path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var info instanceInfo
	if jsonx.Unmarshal(b, &info) != nil || info.URL == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, info.URL+"/api/v1/system/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }() // 只读一个小响应体，关闭错误无影响
	var health struct {
		InstanceID string `json:"instance_id"`
	}
	if jsonx.UnmarshalRead(resp.Body, &health) != nil || health.InstanceID != info.InstanceID {
		return ""
	}
	return info.URL
}

// RunningInstance 返回数据目录中仍在运行的服务地址，供离线维护命令检测冲突。
func RunningInstance(ctx context.Context, dirs Dirs) string {
	return runningInstance(ctx, dirs.Instance)
}

func writeInstance(path string, info instanceInfo) error {
	b, err := jsonx.MarshalIndent(info)
	if err != nil {
		return err
	}
	return fsx.WriteFileAtomic(path, b, 0o644)
}
