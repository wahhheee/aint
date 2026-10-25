// schema-gen 从作品输入结构生成随发布包分发的编辑器 Schema。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wahhheee/yanyi/internal/pkg/fsx"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/work"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	if err := os.MkdirAll("schemas", fsx.DirPerm); err != nil {
		return err
	}
	for name, schema := range work.JSONSchemas() {
		b, err := jsonx.MarshalIndent(schema)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join("schemas", name+".schema.json"), append(b, '\n'), fsx.PrivateFilePerm); err != nil {
			return err
		}
	}
	return nil
}
