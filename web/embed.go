// Package web 内嵌前端构建产物（web/dist，由 `pnpm -C web build` 生成）。
//
// dist/ 中常驻一个 .gitkeep，保证未构建前端时 Go 代码也能编译；
// 此时服务端会显示一个说明页，提示如何启动前端开发服务器或构建前端（见 server/static.go）。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist 返回前端静态资源文件系统（根目录即 dist/）。
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // 只在构建配置错误时发生
	}
	return sub
}
