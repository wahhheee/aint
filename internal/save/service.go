package save

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
		body, err := s.tmpl.Render("prompts/opening.md", m.Opening.Body, map[string]any{"state": st})
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
		// TODO(M3-6)：第 0 阶段的阶段块（初始状态 + 记忆目录）；全为空时不生成。
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
			WorkVersionID: versionID, Status: StatusCommitted, PlainText: opening.Markup,
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
	StateHash     string // 未变化时传父节点的哈希
	MemoryHash    string
	WorkVersionID string
	Status        NodeStatus
	Usage         *llm.Usage
	Meta          NodeMeta
	PlainText     string         // 正文纯文本，写入原文索引
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

// TODO(M3-5)：分支操作——Regenerate（兄弟回合）、SetHead(here | latest-leaf)、Rewrite（修改 AI 正文）、
//   Discard（丢弃中断回合）、DeleteBranch（P1）、CopyAsNew（P1，技术方案 6.2–6.4）。
// TODO(M3-2/M3-9)：记忆与状态的玩家编辑（edit 节点 + 注记）。
// TODO(M5-13)：Exit 与标题生成（技术方案 6.5）。

// MemoryEntry 是记忆目录中的一项（阶段块 <memory_index>）。
type MemoryEntry struct {
	Path    string
	Summary string // frontmatter 的 summary 或首个非空行（≤ 60 字）
}

// MemoryIndex 生成记忆目录（《02-技术方案》12.6）。
// TODO(M3-6)：文件超过 80 个时按目录折叠为"目录名（N 个文件）"；frontmatter summary。
func (s *Service) MemoryIndex(ctx context.Context, memoryHash string) ([]MemoryEntry, error) {
	var mf MemoryManifest
	if err := s.blobs.GetJSON(ctx, s.db.Read(), memoryHash, &mf); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(mf.Files))
	for p := range mf.Files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	out := make([]MemoryEntry, 0, len(paths))
	for _, p := range paths {
		data, err := s.blobs.Get(ctx, s.db.Read(), mf.Files[p].Blob)
		if err != nil {
			return nil, err
		}
		out = append(out, MemoryEntry{Path: p, Summary: FirstLineSummary(string(data))})
	}
	return out, nil
}

// FirstLineSummary 取文本的首个非空行（去掉标题符号与注释，≤ 60 字）作为一行说明。
func FirstLineSummary(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
		if line == "" || strings.HasPrefix(line, "<!--") || line == "---" {
			continue
		}
		if r := []rune(line); len(r) > 60 {
			line = string(r[:60]) + "…"
		}
		return line
	}
	return ""
}
