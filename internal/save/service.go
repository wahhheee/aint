package save

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/store"
	"github.com/wahhheee/aint/internal/tmpl"
	"github.com/wahhheee/aint/internal/usage"
	"github.com/wahhheee/aint/internal/work"
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
	OpeningID string         `json:"opening_id,omitzero"`
	WorkID    string         `json:"work_id"`
	VersionID string         `json:"version_id,omitzero"` // 为空取最新版本
	Title     string         `json:"title,omitzero"`
	Form      map[string]any `json:"form,omitzero"` // 开局表单：状态路径 → 值
	Settings  Settings       `json:"settings"`
}

// Create 新建存档：保存开局表单，创建初始状态与记忆；静态开场同时写入零 token 回合。
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
	op, err := m.Opening(p.OpeningID)
	if err != nil {
		return nil, errs.Invalid("%v", err)
	}
	init, err := s.prepareOpening(m, op, p.Form)
	if err != nil {
		return nil, err
	}
	now := store.NowMillis()
	sv := &Save{
		ID: ids.New(), WorkID: p.WorkID, WorkVersionID: versionID, WorkTitle: m.Manifest.Title,
		Title: strings.TrimSpace(p.Title), Settings: p.Settings, CreatedAt: now, UpdatedAt: now, LastPlayedAt: now,
		OpeningID: op.ID, InitialForm: init.form,
	}
	if sv.Title != "" {
		sv.TitleSource = "manual"
	}
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		if err := insertSave(ctx, tx, sv); err != nil {
			return err
		}
		return s.startOpening(ctx, tx, sv, m, init)
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
	Compaction    *PreparedCompaction
	ExpectedHead  string
	Kind          NodeKind // 缺省为故事回合；choices 为补充选项
	LockOpening   bool     // 与成功的玩家回合一同提交；开场、续写开场与手动编辑不锁定
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
	if c.ExpectedHead != "" {
		sv, err := getSave(ctx, tx, c.SaveID)
		if err != nil {
			return nil, err
		}
		if sv.HeadNodeID != c.ExpectedHead {
			return nil, errs.Conflict("存档已变化，请刷新后重试")
		}
	}
	if c.Compaction != nil {
		if err := s.insertCompaction(ctx, tx, c.Compaction); err != nil {
			return nil, err
		}
	}
	if c.Kind == "" {
		c.Kind = KindTurn
	}
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
		ID: id, SaveID: c.SaveID, ParentID: c.Parent.ID, Depth: c.Parent.Depth + 1, Kind: c.Kind,
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
	if c.LockOpening && c.Status == StatusCommitted {
		if err := lockOpening(ctx, tx, c.SaveID); err != nil {
			return nil, err
		}
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
	WorkVersionID string
	State         map[string]any
	Notes         []string
	SaveID        string
	Parent        *Node        // 整理时的 head
	Epoch         string       // 新阶段块（L4），存为 blob，阶段内原样复用
	KeepFrom      string       // 保留原文的第一个回合节点 ID
	Memory        *MemoryWrite // 落盘写入的记忆；nil 表示没有改动
	Messages      []llm.Message
	Usage         *llm.Usage
	Meta          NodeMeta
	UsageRecords  []usage.Record
}

// PreparedCompaction 保存已准备的不可变内容；节点尚未加入存档树。
// 重写回合使用它在成功时一并提交整理与新正文。
type PreparedCompaction struct {
	Node    *Node
	Records []usage.Record
}

// StageCompaction 只保存内容寻址的 blob，不写节点、不移动进度；未采用的内容由存储回收清理。
func (s *Service) StageCompaction(ctx context.Context, c CompactionCommit) (*PreparedCompaction, error) {
	n := &Node{
		ID: ids.New(), SaveID: c.SaveID, ParentID: c.Parent.ID, Depth: c.Parent.Depth + 1, Kind: KindCompaction,
		Status: StatusCommitted, TurnNo: c.Parent.TurnNo, KeepFrom: c.KeepFrom, StateHash: c.Parent.StateHash,
		MemoryHash: c.Parent.MemoryHash, WorkVersionID: c.Parent.WorkVersionID, Usage: c.Usage, Meta: c.Meta,
		CreatedAt: store.NowMillis(), Notes: c.Notes,
	}
	if c.WorkVersionID != "" {
		n.WorkVersionID = c.WorkVersionID
	}
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if c.State != nil {
			if n.StateHash, err = s.blobs.PutJSON(ctx, tx, c.State); err != nil {
				return err
			}
		}
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
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &PreparedCompaction{Node: n, Records: c.UsageRecords}, nil
}

func (s *Service) insertCompaction(ctx context.Context, tx *sql.Tx, p *PreparedCompaction) error {
	if err := insertNode(ctx, tx, p.Node); err != nil {
		return err
	}
	for _, r := range p.Records {
		r.SaveID, r.NodeID = p.Node.SaveID, p.Node.ID
		if err := usage.Insert(ctx, tx, r); err != nil {
			return err
		}
	}
	return nil
}

// CommitCompaction 原子提交整理节点、用量与当前进度；升级同时提交状态和作品版本。
func (s *Service) CommitCompaction(ctx context.Context, c CompactionCommit) (*Node, error) {
	p, err := s.StageCompaction(ctx, c)
	if err != nil {
		return nil, err
	}
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		if err := s.insertCompaction(ctx, tx, p); err != nil {
			return err
		}
		return setHead(ctx, tx, c.SaveID, p.Node.ID, false)
	})
	if err != nil {
		return nil, err
	}
	return p.Node, nil
}

// RecordAttempt 保存未采用操作的实际用量，不创建故事节点。
func (s *Service) RecordAttempt(ctx context.Context, saveID string, records []usage.Record) error {
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		for _, r := range records {
			r.SaveID, r.NodeID = saveID, ""
			if err := usage.Insert(ctx, tx, r); err != nil {
				return err
			}
		}
		return nil
	})
}
