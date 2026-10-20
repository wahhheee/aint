package app

import (
	"log/slog"
	"os/exec"
	"runtime"
)

// openBrowser 用系统默认浏览器打开 url（失败只记日志：玩家仍可手动打开终端里打印的地址）。
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("无法自动打开浏览器，请手动访问", "url", url, "err", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}
