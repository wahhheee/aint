package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/wahhheee/yanyi/internal/app"
)

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var o app.Options
	fs.StringVar(&o.DataDir, "data", "", "数据目录（默认：程序所在目录下的 yanyi-data/）")
	fs.StringVar(&o.Listen, "listen", "", "监听地址（默认沿用设置，首次为 127.0.0.1）；0.0.0.0 允许局域网访问")
	fs.IntVar(&o.Port, "port", 0, "端口（默认从 7860 起自动选择）")
	fs.BoolVar(&o.NoBrowser, "no-browser", false, "启动后不自动打开浏览器")
	fs.StringVar(&o.FakeLLM, "fake-llm", "", `使用假模型："demo" 为内置演示脚本，或假模型脚本文件路径（开发、测试、复现问题用）`)
	fs.BoolVar(&o.TraceLLM, "trace-llm", false, "把每次模型调用的请求与流事件写入 <数据目录>/logs/llm-trace/（调试用，含正文）")
	fs.StringVar(&o.LogLevel, "log-level", "info", "日志级别：debug | info | warn | error")
	_ = fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return app.Run(ctx, o)
}
