package vfs

import (
	"context"
	"path"
	"slices"
	"strings"
)

// File 是挂载点下的一个文件或目录（由 Source 给出）。
type File struct {
	Path    string   // 完整 VFS 路径，如 /work/lore/characters/lin.md
	IsDir   bool     // 目录项只用来提供目录说明；目录本身由文件路径推导，不必逐个列出
	Size    int      // 字节数
	Blob    string   // 内容哈希（图片以图片块返回时引用）
	Summary string   // 一行说明
	Names   []string // 文件的其他叫法（文档 ID、标题、别名、素材 ID），用于路径写错时找出相近的文件
}

// Source 是一个挂载点的内容来源。实现需要支持并发读取（同一批 read 类工具并行执行）。
type Source interface {
	// Files 返回挂载点下的全部文件（可附带目录说明项），按路径排序。
	Files(ctx context.Context) ([]File, error)
	// Open 返回文件内容；文本可以经过处理（如设定文档的链接补全）。
	Open(ctx context.Context, f File) ([]byte, error)
}

// Mount 是挂载表中的一项。
type Mount struct {
	Root  string // 挂载点，如 /work
	Title string // 一行说明（列出 / 时显示），如"作品资料"
	// Source 提供文件；nil 表示挂载点已规划但暂不可用：访问时如实说明，而不是报"不存在"。
	Source Source
}

// Table 是挂载表，实现 FS。挂载表在回合开始时组装，之后不变；内容的变化（记忆覆盖层）由 Source 反映。
type Table struct {
	mounts []Mount
}

var _ FS = (*Table)(nil)

// NewTable 创建挂载表。挂载点按给出的顺序列出。
func NewTable(mounts ...Mount) *Table {
	return &Table{mounts: mounts}
}

// Roots 返回全部挂载点。
func (t *Table) Roots() []string {
	out := make([]string, len(t.mounts))
	for i, m := range t.mounts {
		out[i] = m.Root
	}
	return out
}

func (t *Table) mountOf(p string) *Mount {
	for i, m := range t.mounts {
		if p == m.Root || strings.HasPrefix(p, m.Root+"/") {
			return &t.mounts[i]
		}
	}
	return nil
}

// tree 是一个挂载点的文件索引。
type tree struct {
	files map[string]File
	dirs  map[string]*dirInfo
}

type dirInfo struct {
	summary  string
	children []string // 直接子项的完整路径（子目录在前、文件在后，各自按路径排序）
	files    int      // 其下（递归）的文件数
	seen     map[string]bool
}

// load 读取挂载点的文件清单，推导出目录结构。每次调用都重新读取：记忆在回合内会变化。
func (t *Table) load(ctx context.Context, m *Mount) (*tree, error) {
	list, err := m.Source.Files(ctx)
	if err != nil {
		return nil, err
	}
	tr := &tree{files: map[string]File{}, dirs: map[string]*dirInfo{}}
	dir := func(p string) *dirInfo {
		d, ok := tr.dirs[p]
		if !ok {
			d = &dirInfo{seen: map[string]bool{}}
			tr.dirs[p] = d
		}
		return d
	}
	dir(m.Root)
	link := func(p string) { // 把 p 及其上级目录逐级挂到挂载点下
		for p != m.Root {
			parent := path.Dir(p)
			pd := dir(parent)
			if pd.seen[p] {
				return
			}
			pd.seen[p] = true
			pd.children = append(pd.children, p)
			p = parent
		}
	}
	for _, f := range list {
		if !strings.HasPrefix(f.Path, m.Root+"/") {
			continue // 来源给出了挂载点以外的路径：忽略（不应发生）
		}
		if f.IsDir {
			dir(f.Path).summary = f.Summary
			link(f.Path)
			continue
		}
		tr.files[f.Path] = f
		link(f.Path)
		for p := path.Dir(f.Path); ; p = path.Dir(p) {
			dir(p).files++
			if p == m.Root {
				break
			}
		}
	}
	for p := range tr.dirs {
		if _, ok := tr.files[p]; ok { // 同一路径既是文件又是目录：以文件为准
			delete(tr.dirs, p)
		}
	}
	for _, d := range tr.dirs {
		slices.SortFunc(d.children, func(a, b string) int {
			_, da := tr.dirs[a]
			_, db := tr.dirs[b]
			if da != db {
				if da {
					return -1
				}
				return 1
			}
			return strings.Compare(a, b)
		})
	}
	return tr, nil
}

// resolve 校验路径并找到所属挂载点；路径不合法或不在任何挂载点下时返回带建议的 PathError。
func (t *Table) resolve(ctx context.Context, raw string) (string, *Mount, error) {
	p, err := Clean(raw)
	if err != nil {
		pe := &PathError{Path: strings.TrimSpace(raw), Err: ErrInvalid, Reason: err.Error()}
		// 漏写了开头的 /（或省略了 /work）：给出存在的完整路径
		if rel := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/"), "/"); rel != "" && !strings.Contains(rel, "..") {
			for _, c := range []string{"/" + rel, "/work/" + rel} {
				if t.exists(ctx, path.Clean(c)) {
					pe.Similar = append(pe.Similar, path.Clean(c))
				}
			}
		}
		return "", nil, pe
	}
	if p == "/" {
		return p, nil, nil
	}
	m := t.mountOf(p)
	if m == nil {
		return "", nil, &PathError{Path: p, Err: ErrNotFound, Roots: t.Roots(), Similar: t.similar(ctx, p)}
	}
	if m.Source == nil {
		return "", nil, &PathError{Path: p, Err: ErrUnavailable, Reason: m.Root + "（" + m.Title + "）暂时不可用"}
	}
	return p, m, nil
}

