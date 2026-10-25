package save

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/ids"
	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/store"
	"github.com/wahhheee/yanyi/internal/vfs"
)

// 玩家修改状态或记忆（《02-技术方案》6.2"改状态/记忆"）：在 head 下创建 edit 节点，写入新的状态快照或记忆清单，
// 并留下注记；注记附在下一回合玩家消息的 <notes> 中告知主笔（不悄悄改历史）。

// EditMemory 修改记忆文件：content 为 nil 表示删除。内容没有变化时不产生节点，返回 nil。
func (s *Service) EditMemory(ctx context.Context, saveID, path string, content *string) (*Node, error) {
	p, err := vfs.Clean(path)
	if err != nil {
		return nil, errs.Invalid("路径不合法：%v", err)
	}
	if err := vfs.Writable(p); err != nil {
		return nil, errs.Invalid("%v", err)
	}
	_, line, err := s.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, err
	}
	head := line[len(line)-1]
	mf, err := s.LoadMemory(ctx, head.MemoryHash)
	if err != nil {
		return nil, err
	}
	old, existed := mf.Files[p]
	w := &MemoryWrite{Files: mf.Files, Data: map[string][]byte{}}
	var note string
	if content == nil {
		if !existed {
			return nil, errs.NotFound("记忆文件 %s 不存在", p)
		}
		delete(w.Files, p)
		note = "玩家删除了记忆：" + p
	} else {
		data := []byte(strings.ReplaceAll(*content, "\r\n", "\n"))
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return nil, errs.Invalid("记忆内容需要是文本")
		}
		if len(data) > vfs.MaxMemoryFile {
			return nil, errs.Invalid("单个记忆文件最多 %s", vfs.FormatSize(vfs.MaxMemoryFile))
		}
		total := len(data)
		for q, f := range mf.Files {
			if q != p {
				total += f.Size
			}
		}
		if total > vfs.MaxMemoryTotal {
			return nil, errs.Invalid("记忆总量最多 %s", vfs.FormatSize(vfs.MaxMemoryTotal))
		}
		if existed && old.Blob == store.HashBytes(data) {
			return nil, nil
		}
		w.Files[p] = MemoryFile{Size: len(data), Turn: head.TurnNo}
		w.Data[p] = data
		note = "玩家修改了记忆：" + p
		if !existed {
			note = "玩家新建了记忆：" + p
		}
	}
	return s.addEdit(ctx, saveID, head, editContent{memory: w, notes: []string{note}})
}

// EditState 修改状态：原子执行一组操作（state.Apply），返回新节点与实际发生的变化。
func (s *Service) EditState(ctx context.Context, saveID string, ops []state.Op) (*Node, []state.Change, error) {
	_, line, err := s.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, nil, err
	}
	head := line[len(line)-1]
	m, err := s.works.Model(ctx, head.WorkVersionID)
	if err != nil {
		return nil, nil, err
	}
	cur, err := s.LoadState(ctx, head.StateHash)
	if err != nil {
		return nil, nil, err
	}
	var migration []state.MigrationValue
	if m.IsDev {
		cur, migration = state.Migrate(m.State, cur)
	}
	next, changes, err := state.Apply(m.State, cur, ops)
	if err != nil {
		return nil, nil, errs.Invalid("%v", err)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		parts[i] = state.Describe(c)
	}
	notes := []string{"玩家修改了状态：" + strings.Join(parts, "；")}
	if m.IsDev && head.Meta.WorkHash != m.TreeHash {
		notes = append(notes, "作品已重载，请使用当前作品定义与状态。")
	}
	n, err := s.addEdit(ctx, saveID, head, editContent{state: next, notes: notes, migration: migration, workHash: m.TreeHash})
	return n, changes, err
}

type editContent struct {
	state     map[string]any // 非 nil 时写入新快照
	memory    *MemoryWrite   // 非 nil 时写入新清单
	notes     []string
	migration []state.MigrationValue
	workHash  string
}

// addEdit 在 head 下创建 edit 节点并移动 head。
func (s *Service) addEdit(ctx context.Context, saveID string, head *Node, c editContent) (*Node, error) {
	n := &Node{
		ID: ids.New(), SaveID: saveID, ParentID: head.ID, Depth: head.Depth + 1, Kind: KindEdit, Status: StatusCommitted,
		TurnNo: head.TurnNo, StateHash: head.StateHash, MemoryHash: head.MemoryHash, WorkVersionID: head.WorkVersionID,
		Notes: c.notes, CreatedAt: store.NowMillis(),
		Meta: NodeMeta{Migration: c.migration, WorkHash: head.Meta.WorkHash},
	}
	if c.workHash != "" {
		n.Meta.WorkHash = c.workHash
	}
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if c.state != nil {
			if n.StateHash, err = s.blobs.PutJSON(ctx, tx, c.state); err != nil {
				return err
			}
		}
		if c.memory != nil {
			if n.MemoryHash, err = s.putMemory(ctx, tx, c.memory); err != nil {
				return err
			}
		}
		if err := insertNode(ctx, tx, n); err != nil {
			return err
		}
		return setHead(ctx, tx, saveID, n.ID, false)
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "玩家修改了存档", "save_id", saveID, "node", n.ID, "notes", c.notes)
	return n, nil
}

// PendingNotes 返回进度线末尾、最后一个回合之后的系统注记（玩家修改了状态或记忆等），按发生顺序。
// 它们附在下一回合玩家消息的 <notes> 中。
func PendingNotes(line []*Node) []string {
	var out []string
	for i := len(line) - 1; i >= 0 && line[i].Kind != KindTurn; i-- {
		out = append(append([]string(nil), line[i].Notes...), out...)
	}
	return out
}
