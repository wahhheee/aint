package save

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/ids"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/store"
	"github.com/wahhheee/yanyi/internal/tmpl"
	"github.com/wahhheee/yanyi/internal/usage"
	"github.com/wahhheee/yanyi/internal/work"
)

// Service 是存档服务。并发安全；"同一存档同一时刻只有一个写者"由 runtime 的会话锁保证。
type Service struct {
	db    *store.DB
	blobs *store.Blobs
	works *work.Service
	tmpl  *tmpl.Engine
}

// NewService 创建存档服务。
func NewService(db *store.DB, blobs *store.Blobs, works *work.Service, tm *tmpl.Engine) *Service {
	return &Service{db: db, blobs: blobs, works: works, tmpl: tm}
}

// CreateParams 是新建存档的参数。
type CreateParams struct {
	WorkID    string         `json:"work_id"`
	VersionID string         `json:"version_id,omitzero"` // 为空取最新版本
	Title     string         `json:"title,omitzero"`
	Form      map[string]any `json:"form,omitzero"` // 开局表单：状态路径 → 值
	Settings  Settings       `json:"settings"`
}

// Create 新建存档：root 节点（初始状态 + 初始记忆）；静态开场时再加一个开场回合（零 token）。
func (s *Service) Create(ctx context.Context, p CreateParams) (*Save, error) {
	versionID := p.VersionID
	if versionID == "" {
		var err error
		if versionID, err = s.works.LatestVersionID(ctx, p.WorkID); err != nil {
			return nil, err
		}
	}
	m, err := s.works.Model(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if m.WorkID != p.WorkID {
		return nil, errs.Invalid("作品版本 %s 不属于作品 %s", versionID, p.WorkID)
	}

	// 初始状态 = 默认值 + 开局表单
	st := m.State.Defaults()
	for _, f := range m.Manifest.NewGame.Fields {
		v, ok := p.Form[f.Key]
		if !ok || v == nil {
			if f.Required {
				return nil, errs.Invalid("请填写「%s」", f.Label)
			}
			v = f.Default
		}
		if v == nil {
			continue
		}
		if err := state.Set(m.State, st, f.Key, v); err != nil {
			return nil, errs.Invalid("「%s」填写不正确：%v", f.Label, err)
		}
	}

	// 初始记忆 = 作品 memory/ 模板：直接引用作品文件的 blob，零拷贝
	mem := MemoryManifest{V: 1, Files: map[string]MemoryFile{}}
	for path, ref := range m.Files {
		if rest, ok := strings.CutPrefix(path, "memory/"); ok {
			mem.Files["/memory/"+rest] = MemoryFile{Blob: ref.Blob, Size: ref.Size}
		}
	}

	// 静态开场：以初始状态渲染一次
	var opening *DisplayItem
	var openingChoices *DisplayItem
	if m.Opening != nil && m.Opening.Mode == "static" {
		body, err := s.tmpl.RenderWith("prompts/opening.md", m.Opening.Body, map[string]any{"state": st}, m.Partials)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeWorkInvalid, "开场模板渲染失败")
		}
		opening = &DisplayItem{T: "text", Markup: strings.TrimSpace(body)}
		if len(m.Opening.Choices) > 0 {
			openingChoices = &DisplayItem{T: "choices", Free: m.Manifest.Features.FreeInput}
			for _, c := range m.Opening.Choices {
				openingChoices.Items = append(openingChoices.Items, ChoiceItem{ID: c.ID, Label: c.Label, Detail: c.Detail})
			}
		}
	}

	now := store.NowMillis()
	sv := &Save{
		ID: ids.New(), WorkID: p.WorkID, WorkVersionID: versionID, WorkTitle: m.Manifest.Title,
		Title: strings.TrimSpace(p.Title), Settings: p.Settings, CreatedAt: now, UpdatedAt: now, LastPlayedAt: now,
	}
	if sv.Title != "" {
		sv.TitleSource = "manual"
	}

	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		stateHash, err := s.blobs.PutJSON(ctx, tx, st)
		if err != nil {
			return err
		}
		memHash, err := s.blobs.PutJSON(ctx, tx, mem)
		if err != nil {
			return err
		}
		root := &Node{
			ID: ids.New(), SaveID: sv.ID, Kind: KindRoot, Status: StatusCommitted,
			StateHash: stateHash, MemoryHash: memHash, WorkVersionID: versionID, CreatedAt: now,
		}
		// 第 0 阶段的阶段块只由初始状态与初始记忆决定，构建请求时即时渲染，不存储（D-39）
		sv.HeadNodeID = root.ID
		if err := insertSave(ctx, tx, sv); err != nil {
			return err
		}
		if err := insertNode(ctx, tx, root); err != nil {
			return err
		}
		if opening == nil {
			return nil
		}

		display := []DisplayItem{*opening}
		if openingChoices != nil {
			display = append(display, *openingChoices)
		}
		// 合成的助手消息：开场正文 + 可选行动（《02-技术方案》9.8）
		text := opening.Markup
		if openingChoices != nil {
			var parts []string
			for _, c := range openingChoices.Items {
				parts = append(parts, strings.ToUpper(c.ID)+" "+c.Label)
			}
			text += "\n\n（可选行动：" + strings.Join(parts, "；") + "）"
		}
		// 先放一条合成的 user 消息：历史始终以 user 开头，与生成式开场的结构一致（D-33）
		messages := []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.StaticOpeningInput)}},
			{Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text(text)}},
		}
		node, err := s.commitTx(ctx, tx, TurnCommit{
			SaveID: sv.ID, Parent: root, TurnNo: 1, Input: TurnInput{Kind: InputOpening},
			Messages: messages, Display: display, StateHash: stateHash, MemoryHash: memHash,
			WorkVersionID: versionID, Status: StatusCommitted, PlainText: TranscriptBody(display, m.SpeakerName),
			Meta: NodeMeta{Stop: "static_opening"},
		})
		if err != nil {
			return err
		}
		sv.HeadNodeID, sv.TurnCount = node.ID, 1
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("创建存档失败: %w", err)
	}
	slog.InfoContext(ctx, "已新建存档", "save_id", sv.ID, "work_id", sv.WorkID, "version_id", versionID)
	return sv, nil
}

