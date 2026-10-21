package app

import (
	"context"
	"log/slog"
	"os/exec"
	"runtime"
)

// openBrowser 用系统默认浏览器打开 url（失败只记日志：玩家仍可手动打开终端里打印的地址）。
func openBrowser(ctx context.Context, url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	// url 由本进程根据监听地址拼出（http://127.0.0.1:端口），不来自外部输入。
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // G204：url 由本进程生成
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url) //nolint:gosec // G204：url 由本进程生成
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url) //nolint:gosec // G204：url 由本进程生成
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("无法自动打开浏览器，请手动访问", "url", url, "err", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}
