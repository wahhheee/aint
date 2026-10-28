// Package examples 内嵌示例作品：首次启动即可在书架上看到。
//
// 每个 works/ 下的顶层目录是一部作品。修改示例作品后，启动时会按新的文件树哈希导入为新版本，
// 旧存档继续使用旧版本（与玩家导入新版本的行为一致）。
package examples

import (
	"embed"
	"io/fs"
)

//go:embed all:works
var works embed.FS

// Works 返回示例作品文件系统，根目录下每个子目录是一部作品。
func Works() fs.FS {
	sub, err := fs.Sub(works, "works")
	if err != nil {
		panic(err) // 只在构建配置错误时发生
	}
	return sub
}
