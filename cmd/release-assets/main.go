// release-assets 校验并打包发布包内的示例作品，不写入运行时数据。
package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wahhheee/yanyi/examples"
	"github.com/wahhheee/yanyi/internal/pkg/fsx"
	"github.com/wahhheee/yanyi/internal/work"
)

func main() {
	if err := prepare(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepare() error {
	if err := os.MkdirAll(".release/examples", fsx.DirPerm); err != nil {
		return err
	}
	usage, err := os.ReadFile("docs/release-usage.md")
	if err != nil {
		return err
	}
	if err := os.WriteFile(".release/使用说明.md", usage, fsx.PrivateFilePerm); err != nil { //nolint:gosec // 输出路径是固定的发布暂存文件，不包含外部路径输入。
		return err
	}
	entries, err := fs.ReadDir(examples.Works(), ".")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := fs.Sub(examples.Works(), e.Name())
		if err != nil {
			return err
		}
		set, err := work.FromFS(sub, work.DefaultLimits)
		if err != nil {
			return err
		}
		_, report := work.Compile(set)
		if report.HasErrors() {
			return fmt.Errorf("示例 %s 校验失败：%v", e.Name(), report.Issues)
		}
		var out bytes.Buffer
		if err := work.WriteZip(&out, set); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(".release/examples", e.Name()+".zip"), out.Bytes(), fsx.PrivateFilePerm); err != nil {
			return err
		}
	}
	return nil
}
