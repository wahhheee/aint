// Package vfs 是主笔使用的虚拟文件系统与检索（《02-技术方案》第 12 节）。
//
// 挂载表：
//
//	/work/lore/**      作品设定（只读）
//	/work/assets/**    作品素材（只读；读取图片即"看图"）
//	/memory/**         本存档记忆（读写；回合内为写时复制覆盖层，随回合提交或作废）
//	/uploads/**        进度线上玩家上传的图片（只读）
//	/transcript/NNNN   进度线上第 NNNN 回合的原文（只读，虚拟文件）
//
// 安全边界：工具只能访问这里列出的路径；写操作仅限 /memory。所有路径先经 Clean 校验。
// 分支隔离：/memory 与 /transcript 都只反映当前进度线（不变量 3）。
package vfs

import (
	"context"
	"fmt"
	"path"
	"strings"
)

// 记忆写入约束（12.2）。
const (
	MaxMemoryFile  = 32 << 10
	MaxMemoryTotal = 2 << 20
	MaxMemoryDepth = 6
	MaxNameLen     = 80
)

// Clean 校验并规范化 VFS 路径：必须是绝对路径、位于已知挂载点下、不含 ".."。
func Clean(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("路径需要以 / 开头，如 /memory/index.md")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", fmt.Errorf("路径中不能含 ..")
		}
	}
	p = path.Clean(p)
	for _, m := range []string{"/work", "/memory", "/uploads", "/transcript"} {
		if p == m || strings.HasPrefix(p, m+"/") {
			return p, nil
		}
	}
	if p == "/" {
		return p, nil
	}
	return "", fmt.Errorf("%s 不在可访问的目录中，可用的目录有 /work、/memory、/transcript、/uploads", p)
}

// Writable 判断路径是否可写（仅 /memory 下的 .md / .txt / .yaml）。
func Writable(p string) error {
	if !strings.HasPrefix(p, "/memory/") {
		return fmt.Errorf("只能写入 /memory 下的文件")
	}
	switch path.Ext(p) {
	case ".md", ".txt", ".yaml":
	default:
		return fmt.Errorf("记忆文件可以用 .md、.txt 或 .yaml 扩展名")
	}
	if strings.Count(p, "/")-1 > MaxMemoryDepth {
		return fmt.Errorf("目录层级最多 %d 层", MaxMemoryDepth)
	}
	if len([]rune(path.Base(p))) > MaxNameLen {
		return fmt.Errorf("文件名最多 %d 个字符", MaxNameLen)
	}
	return nil
}

// Entry 是目录列表中的一项。
type Entry struct {
	Path    string
	IsDir   bool
	Size    int
	Summary string // 一行说明（frontmatter summary 或首个非空行）
}

// ReadResult 是读取结果。
type ReadResult struct {
	Text       string
	FromLine   int
	ToLine     int
	TotalLines int
	ImageBlob  string // 图片文件：blob 哈希（由运行器转成图片块）
}

// Query 是检索参数。
type Query struct {
	Text   string
	Prefix string
	Regex  bool
	Limit  int
}

// Hit 是一条检索结果。
type Hit struct {
	Path    string
	Line    int
	Snippet string
}

// FS 是主笔看到的文件系统（每个回合草稿一个实例）。
//
// TODO(M3-1/M3-3/M3-4)：实现（挂载作品文件——读取 /work/lore 时经 work.Model.LinkDocs 补全 [[id]] 链接（D-35）；
// 记忆覆盖层、原文虚拟文件、上传图片；检索排序：
// 记忆 > 作品资料 > 原文（新→旧），每个文件最多 5 条，总计 ≤ 50 条 / 约 6,000 字符；别名扩展）。
type FS interface {
	List(ctx context.Context, p string, depth int) ([]Entry, error)
	Read(ctx context.Context, p string, offset, limit int) (*ReadResult, error)
	Search(ctx context.Context, q Query) ([]Hit, error)
	Write(ctx context.Context, p, content string) error
	Edit(ctx context.Context, p, oldText, newText string, replaceAll bool) (replaced int, err error)
	Remove(ctx context.Context, p string) error
}
