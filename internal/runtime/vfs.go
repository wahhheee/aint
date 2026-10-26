package runtime

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wahhheee/aint/internal/media"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/vfs"
	"github.com/wahhheee/aint/internal/work"
)

// buildVFS 组装本回合的挂载表（《02-技术方案》12.1）：
//
//	/work        作品资料：lore/ 下的设定文档（读取时补全 [[id]] 链接，D-35）与 assets/ 下的素材
//	/memory      本存档的记忆：以进度线 head 的记忆清单为底的写时复制覆盖层，写入随回合提交
//	/transcript  进度线上已提交回合的原文：/transcript/0042 是第 42 回合（虚拟文件，检索走全文索引）
//	/uploads     进度线上（含本回合）玩家上传的图片，只在模型能看图时挂载：/uploads/0005-1.jpg 是第 5 回合的第 1 张
//
// 挂载表在回合开始时确定，之后不变；分支隔离由"只取进度线上的记忆清单与上传"保证（不变量 3）。
func (m *Manager) buildVFS(ctx context.Context, w *work.Model, fs prompts.FeatureSet, saveID string, line []*save.Node, turnNo int, uploads []uploadFile) (*vfs.Table, *vfs.Overlay, error) {
	head := line[len(line)-1]
	mf, err := m.d.Saves.LoadMemory(ctx, head.MemoryHash)
	if err != nil {
		return nil, nil, err
	}
	base := make(map[string]vfs.FileRef, len(mf.Files))
	for p, f := range mf.Files {
		base[p] = vfs.FileRef{Blob: f.Blob, Size: f.Size, Turn: f.Turn}
	}
	overlay := vfs.NewOverlay(base, turnNo)
	mounts := []vfs.Mount{
		{Root: "/work", Title: "作品资料（只读）", Source: &workSource{w: w, works: m.d.Works}},
		{Root: "/memory", Title: "你的记忆笔记", Source: vfs.NewMemorySource(overlay, m.d.Saves.Blob)},
		{Root: "/transcript", Title: "已发生的原文（只读）", Source: newTranscriptSource(m.d.Saves, saveID, w, line)},
	}
	if fs.Has(prompts.FVision) {
		mounts = append(mounts, vfs.Mount{Root: "/uploads", Title: "玩家上传的图片（只读）", Source: uploadsSource{files: uploads, blob: m.d.Saves.Blob}})
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
		text := s.w.LinkDocs(string(data)) // D-35：[[id]] → [[id]](/work/lore/…)
		if d, ok := s.w.DocByPath(strings.TrimPrefix(f.Path, "/work/")); ok && len(d.Images) > 0 {
			text = strings.TrimRight(text, "\n") + "\n\n" + docImages(s.w, d)
		}
		return []byte(text), nil
	}
	return data, nil
}

// uploadFile 是 /uploads 中的一张图片。
type uploadFile struct {
	Path string // /uploads/0005-1.jpg
	Blob string
	Size int
	Turn int
}

type uploadSet struct {
	all     []uploadFile // 进度线上（含本回合）的全部上传，按回合排序
	current []uploadFile // 本回合附带的图片
}

// uploads 收集进度线上各回合附带的图片，并检查本回合的附件都是登记过的上传图片。
func (m *Manager) uploads(ctx context.Context, line []*save.Node, attachments []string, turnNo int) (uploadSet, error) {
	var set uploadSet
	add := func(turn int, hashes []string, current bool) error {
		for i, h := range hashes {
			u, err := m.d.Images.Upload(ctx, h)
			if err != nil {
				if current {
					return errs.Invalid("附带的图片不存在，请重新上传")
				}
				continue // 历史回合引用的图片缺失（不应发生）：不出现在 /uploads 中
			}
			f := uploadFile{Path: fmt.Sprintf("/uploads/%04d-%d%s", turn, i+1, media.Ext(u.MIME)), Blob: u.Hash, Size: u.Size, Turn: turn}
			set.all = append(set.all, f)
			if current {
				set.current = append(set.current, f)
			}
		}
		return nil
	}
	for _, n := range line {
		if n.Kind == save.KindTurn && n.Input != nil && len(n.Input.Attachments) > 0 {
			if err := add(n.TurnNo, n.Input.Attachments, false); err != nil {
				return set, err
			}
		}
	}
	return set, add(turnNo, attachments, true)
}

// uploadsSource 是 /uploads 挂载点。
type uploadsSource struct {
	files []uploadFile
	blob  func(ctx context.Context, hash string) ([]byte, error)
}

