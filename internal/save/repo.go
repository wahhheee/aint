package save

import (
	"context"
	"database/sql"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/store"
)

// 本文件集中存档与节点相关的 SQL。表结构见 store/migrations。

func insertSave(ctx context.Context, q store.Querier, s *Save) error {
	settings, err := jsonx.Marshal(s.Settings)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO saves (id, work_id, work_version_id, title, title_source, head_node_id, settings,
		                   created_at, updated_at, last_played_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.WorkID, s.WorkVersionID, store.NullString(s.Title), store.NullString(s.TitleSource), store.NullString(s.HeadNodeID),
		string(settings), s.CreatedAt, s.UpdatedAt, s.LastPlayedAt)
	return err
}

func setHead(ctx context.Context, q store.Querier, saveID, headID string, played bool) error {
	now := store.NowMillis()
	if played {
		_, err := q.ExecContext(ctx, `UPDATE saves SET head_node_id = ?, updated_at = ?, last_played_at = ? WHERE id = ?`,
			headID, now, now, saveID)
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE saves SET head_node_id = ?, updated_at = ? WHERE id = ?`, headID, now, saveID)
	return err
}

// saveColumns 附带作品标题与 head 的回合号，存档列表与详情都用它。
const saveColumns = `s.id, s.work_id, s.work_version_id, w.title, COALESCE(s.title, ''), COALESCE(s.title_source, ''),
	COALESCE(s.head_node_id, ''), COALESCE(n.turn_no, 0), s.settings, s.created_at, s.updated_at, COALESCE(s.last_played_at, 0)`

const saveFrom = ` FROM saves s JOIN works w ON w.id = s.work_id LEFT JOIN nodes n ON n.id = s.head_node_id `

func scanSave(sc interface{ Scan(...any) error }) (*Save, error) {
	s := &Save{}
	var settings string
	if err := sc.Scan(&s.ID, &s.WorkID, &s.WorkVersionID, &s.WorkTitle, &s.Title, &s.TitleSource, &s.HeadNodeID,
		&s.TurnCount, &settings, &s.CreatedAt, &s.UpdatedAt, &s.LastPlayedAt); err != nil {
		return nil, err
	}
	if err := jsonx.Unmarshal([]byte(settings), &s.Settings); err != nil {
		return nil, err
	}
	return s, nil
}

func getSave(ctx context.Context, q store.Querier, id string) (*Save, error) {
	return scanSave(q.QueryRowContext(ctx, `SELECT `+saveColumns+saveFrom+`WHERE s.id = ?`, id))
}

func querySaves(ctx context.Context, q store.Querier, where string, args ...any) ([]*Save, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+saveColumns+saveFrom+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Save{}
	for rows.Next() {
		s, err := scanSave(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func insertNode(ctx context.Context, q store.Querier, n *Node) error {
	var input, notes, usage any
	if n.Input != nil {
		b, err := jsonx.Marshal(n.Input)
		if err != nil {
			return err
		}
		input = string(b)
	}
	if len(n.Notes) > 0 {
		notes = string(jsonx.MustMarshal(n.Notes))
	}
	if n.Usage != nil {
		usage = string(jsonx.MustMarshal(n.Usage))
	}
	meta := string(jsonx.MustMarshal(n.Meta))
	var turnNo any
	if n.TurnNo > 0 {
		turnNo = n.TurnNo
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO nodes (id, save_id, parent_id, depth, kind, status, turn_no, input, messages, display, epoch_block,
		                   keep_from, state_hash, memory_hash, work_version_id, notes, usage, meta, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.SaveID, store.NullString(n.ParentID), n.Depth, n.Kind, n.Status, turnNo, input,
		store.NullString(n.MessagesHash), store.NullString(n.DisplayHash), store.NullString(n.EpochHash), store.NullString(n.KeepFrom),
		n.StateHash, n.MemoryHash, n.WorkVersionID, notes, usage, meta, n.CreatedAt)
	return err
}

const nodeColumns = `id, save_id, COALESCE(parent_id, ''), depth, kind, status, COALESCE(turn_no, 0), input,
	COALESCE(messages, ''), COALESCE(display, ''), COALESCE(epoch_block, ''), COALESCE(keep_from, ''),
	state_hash, memory_hash, work_version_id, notes, usage, COALESCE(meta, '{}'), created_at`

func scanNode(sc interface{ Scan(...any) error }) (*Node, error) {
	n := &Node{}
	var input, notes, usage sql.NullString
	var meta string
	if err := sc.Scan(&n.ID, &n.SaveID, &n.ParentID, &n.Depth, &n.Kind, &n.Status, &n.TurnNo, &input,
		&n.MessagesHash, &n.DisplayHash, &n.EpochHash, &n.KeepFrom, &n.StateHash, &n.MemoryHash, &n.WorkVersionID,
		&notes, &usage, &meta, &n.CreatedAt); err != nil {
		return nil, err
	}
	if input.Valid {
		n.Input = &TurnInput{}
		if err := jsonx.Unmarshal([]byte(input.String), n.Input); err != nil {
			return nil, err
		}
	}
	if notes.Valid {
		if err := jsonx.Unmarshal([]byte(notes.String), &n.Notes); err != nil {
			return nil, err
		}
	}
	if usage.Valid {
		if err := jsonx.Unmarshal([]byte(usage.String), &n.Usage); err != nil {
			return nil, err
		}
	}
	if err := jsonx.Unmarshal([]byte(meta), &n.Meta); err != nil {
		return nil, err
	}
	return n, nil
}

func getNode(ctx context.Context, q store.Querier, id string) (*Node, error) {
	return scanNode(q.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE id = ?`, id))
}

// pathToRoot 沿 parent_id 递归取出 root → headID 的进度线（按深度升序）。
func pathToRoot(ctx context.Context, q store.Querier, headID string) ([]*Node, error) {
	rows, err := q.QueryContext(ctx, `
		WITH RECURSIVE line(id) AS (
			SELECT ?
			UNION ALL
			SELECT n.parent_id FROM nodes n JOIN line ON n.id = line.id WHERE n.parent_id IS NOT NULL
		)
		SELECT `+nodeColumns+` FROM nodes WHERE id IN (SELECT id FROM line) ORDER BY depth`, headID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func insertTranscript(ctx context.Context, q store.Querier, saveID, nodeID, text string) error {
	if text == "" {
		return nil
	}
	_, err := q.ExecContext(ctx, `INSERT INTO transcript_fts (text, node_id, save_id) VALUES (?, ?, ?)`, text, nodeID, saveID)
	return err
}
