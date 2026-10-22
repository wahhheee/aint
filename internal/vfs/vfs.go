// Package vfs 是主笔使用的虚拟文件系统与检索（《02-技术方案》第 12 节）。
//
// 挂载表（每个回合由 runtime 组装，见 runtime/vfs.go）：
//
//	/work/lore/**      作品设定（只读；读取设定文档时补全 [[id]] 链接，D-35）
//	/work/assets/**    作品素材（只读；读取图片即"看图"）
//	/memory/**         本存档记忆（读写；回合内为写时复制覆盖层，随回合提交或作废）
//	/uploads/**        进度线上玩家上传的图片（只读）
//	/transcript/NNNN   进度线上第 NNNN 回合的原文（只读，虚拟文件）
//
// 本包只做与来源无关的部分：路径校验、挂载分派、目录推导、按行读取与截断、文件类型判断、相近路径建议。
// 文件从哪里来由 Source 决定（作品文件见 work.Service.Source，记忆见 MemorySource），
// 给模型看的文字由 tools 包组织（本包的错误只陈述事实，附带改正所需的信息）。
//
// 安全边界：工具只能访问挂载表中的路径；所有路径先经 Clean 校验，读取只按来源给出的文件清单查找，
// 不触及真实文件系统。写操作仅限 /memory。
// 分支隔离：/memory 与 /transcript 都只反映当前进度线（不变量 3）。
package vfs

import (
	"context"
	"errors"
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

// 读取与列目录的默认值与上限（10.2）。
const (
	DefaultReadLines = 400
	MaxReadLines     = 1000
	// MaxReadRunes 是一次读取返回的正文字数上限：给结果的标题与续读提示留出余量，
	// 使 read 的结果不会再被工具结果的总上限（tools.MaxResultChars = 8000）从中间截断。
	MaxReadRunes  = 7000
	MaxListDepth  = 3
	MaxListShown  = 200 // 列目录最多显示的条目数（tools 据此截断并注明）
	maxSuggestion = 3
)

// Clean 校验并规范化 VFS 路径：必须是以 / 开头的绝对路径、不含 ".."。
// 是否位于已挂载的目录下由 Table 判断（挂载表随作品与模型能力变化）。
func Clean(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" {
		return "", errors.New("路径为空")
	}
	if !strings.HasPrefix(p, "/") {
		return "", errors.New("路径需要以 / 开头")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", errors.New("路径中不能含有上级目录（..）")
		}
	}
	return path.Clean(p), nil
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
	Path        string // 完整 VFS 路径（目录不带结尾的 /）
	IsDir       bool
	Depth       int    // 相对所列目录的层级，1 表示直接子项
	Files       int    // 目录：其下（递归）的文件数
	Summary     string // 一行说明（设定文档的标题与摘要、素材说明、记忆文件的 summary 或首行……）
	Unavailable bool   // 已规划但暂不可用的挂载点
}

// ReadResult 是读取结果。文本文件按行截取；图片不读内容，返回引用（由 tools 转成图片块）。
type ReadResult struct {
	Path       string
	Summary    string // 文件的一行说明（图片的素材说明等）
	Text       string // FromLine–ToLine 行的内容（不含最后一行的换行）
	FromLine   int
	ToLine     int
	TotalLines int
	LineCut    bool   // ToLine 这一行过长，只返回了前 MaxReadRunes 字
	Image      *Image // 非空表示是图片
}

// Image 是图片文件的引用。
type Image struct {
	Blob string
	MIME string
	Size int
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

// FS 是主笔看到的文件系统（每个回合草稿一个实例，见 Table）。
//
// TODO(M3-2)：Write / Edit / Remove——写入 /memory 的回合覆盖层（Overlay），约束见 Writable 与 12.2；
// MemorySource 已经以覆盖层为准，写入后立即可读。
// TODO(M3-4)：Search(ctx, Query) ([]Hit, error)——排序：记忆 > 作品资料 > 原文（新→旧），
// 每个文件最多 5 条，总计 ≤ 50 条 / 约 6,000 字符；别名扩展（File.Names）。
type FS interface {
	List(ctx context.Context, p string, depth int) ([]Entry, error)
	Read(ctx context.Context, p string, offset, limit int) (*ReadResult, error)
}

// 路径错误的类别（errors.Is 判断）。
var (
	ErrInvalid     = errors.New("路径不合法")
	ErrNotFound    = errors.New("不存在")
	ErrIsDir       = errors.New("是目录")
	ErrNotDir      = errors.New("不是目录")
	ErrUnavailable = errors.New("暂时不可用")
	ErrBinary      = errors.New("不是文本文件")
	ErrOutOfRange  = errors.New("超出文件范围")
)

// PathError 是路径相关的错误：陈述事实，并附带改正所需的信息（由 tools 组织成给模型的说明，D-21）。
type PathError struct {
	Path string
	Err  error // 上面的类别之一

	Reason   string   // ErrInvalid：不合法的原因；ErrBinary：文件类型；ErrUnavailable：挂载点说明
	Summary  string   // ErrBinary：文件的一行说明
	Similar  []string // ErrNotFound / ErrInvalid：相近的路径（最多 3 个）
	Parent   string   // ErrNotFound：最近的存在的上级目录
	Roots    []string // ErrNotFound（不在任何挂载点下）：可访问的目录
	Children []string // ErrIsDir：目录下的直接子项（子目录以 / 结尾）
	Total    int      // ErrOutOfRange：文件总行数
}

func (e *PathError) Error() string {
	if e.Reason != "" {
		return e.Path + "：" + e.Reason
	}
	return e.Path + " " + e.Err.Error()
}

func (e *PathError) Unwrap() error { return e.Err }