// Get 返回存档。
func (s *Service) Get(ctx context.Context, id string) (*Save, error) {
	sv, err := getSave(ctx, s.db.Read(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.NotFound("存档不存在")
	}
	return sv, err
}

// ListByWork 返回作品的全部存档（最近游玩在前）。
func (s *Service) ListByWork(ctx context.Context, workID string) ([]*Save, error) {
	return querySaves(ctx, s.db.Read(), `WHERE s.work_id = ? ORDER BY s.last_played_at DESC`, workID)
}

// Recent 返回最近游玩的存档（书架"继续阅读"）。
func (s *Service) Recent(ctx context.Context, limit int) ([]*Save, error) {
	return querySaves(ctx, s.db.Read(), `ORDER BY s.last_played_at DESC LIMIT ?`, limit)
}

// Rename 重命名存档。title 为空表示清除名称、恢复自动标题。
func (s *Service) Rename(ctx context.Context, id, title string) error {
	title = strings.TrimSpace(title)
	source := "manual"
	if title == "" {
		source = ""
	}
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE saves SET title = ?, title_source = ?, updated_at = ? WHERE id = ?`,
			store.NullString(title), store.NullString(source), store.NowMillis(), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errs.NotFound("存档不存在")
		}
		return nil
	})
}

// Delete 删除存档（节点级联删除；原文索引一并删除；blob 交给 GC）。
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM transcript_fts WHERE save_id = ?`, id); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM saves WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errs.NotFound("存档不存在")
		}
		return nil
	})
}

// ProgressLine 返回存档及其进度线（root → head）。
func (s *Service) ProgressLine(ctx context.Context, saveID string) (*Save, []*Node, error) {
	sv, err := s.Get(ctx, saveID)
	if err != nil {
		return nil, nil, err
	}
	line, err := pathToRoot(ctx, s.db.Read(), sv.HeadNodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("读取进度线失败: %w", err)
	}
	return sv, line, nil
}

