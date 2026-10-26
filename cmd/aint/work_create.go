package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wahhheee/aint/internal/pkg/fsx"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/work"
)

func workInit(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法：aint work init <新目录或空目录>")
	}
	dir := args[0]
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("目录不为空，请选择新目录或空目录：%s", dir)
	}
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), fsx.DirPerm); err != nil { //nolint:gosec // 目标目录直接来自本机命令行，命令职责就是在指定位置创建模板。
		return err
	}
	manifest := "format: 1\nengine: ^" + work.EngineVersion + "\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nid: local." + ids.New() + "\nversion: 0.1.0\ntitle: 我的故事\ndescription: 在这里介绍你的作品。\n"
	if err := os.WriteFile(filepath.Join(dir, "work.yaml"), []byte(manifest), fsx.PrivateFilePerm); err != nil { //nolint:gosec // 目标目录由本机命令行指定，且已确认不存在其他文件。
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "narrator.md"), []byte("# 执笔指令\n\n与玩家共同讲述一个故事。描写人物、环境与行动的后果，在需要玩家决定时给出选项。\n\n在这里补充世界、人物、文风与故事的走向。\n"), fsx.PrivateFilePerm); err != nil { //nolint:gosec // 固定文件名写入本机命令行指定的空目录。
		return err
	}
	fmt.Printf("已创建作品：%s\n使用 aint work validate 校验，或在书架中链接为开发目录。\n", dir)
	return nil
}

func workPack(args []string) error {
	fs := flag.NewFlagSet("work pack", flag.ContinueOnError)
	out := fs.String("o", "", "输出 zip 路径（默认在作品目录旁；不覆盖已有文件）")
	if err := fs.Parse(reorderFlags(args, "o")); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("用法：aint work pack <目录> [-o file.zip]")
	}
	dir, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if *out == "" {
		*out = dir + ".zip"
	}
	output, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(dir, output); err == nil && (rel == "." || filepath.IsLocal(rel)) {
		return fmt.Errorf("输出文件请放在作品目录之外，避免把打包产物收入作品")
	}
	_, rep, set, err := loadWorkFiles(dir)
	if err != nil {
		return err
	}
	for _, issue := range rep.Issues {
		fmt.Println(issue.String())
	}
	if rep.HasErrors() {
		return fmt.Errorf("作品校验失败，无法打包")
	}
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fsx.PrivateFilePerm)
	if err != nil {
		return err
	}
	err = work.WriteZip(f, set)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(output) // 写入失败的包没有可用内容。
		return err
	}
	fmt.Printf("已打包：%s\n文件树哈希：%s\n", output, set.TreeHash())
	return nil
}
