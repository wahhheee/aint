// 命令 yanyi 是演义的唯一入口：本地服务 + 创作者工具。
//
//	yanyi [serve] [选项]           启动服务并打开浏览器（默认命令）
//	yanyi work validate <目录>     校验作品（也接受 zip 包）
//	yanyi work prompt <目录>       打印按需组装后的完整提示词与工具定义
//	yanyi work init <目录>         生成作品模板（TODO(M6-3)）
//	yanyi work pack <目录>         确定性打包（TODO(M6-3)）
//	yanyi password set|clear       访问密码（TODO(M6-9)）
//	yanyi version                  版本信息
package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}

	var err error
	switch cmd {
	case "serve":
		err = runServe(args)
	case "work":
		err = runWork(args)
	case "password":
		err = fmt.Errorf("yanyi password 尚未实现（TODO(M6-9)）")
	case "version":
		err = runVersion(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令：%s\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误：", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `演义（Yanyi）—— 以作品为单位的 AI 互动小说运行时

用法：
  yanyi [serve] [选项]          启动服务并打开浏览器（默认命令；yanyi serve -h 查看选项）
  yanyi work validate <目录>    校验作品目录或 zip 包（--json 输出机器可读结果）
  yanyi work prompt <目录>      打印按需组装后的提示词与工具定义（--vision 模拟看图模型）
  yanyi work init <目录>        生成作品模板（尚未实现）
  yanyi work pack <目录>        确定性打包为 zip（尚未实现）
  yanyi password set|clear      设置或清除访问密码（尚未实现）
  yanyi version                 版本信息
`)
}