// TurnCommit 是一次回合提交的全部内容（由 runtime 组装）。
type TurnCommit struct {
	SaveID        string
	Parent        *Node
	TurnNo        int
	Input         TurnInput
	Messages      []llm.Message // 本回合产生的消息（发送形态）
	Display       []DisplayItem
	StateHash     string         // 未变化时传父节点的哈希
	State         map[string]any // 非 nil 时在同一事务中写入新的状态快照，StateHash 取其哈希
	MemoryHash    string         // 未变化时传父节点的哈希
	Memory        *MemoryWrite   // 非 nil 时在同一事务中写入记忆文件与新清单，MemoryHash 取新清单的哈希
	WorkVersionID string
	Status        NodeStatus
	Usage         *llm.Usage
	Meta          NodeMeta
	PlainText     string         // 原文正文（TranscriptBody），写入原文索引
	UsageRecords  []usage.Record // 本回合的每次模型调用
	NodeID        string         // 可选：预先分配的节点 ID（runtime 用回合 ID 作节点 ID，便于前后端对应）
}

// CommitTurn 在单个事务中提交回合：消息与显示项 blob、节点、原文索引、用量、存档 head（不变量 2）。
func (s *Service) CommitTurn(ctx context.Context, c TurnCommit) (*Node, error) {
	var node *Node
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		node, err = s.commitTx(ctx, tx, c)
		return err
	})
	return node, err
}

func (s *Service) commitTx(ctx context.Context, tx *sql.Tx, c TurnCommit) (*Node, error) {
	msgHash, err := s.blobs.PutJSON(ctx, tx, c.Messages)
	if err != nil {
		return nil, err
	}
	dispHash, err := s.blobs.PutJSON(ctx, tx, c.Display)
	if err != nil {
		return nil, err
	}
	if c.State != nil {
		if c.StateHash, err = s.blobs.PutJSON(ctx, tx, c.State); err != nil {
			return nil, err
		}
	}
	if c.Memory != nil {
		if c.MemoryHash, err = s.putMemory(ctx, tx, c.Memory); err != nil {
			return nil, err
		}
	}
	id := c.NodeID
	if id == "" {
		id = ids.New()
	}
	in := c.Input
	n := &Node{
		ID: id, SaveID: c.SaveID, ParentID: c.Parent.ID, Depth: c.Parent.Depth + 1, Kind: KindTurn,
		Status: c.Status, TurnNo: c.TurnNo, Input: &in, MessagesHash: msgHash, DisplayHash: dispHash,
		StateHash: c.StateHash, MemoryHash: c.MemoryHash, WorkVersionID: c.WorkVersionID,
		Usage: c.Usage, Meta: c.Meta, CreatedAt: store.NowMillis(),
	}
	if err := insertNode(ctx, tx, n); err != nil {
		return nil, fmt.Errorf("写入节点失败: %w", err)
	}
	if err := insertTranscript(ctx, tx, c.SaveID, n.ID, c.PlainText); err != nil {
		return nil, fmt.Errorf("写入原文索引失败: %w", err)
	}
	for _, r := range c.UsageRecords {
		r.SaveID, r.NodeID = c.SaveID, n.ID
		if err := usage.Insert(ctx, tx, r); err != nil {
			return nil, fmt.Errorf("写入用量失败: %w", err)
		}
	}
	if err := setHead(ctx, tx, c.SaveID, n.ID, true); err != nil {
		return nil, err
	}
	return n, nil
}

// MemoryWrite 是一次提交中记忆的新内容：Files 是完整清单，Data 是本次新写入的文件内容（按路径）。
type MemoryWrite struct {
	Files map[string]MemoryFile
	Data  map[string][]byte
}

// putMemory 写入记忆文件内容与新清单，返回清单哈希。
func (s *Service) putMemory(ctx context.Context, q store.Querier, w *MemoryWrite) (string, error) {
	files := make(map[string]MemoryFile, len(w.Files))
	for p, f := range w.Files {
		files[p] = f
	}
	for p, data := range w.Data {
		f, ok := files[p]
		if !ok {
			continue // 写入后又删除了
		}
		hash, err := s.blobs.Put(ctx, q, data, memoryMIME(p))
		if err != nil {
			return "", err
		}
		f.Blob, f.Size = hash, len(data)
		files[p] = f
	}
	return s.blobs.PutJSON(ctx, q, MemoryManifest{V: 1, Files: files})
}