func (s uploadsSource) Files(context.Context) ([]vfs.File, error) {
	out := make([]vfs.File, len(s.files))
	for i, f := range s.files {
		out[i] = vfs.File{Path: f.Path, Size: f.Size, Blob: f.Blob, Summary: fmt.Sprintf("第 %d 回合玩家附带的图片", f.Turn)}
	}
	return out, nil
}

func (s uploadsSource) Open(ctx context.Context, f vfs.File) ([]byte, error) {
	return s.blob(ctx, f.Blob)
}

// docImages 是设定文档末尾附加的关联图片说明（《03-作品格式规范》7.1 images）：AI 读到文档时知道有哪些图片可以查看。
func docImages(w *work.Model, d work.Doc) string {
	lines := []string{"（关联图片，可以用 read 查看：）"}
	for _, ref := range d.Images {
		a, ok := w.Asset(ref)
		if !ok {
			continue
		}
		line := "- /work/" + a.Path
		if a.Description != "" {
			line += " — " + a.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// transcriptSource 是 /transcript 挂载点：进度线上已提交的回合，每回合一个虚拟文件（《02-技术方案》12.4）。
// 只含当前进度线（分支隔离，不变量 3）；正在进行的回合不在其中。
type transcriptSource struct {
	saves  *save.Service
	saveID string
	name   func(string) string // <say who> 的人物 ID → 人物名
	turns  []*save.Node        // 进度线上的 turn 节点，按回合号升序
	byNode map[string]*save.Node
}

func newTranscriptSource(saves *save.Service, saveID string, w *work.Model, line []*save.Node) *transcriptSource {
	s := &transcriptSource{saves: saves, saveID: saveID, name: w.SpeakerName, byNode: map[string]*save.Node{}}
	for _, n := range line {
		if n.Kind == save.KindTurn {
			s.turns = append(s.turns, n)
			s.byNode[n.ID] = n
		}
	}
	return s
}

func (s *transcriptSource) Files(context.Context) ([]vfs.File, error) {
	out := make([]vfs.File, len(s.turns))
	for i, n := range s.turns {
		out[i] = vfs.File{Path: save.TranscriptPath(n.TurnNo), Summary: save.InputSummary(n.Input), Names: []string{strconv.Itoa(n.TurnNo)}}
	}
	return out, nil
}

func (s *transcriptSource) Open(ctx context.Context, f vfs.File) ([]byte, error) {
	for _, n := range s.turns {
		if save.TranscriptPath(n.TurnNo) != f.Path {
			continue
		}
		// 由显示项生成（与写入原文索引的是同一个函数），按哈希读取，不必扫描全文索引
		display, err := s.saves.LoadDisplay(ctx, n.DisplayHash)
		if err != nil {
			return nil, err
		}
		return []byte(save.TranscriptFile(n.TurnNo, save.TranscriptBody(display, s.name))), nil
	}
	return nil, &vfs.PathError{Path: f.Path, Err: vfs.ErrNotFound}
}

// Search 检索原文：字面检索的词都不少于 3 个字时先用全文索引（trigram）取候选回合，否则读出进度线上的全部原文逐行扫描；
// 结果只保留进度线上的回合，按回合号新→旧排列。
func (s *transcriptSource) Search(ctx context.Context, m *vfs.Matcher, prefix string) ([]vfs.Hit, error) {
	var texts map[string]string
	var err error
	if len(m.Terms) > 0 && !slices.ContainsFunc(m.Terms, func(t string) bool { return utf8.RuneCountInString(t) < 3 }) {
		texts, err = s.saves.MatchTranscript(ctx, s.saveID, m.Terms)
	} else {
		texts, err = s.saves.TranscriptTexts(ctx, s.saveID)
	}
	if err != nil {
		return nil, err
	}
	var hits []vfs.Hit
	for id, body := range texts {
		n, ok := s.byNode[id]
		if !ok {
			continue // 其他分支的回合
		}
		p := save.TranscriptPath(n.TurnNo)
		if prefix != "/" && prefix != "/transcript" && prefix != p {
			continue
		}
		lines := vfs.Lines([]byte(save.TranscriptFile(n.TurnNo, body)))
		for _, h := range vfs.ScanLines(p, lines, m) {
			if h.Line == 1 { // 标题行"# 第 N 回合"不参与检索
				continue
			}
			h.Order = -n.TurnNo
			hits = append(hits, h)
		}
	}
	return hits, nil
}
