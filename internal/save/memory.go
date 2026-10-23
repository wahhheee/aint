package save

import (
	"context"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/vfs"
)

// MemoryEntry 是记忆目录中的一项（阶段块 <memory_index>，《02-技术方案》12.6）。
// Path 以 / 结尾时是折叠后的目录，Summary 为"N 个文件"。
type MemoryEntry struct {
	Path    string
	Summary string // frontmatter 的 summary 或首个非空行（≤ 60 字）
}

// memoryIndexFold 是记忆目录逐个列出文件的上限：超过时按目录折叠。
const memoryIndexFold = 80

// MemoryIndex 生成记忆目录：每个文件一行说明；文件超过 80 个时，/memory 下第一层的文件照常列出，
// 子目录折叠为"目录名（N 个文件）"。
func (s *Service) MemoryIndex(ctx context.Context, memoryHash string) ([]MemoryEntry, error) {
	mf, err := s.LoadMemory(ctx, memoryHash)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(mf.Files))
	for p := range mf.Files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	folded := map[string]int{}
	var out []MemoryEntry
	for _, p := range paths {
		if len(paths) > memoryIndexFold {
			if dir := topDir(p); dir != "" {
				if folded[dir] == 0 {
					out = append(out, MemoryEntry{Path: dir + "/"})
				}
				folded[dir]++
				continue
			}
		}
		data, err := s.Blob(ctx, mf.Files[p].Blob)
		if err != nil {
			return nil, err
		}
		out = append(out, MemoryEntry{Path: p, Summary: vfs.Summary(string(data))})
	}
	for i, e := range out {
		if n := folded[strings.TrimSuffix(e.Path, "/")]; n > 0 {
			out[i].Summary = fmt.Sprintf("%d 个文件", n)
		}
	}
	return out, nil
}

// topDir 返回 /memory 下的第一层目录（/memory/characters/lin.md → /memory/characters）；直接在 /memory 下的文件返回空。
func topDir(p string) string {
	rest := strings.TrimPrefix(p, "/memory/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return "/memory/" + rest[:i]
	}
	return ""
}

// MemoryView 是存档当前（head）的记忆（记忆抽屉）。
type MemoryView struct {
	NodeID string           `json:"node_id"`
	TurnNo int              `json:"turn_no"`
	Files  []MemoryFileView `json:"files"`
	Total  int              `json:"total"` // 全部文件的字节数
	Limit  int              `json:"limit"` // 记忆总量上限
}

// MemoryFileView 是记忆中的一个文件。
type MemoryFileView struct {
	Path    string `json:"path"`
	Size    int    `json:"size"`
	Turn    int    `json:"turn"` // 最后修改它的回合（0 表示来自作品的初始模板）
	Summary string `json:"summary,omitzero"`
	Content string `json:"content,omitzero"` // 只在读取单个文件时返回
}

// Memory 返回 head 处的记忆文件列表。
func (s *Service) Memory(ctx context.Context, saveID string) (*MemoryView, error) {
	_, line, err := s.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, err
	}
	head := line[len(line)-1]
	mf, err := s.LoadMemory(ctx, head.MemoryHash)
	if err != nil {
		return nil, err
	}
	v := &MemoryView{NodeID: head.ID, TurnNo: head.TurnNo, Files: []MemoryFileView{}, Limit: vfs.MaxMemoryTotal}
	for _, p := range slices.Sorted(maps.Keys(mf.Files)) {
		f := mf.Files[p]
		data, err := s.Blob(ctx, f.Blob)
		if err != nil {
			return nil, err
		}
		v.Files = append(v.Files, MemoryFileView{Path: p, Size: f.Size, Turn: f.Turn, Summary: vfs.Summary(string(data))})
		v.Total += f.Size
	}
	return v, nil
}

// MemoryFileAt 返回 head 处的一个记忆文件（含内容）。
func (s *Service) MemoryFileAt(ctx context.Context, saveID, p string) (*MemoryFileView, error) {
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	_, line, err := s.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, err
	}
	mf, err := s.LoadMemory(ctx, line[len(line)-1].MemoryHash)
	if err != nil {
		return nil, err
	}
	f, ok := mf.Files[p]
	if !ok {
		return nil, errs.NotFound("记忆文件 %s 不存在", p)
	}
	data, err := s.Blob(ctx, f.Blob)
	if err != nil {
		return nil, err
	}
	return &MemoryFileView{Path: p, Size: f.Size, Turn: f.Turn, Summary: vfs.Summary(string(data)), Content: string(data)}, nil
}