func memoryMIME(p string) string {
	switch {
	case strings.HasSuffix(p, ".md"):
		return "text/markdown; charset=utf-8"
	case strings.HasSuffix(p, ".yaml"):
		return "application/yaml"
	}
	return "text/plain; charset=utf-8"
}

// LoadMessages 读取节点的消息。
func (s *Service) LoadMessages(ctx context.Context, hash string) ([]llm.Message, error) {
	var out []llm.Message
	if hash == "" {
		return out, nil
	}
	return out, s.blobs.GetJSON(ctx, s.db.Read(), hash, &out)
}

// LoadDisplay 读取节点的显示项。
func (s *Service) LoadDisplay(ctx context.Context, hash string) ([]DisplayItem, error) {
	out := []DisplayItem{}
	if hash == "" {
		return out, nil
	}
	return out, s.blobs.GetJSON(ctx, s.db.Read(), hash, &out)
}

// LoadState 读取状态快照。
func (s *Service) LoadState(ctx context.Context, hash string) (map[string]any, error) {
	out := map[string]any{}
	return out, s.blobs.GetJSON(ctx, s.db.Read(), hash, &out)
}

// LoadMemory 读取记忆清单（VFS /memory 的底层）。
func (s *Service) LoadMemory(ctx context.Context, hash string) (MemoryManifest, error) {
	mf := MemoryManifest{Files: map[string]MemoryFile{}}
	if hash == "" {
		return mf, nil
	}
	if err := s.blobs.GetJSON(ctx, s.db.Read(), hash, &mf); err != nil {
		return mf, err
	}
	if mf.Files == nil {
		mf.Files = map[string]MemoryFile{}
	}
	return mf, nil
}

// Blob 按哈希读取内容（记忆文件等）。
func (s *Service) Blob(ctx context.Context, hash string) ([]byte, error) {
	return s.blobs.Get(ctx, s.db.Read(), hash)
}

// CompactionCommit 是一次上下文整理的提交内容（由 runtime 组装）。
type CompactionCommit struct {
	SaveID       string
	Parent       *Node        // 整理时的 head
	Epoch        string       // 新阶段块（L4），存为 blob，阶段内原样复用
	KeepFrom     string       // 保留原文的第一个回合节点 ID
	Memory       *MemoryWrite // 落盘写入的记忆；nil 表示没有改动
	Messages     []llm.Message
	Usage        *llm.Usage
	Meta         NodeMeta
	UsageRecords []usage.Record
}

// CommitCompaction 在单个事务中提交整理节点：阶段块、整理过程的消息（只供排查，不进入之后的上下文）、
// 新记忆清单、用量，并把 head 移到整理节点。状态不变。
func (s *Service) CommitCompaction(ctx context.Context, c CompactionCommit) (*Node, error) {
	n := &Node{
		ID: ids.New(), SaveID: c.SaveID, ParentID: c.Parent.ID, Depth: c.Parent.Depth + 1, Kind: KindCompaction,
		Status: StatusCommitted, TurnNo: c.Parent.TurnNo, KeepFrom: c.KeepFrom, StateHash: c.Parent.StateHash,
		MemoryHash: c.Parent.MemoryHash, WorkVersionID: c.Parent.WorkVersionID, Usage: c.Usage, Meta: c.Meta,
		CreatedAt: store.NowMillis(),
	}
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if n.EpochHash, err = s.blobs.Put(ctx, tx, []byte(c.Epoch), "text/plain; charset=utf-8"); err != nil {
			return err
		}
		if n.MessagesHash, err = s.blobs.PutJSON(ctx, tx, c.Messages); err != nil {
			return err
		}
		if c.Memory != nil {
			if n.MemoryHash, err = s.putMemory(ctx, tx, c.Memory); err != nil {
				return err
			}
		}
		if err := insertNode(ctx, tx, n); err != nil {
			return fmt.Errorf("写入整理节点失败: %w", err)
		}
		for _, r := range c.UsageRecords {
			r.SaveID, r.NodeID = c.SaveID, n.ID
			if err := usage.Insert(ctx, tx, r); err != nil {
				return fmt.Errorf("写入用量失败: %w", err)
			}
		}
		return setHead(ctx, tx, c.SaveID, n.ID, false)
	})
	if err != nil {
		return nil, err
	}
	return n, nil
}