func (t *Table) exists(ctx context.Context, p string) bool {
	m := t.mountOf(p)
	if m == nil || m.Source == nil {
		return false
	}
	tr, err := t.load(ctx, m)
	if err != nil {
		return false
	}
	_, isFile := tr.files[p]
	_, isDir := tr.dirs[p]
	return isFile || isDir
}

// Read 读取文本文件的 offset 行起至多 limit 行（offset ≤ 0 取 1；limit ≤ 0 取默认值，超过上限取上限）。
func (t *Table) Read(ctx context.Context, raw string, offset, limit int) (*ReadResult, error) {
	p, m, err := t.resolve(ctx, raw)
	if err != nil {
		return nil, err
	}
	if m == nil { // 根目录
		return nil, &PathError{Path: "/", Err: ErrIsDir, Children: dirNames(t.Roots())}
	}
	tr, err := t.load(ctx, m)
	if err != nil {
		return nil, err
	}
	f, ok := tr.files[p]
	if !ok {
		if d, isDir := tr.dirs[p]; isDir {
			return nil, &PathError{Path: p, Err: ErrIsDir, Children: childNames(tr, d)}
		}
		return nil, t.notFound(ctx, p, m, tr)
	}

	switch kind, mime := mediaOf(p); {
	case mime != "":
		return &ReadResult{Path: p, Summary: f.Summary, Image: &Image{Blob: f.Blob, MIME: mime, Size: f.Size}}, nil
	case kind != "":
		return nil, &PathError{Path: p, Err: ErrBinary, Reason: "是" + kind + "，不能以文字读取", Summary: f.Summary}
	}
	data, err := m.Source.Open(ctx, f)
	if err != nil {
		return nil, err
	}
	if !isText(data) {
		return nil, &PathError{Path: p, Err: ErrBinary, Reason: "不是文本文件，不能以文字读取", Summary: f.Summary}
	}

	lines := splitLines(data)
	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		limit = DefaultReadLines
	}
	limit = min(limit, MaxReadLines)
	res := &ReadResult{Path: p, Summary: f.Summary, TotalLines: len(lines)}
	if len(lines) == 0 {
		return res, nil // 空文件
	}
	if offset > len(lines) {
		return nil, &PathError{Path: p, Err: ErrOutOfRange, Total: len(lines)}
	}
	res.FromLine = offset
	res.Text, res.ToLine, res.LineCut = sliceLines(lines, offset, limit)
	return res, nil
}

// List 列出目录内容，展开 depth 层（1–MaxListDepth）。列出 / 时给出各挂载点。
func (t *Table) List(ctx context.Context, raw string, depth int) ([]Entry, error) {
	depth = max(1, min(depth, MaxListDepth))
	p, m, err := t.resolve(ctx, raw)
	if err != nil {
		return nil, err
	}
	if m == nil { // 根目录：挂载点
		var out []Entry
		for i := range t.mounts {
			mt := &t.mounts[i]
			e := Entry{Path: mt.Root, IsDir: true, Depth: 1, Summary: mt.Title, Unavailable: mt.Source == nil}
			if mt.Source == nil {
				out = append(out, e)
				continue
			}
			tr, err := t.load(ctx, mt)
			if err != nil {
				return nil, err
			}
			e.Files = tr.dirs[mt.Root].files
			out = append(out, e)
			if depth > 1 {
				out = append(out, walk(tr, mt.Root, 2, depth)...)
			}
		}
		return out, nil
	}
	tr, err := t.load(ctx, m)
	if err != nil {
		return nil, err
	}
	if _, isFile := tr.files[p]; isFile {
		return nil, &PathError{Path: p, Err: ErrNotDir}
	}
	if _, isDir := tr.dirs[p]; !isDir {
		return nil, t.notFound(ctx, p, m, tr)
	}
	return walk(tr, p, 1, depth), nil
}

// walk 列出目录 dir 的子项（层级从 level 开始），展开到 maxDepth 层。
func walk(tr *tree, dir string, level, maxDepth int) []Entry {
	var out []Entry
	for _, c := range tr.dirs[dir].children {
		if d, isDir := tr.dirs[c]; isDir {
			out = append(out, Entry{Path: c, IsDir: true, Depth: level, Files: d.files, Summary: d.summary})
			if level < maxDepth {
				out = append(out, walk(tr, c, level+1, maxDepth)...)
			}
			continue
		}
		f := tr.files[c]
		out = append(out, Entry{Path: c, Depth: level, Summary: f.Summary})
	}
	return out
}

// childNames 返回目录的直接子项（子目录以 / 结尾）。
func childNames(tr *tree, d *dirInfo) []string {
	out := make([]string, 0, len(d.children))
	for _, c := range d.children {
		if _, isDir := tr.dirs[c]; isDir {
			c += "/"
		}
		out = append(out, c)
	}
	return out
}

func dirNames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = p + "/"
	}
	return out
}

// notFound 构造"不存在"的错误：附上相近的路径，以及最近的存在的上级目录。
func (t *Table) notFound(ctx context.Context, p string, m *Mount, tr *tree) error {
	parent := path.Dir(p)
	for {
		if _, ok := tr.dirs[parent]; ok || parent == m.Root {
			break
		}
		parent = path.Dir(parent)
	}
	return &PathError{Path: p, Err: ErrNotFound, Parent: parent, Similar: t.similar(ctx, p)}
}
