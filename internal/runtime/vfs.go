package runtime

import (
	"context"
	"path"
	"strings"

	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/vfs"
	"github.com/wahhheee/yanyi/internal/work"
)

// buildVFS 组装本回合的挂载表（《02-技术方案》12.1）：
//
//	/work        作品资料：lore/ 下的设定文档（读取时补全 [[id]] 链接，D-35）与 assets/ 下的素材
//	/memory      本存档的记忆：以进度线 head 的记忆清单为底的写时复制覆盖层（写入随 M3-2）
//	/transcript  已发生的原文（TODO(M3-3)：虚拟文件；目前挂载点存在但暂不可用）
//	/uploads     玩家上传的图片（只在模型能看图时挂载；TODO(M2-10)：目前暂不可用）
//
// 挂载表在回合开始时确定，之后不变；分支隔离由"只取 head 的记忆清单"保证（不变量 3）。
func (m *Manager) buildVFS(ctx context.Context, w *work.Model, fs prompts.FeatureSet, head *save.Node) (*vfs.Table, *vfs.Overlay, error) {
	mf, err := m.d.Saves.LoadMemory(ctx, head.MemoryHash)
	if err != nil {
		return nil, nil, err
	}
	base := make(map[string]vfs.FileRef, len(mf.Files))
	for p, f := range mf.Files {
		base[p] = vfs.FileRef{Blob: f.Blob, Size: f.Size, Turn: f.Turn}
	}
	overlay := vfs.NewOverlay(base)
	mounts := []vfs.Mount{
		{Root: "/work", Title: "作品资料（只读）", Source: &workSource{w: w, works: m.d.Works}},
		{Root: "/memory", Title: "你的记忆笔记", Source: vfs.NewMemorySource(overlay, m.d.Saves.Blob)},
		{Root: "/transcript", Title: "已发生的原文（只读）"},
	}
	if fs.Has(prompts.FVision) {
		mounts = append(mounts, vfs.Mount{Root: "/uploads", Title: "玩家上传的图片（只读）"})
	}
	return vfs.NewTable(mounts...), overlay, nil
}

// workSource 是 /work 挂载点：作品的设定文档与素材。只列出作品编译后认可的文件，不触及真实文件系统。
type workSource struct {
	w     *work.Model
	works *work.Service
}

func (s *workSource) Files(context.Context) ([]vfs.File, error) {
	var out []vfs.File
	for _, d := range s.w.Docs {
		ref := s.w.Files[d.Path]
		p := "/work/" + d.Path
		if d.IsIndex { // 目录摘要 _index.md：既作为目录说明，也可以直接读取
			out = append(out, vfs.File{Path: path.Dir(p), IsDir: true, Summary: docSummary(d)})
		}
		names := append([]string{d.ID, d.Title}, d.Aliases...)
		out = append(out, vfs.File{Path: p, Size: ref.Size, Blob: ref.Blob, Summary: docSummary(d), Names: names})
	}
	for _, a := range s.w.Assets {
		ref := s.w.Files[a.Path]
		sum := a.Description
		if sum == "" {
			sum = a.Role
		}
		out = append(out, vfs.File{Path: "/work/" + a.Path, Size: ref.Size, Blob: a.Blob, Summary: sum, Names: []string{a.ID}})
	}
	return out, nil
}

// docSummary 是设定文档的一行说明：标题，有摘要时附上摘要。
func docSummary(d work.Doc) string {
	switch {
	case d.Title != "" && d.Summary != "":
		return d.Title + "：" + d.Summary
	case d.Title != "":
		return d.Title
	}
	return d.Summary
}

func (s *workSource) Open(ctx context.Context, f vfs.File) ([]byte, error) {
	data, err := s.works.AssetData(ctx, f.Blob)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, &vfs.PathError{Path: f.Path, Err: vfs.ErrNotFound}
	}
	if strings.HasPrefix(f.Path, "/work/lore/") && strings.HasSuffix(f.Path, ".md") {
		return []byte(s.w.LinkDocs(string(data))), nil // D-35：[[id]] → [[id]](/work/lore/…)
	}
	return data, nil
}
