package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/wahhheee/yanyi/internal/access"
	"github.com/wahhheee/yanyi/internal/app"
	"github.com/wahhheee/yanyi/internal/provider"
)

func runPassword(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法：yanyi password set|clear [--data 目录] [--stdin]")
	}
	sub := args[0]
	if sub != "set" && sub != "clear" {
		return fmt.Errorf("密码操作应为 set 或 clear")
	}
	fs := flag.NewFlagSet("password", flag.ContinueOnError)
	dir := fs.String("data", "", "数据目录")
	stdin := fs.Bool("stdin", false, "从标准输入读取一行密码（用于脚本）")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("密码不能通过命令行参数传入，请在提示后输入")
	}
	dirs, err := app.ResolveDirs(*dir)
	if err != nil {
		return err
	}
	if app.RunningInstance(context.Background(), dirs) != "" {
		return fmt.Errorf("请先停止此数据目录的演义服务，再修改访问密码")
	}
	secrets, err := provider.OpenSecrets(dirs.Secrets)
	if err != nil {
		return err
	}
	if sub == "clear" {
		return secrets.Delete("access")
	}
	svc, err := access.New(secrets)
	if err != nil {
		return err
	}
	var password string
	if *stdin {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 1025), 1025)
		if !scanner.Scan() {
			return fmt.Errorf("未读取到密码")
		}
		password = strings.TrimSuffix(scanner.Text(), "\r")
	} else {
		fmt.Fprint(os.Stderr, "新访问密码：")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		password = string(b)
		fmt.Fprint(os.Stderr, "再次输入：")
		confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		if password != string(confirm) {
			return fmt.Errorf("两次输入的密码不一致")
		}
	}
	if password == "" {
		return fmt.Errorf("密码不能为空；清除密码请使用 password clear")
	}
	return svc.SetPassword("", password, true)
}
