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
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/store"
)

// 分支操作（《02-技术方案》6.2）。这里的操作只改节点树与 head，不调用模型；需要调用模型的重新生成、
// 修改玩家输入由 runtime 以指定的父节点提交新回合。调用方（runtime.Manager）保证同一存档同一时刻只有一个写者。

// head 的移动方式（POST /saves/{id}/head）。
const (
	HeadHere       = "here"        // 回到此处：head = 指定节点，下一次提交即在此分叉
	HeadLatestLeaf = "latest-leaf" // 切换到该节点所在分支的最新进度：head = 其子树中最近创建的叶子
)

// Node 返回存档中的一个节点。
func (s *Service) Node(ctx context.Context, saveID, nodeID string) (*Node, error) {
	n, err := scanNode(s.db.Read().QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE id = ? AND save_id = ?`, nodeID, saveID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.NotFound("存档中没有这个回合")
	}
	return n, err
}

// ProgressLineAt 返回 root → nodeID 的路径（按深度升序）。
func (s *Service) ProgressLineAt(ctx context.Context, saveID, nodeID string) ([]*Node, error) {
	if _, err := s.Node(ctx, saveID, nodeID); err != nil {
		return nil, err
	}
	line, err := pathToRoot(ctx, s.db.Read(), nodeID)
	if err != nil {
		return nil, fmt.Errorf("读取进度线失败: %w", err)
	}
	return line, nil
}

// SetHead 移动 head（回到此处 / 切换到某分支的最新进度），返回新的 head 节点 ID。
func (s *Service) SetHead(ctx context.Context, saveID, nodeID, mode string) (string, error) {
	n, err := s.Node(ctx, saveID, nodeID)
	if err != nil {
		return "", err
	}
	target := n.ID
	switch mode {
	case HeadHere, "":
	case HeadLatestLeaf:
		if target, err = latestLeaf(ctx, s.db.Read(), saveID, n.ID); err != nil {
			return "", err
		}
	default:
		return "", errs.Invalid("未知的 mode：%q（可用 here、latest-leaf）", mode)
	}
	err = s.db.Tx(ctx, func(tx *sql.Tx) error { return setHead(ctx, tx, saveID, target, false) })
	if err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "已移动 head", "save_id", saveID, "head", target, "mode", mode)
	return target, nil
}

// latestLeaf 返回 nodeID 子树（含自身）中最近创建的叶子。
func latestLeaf(ctx context.Context, q store.Querier, saveID, nodeID string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, subtreeCTE+`
		SELECT n.id FROM nodes n
		WHERE n.id IN (SELECT id FROM sub)
		  AND NOT EXISTS (SELECT 1 FROM nodes k WHERE k.save_id = n.save_id AND k.parent_id = n.id)
		ORDER BY n.created_at DESC, n.id DESC LIMIT 1`, nodeID, saveID).Scan(&id)
	return id, err
}

// subtree 返回以 nodeID 为根的子树中全部节点 ID（含自身）。
func subtree(ctx context.Context, q store.Querier, saveID, nodeID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, subtreeCTE+`SELECT id FROM sub`, nodeID, saveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func hasChildren(ctx context.Context, q store.Querier, saveID, nodeID string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE save_id = ? AND parent_id = ?`, saveID, nodeID).Scan(&n)
	return n > 0, err
}

// Rewrite 修改 AI 正文：创建原回合的兄弟节点，复制输入、状态、记忆、工具调用与结果，把正文换成编辑后的文本；
// 不调用模型。消息中各段助手文本合并为一段放在第一段的位置，助手消息的协议原生内容（推理签名等）去掉——
// 内容已经改变，原样回传会与之不符。新节点成为 head，之后从这里继续。
func (s *Service) Rewrite(ctx context.Context, saveID, nodeID, text string) (*Node, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return nil, errs.Invalid("正文不能为空")
	}
	n, err := s.Node(ctx, saveID, nodeID)
	if err != nil {
		return nil, err
	}
	if n.Kind != KindTurn || n.ParentID == "" {
		return nil, errs.Invalid("只能修改回合的正文")
	}
	parent, err := s.Node(ctx, saveID, n.ParentID)
	if err != nil {
		return nil, err
	}
	msgs, err := s.LoadMessages(ctx, n.MessagesHash)
	if err != nil {
		return nil, err
	}
	display, err := s.LoadDisplay(ctx, n.DisplayHash)
	if err != nil {
		return nil, err
	}
	m, err := s.works.Model(ctx, n.WorkVersionID)
	if err != nil {
		return nil, err
	}
	display = rewriteDisplay(display, text)
	var node *Node
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		node, err = s.commitTx(ctx, tx, TurnCommit{
			SaveID: saveID, Parent: parent, TurnNo: n.TurnNo, Input: *n.Input,
			Messages: rewriteMessages(msgs, text), Display: display,
			StateHash: n.StateHash, MemoryHash: n.MemoryHash, WorkVersionID: n.WorkVersionID, Status: StatusCommitted,
			PlainText: TranscriptBody(display, m.SpeakerName),
			Meta:      NodeMeta{Model: n.Meta.Model, Stop: n.Meta.Stop, PrefixHash: n.Meta.PrefixHash, PromptVersion: n.Meta.PromptVersion, EditedFrom: n.ID},
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "已修改回合正文", "save_id", saveID, "from", n.ID, "node", node.ID, "turn_no", n.TurnNo)
	return node, nil
}

// rewriteMessages 把助手消息中的文本换成编辑后的一段：第一段文本的位置放新文本，其余文本与推理摘要去掉，
// 去掉协议原生内容；变空的助手消息删除。没有任何助手文本时放在第一条助手消息的开头。
func rewriteMessages(msgs []llm.Message, text string) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	placed := false
	for _, m := range msgs {
		if m.Role != llm.RoleAssistant {
			out = append(out, m)
			continue
		}
		nm := llm.Message{Role: m.Role}
		for _, p := range m.Parts {
			switch p.Kind {
			case llm.PartText:
				if !placed {
					nm.Parts = append(nm.Parts, llm.Text(text))
					placed = true
				}
			case llm.PartReasoning:
			default:
				nm.Parts = append(nm.Parts, p)
			}
		}
		if len(nm.Parts) > 0 {
			out = append(out, nm)
		}
	}
	if !placed {
		for i, m := range out {
			if m.Role == llm.RoleAssistant {
				out[i].Parts = append([]llm.Part{llm.Text(text)}, m.Parts...)
				return out
			}
		}
		out = append(out, llm.Message{Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text(text)}})
	}
	return out
}

// rewriteDisplay 把显示项中的正文换成编辑后的一段（第一个正文项的位置），其他显示项不变。
func rewriteDisplay(display []DisplayItem, text string) []DisplayItem {
	out := make([]DisplayItem, 0, len(display)+1)
	placed := false
	for _, it := range display {
		if it.T == "text" {
			if !placed {
				out = append(out, DisplayItem{T: "text", Markup: text})
				placed = true
			}
			continue
		}
		out = append(out, it)
	}
	if !placed {
		at := 0
		if len(out) > 0 && out[0].T == "input" {
			at = 1
		}
		out = slices.Insert(out, at, DisplayItem{T: "text", Markup: text})
	}
	return out
}

// Discard 丢弃中断的回合：删除该节点（必须是没有后续的叶子），head 在它上面时回到父节点。返回新的 head。
func (s *Service) Discard(ctx context.Context, saveID, nodeID string) (string, error) {
	n, err := s.Node(ctx, saveID, nodeID)
	if err != nil {
		return "", err
	}
	if n.Kind != KindTurn || n.Status != StatusInterrupted {
		return "", errs.Invalid("只能丢弃中断的回合")
	}
	var head string
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		if more, err := hasChildren(ctx, tx, saveID, n.ID); err != nil {
			return err
		} else if more {
			return errs.Invalid("这个回合之后还有内容，不能丢弃")
		}
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(head_node_id, '') FROM saves WHERE id = ?`, saveID).Scan(&head); err != nil {
			return err
		}
		if err := deleteSubtree(ctx, tx, saveID, n.ID); err != nil {
			return err
		}
		if head == n.ID {
			head = n.ParentID
			return setHead(ctx, tx, saveID, head, false)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "已丢弃中断的回合", "save_id", saveID, "node", n.ID)
	return head, nil
}

// DeleteBranch 删除以 nodeID 为根、且不包含 head 的分支（P1）：节点与原文索引一并删除，blob 交给 GC。
func (s *Service) DeleteBranch(ctx context.Context, saveID, nodeID string) (int, error) {
	n, err := s.Node(ctx, saveID, nodeID)
	if err != nil {
		return 0, err
	}
	if n.Kind == KindRoot {
		return 0, errs.Invalid("不能删除整个存档的根")
	}
	var count int
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		ids, err := subtree(ctx, tx, saveID, n.ID)
		if err != nil {
			return err
		}
		var head string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(head_node_id, '') FROM saves WHERE id = ?`, saveID).Scan(&head); err != nil {
			return err
		}
		if slices.Contains(ids, head) {
			return errs.Conflict("当前进度在这条分支上，先切换到别的分支再删除")
		}
		count = len(ids)
		return deleteSubtree(ctx, tx, saveID, n.ID)
	})
	if err != nil {
		return 0, err
	}
	slog.InfoContext(ctx, "已删除分支", "save_id", saveID, "node", n.ID, "nodes", count)
	return count, nil
}

// subtreeCTE 是以参数（节点 ID、存档 ID）为根的子树（含根）。
const subtreeCTE = `WITH RECURSIVE sub(id) AS (
	SELECT ?
	UNION ALL
	SELECT n.id FROM nodes n JOIN sub ON n.save_id = ? AND n.parent_id = sub.id
) `

// deleteSubtree 删除以 rootID 为根的整棵子树及其原文索引。
func deleteSubtree(ctx context.Context, tx *sql.Tx, saveID, rootID string) error {
	if _, err := tx.ExecContext(ctx, subtreeCTE+`DELETE FROM transcript_fts WHERE save_id = ? AND node_id IN (SELECT id FROM sub)`,
		rootID, saveID, saveID); err != nil {
		return fmt.Errorf("删除原文索引失败: %w", err)
	}
	// 一条语句删除整棵子树：外键（parent_id）在语句结束时检查，子树内部的引用不构成违反
	if _, err := tx.ExecContext(ctx, subtreeCTE+`DELETE FROM nodes WHERE save_id = ? AND id IN (SELECT id FROM sub)`,
		rootID, saveID, saveID); err != nil {
		return fmt.Errorf("删除节点失败: %w", err)
	}
	return nil
}

// ForkInfo 记录"复制为新存档"的来源。
type ForkInfo struct {
	SaveID string `json:"save_id"`
	NodeID string `json:"node_id"`
	TurnNo int    `json:"turn_no"`
	Title  string `json:"title,omitzero"` // 来源存档当时的标题（列表中注明"复制自《…》第 k 回合"）
}

// CopyAsNew 把 root → nodeID 这条路径复制为一个独立的新存档（P1，6.4）：复制节点行（新 ID，父子关系对应）与原文索引；
// 状态、记忆、消息等 blob 内容寻址共享，不复制内容。新存档标题为空，head 为复制出的 nodeID。
func (s *Service) CopyAsNew(ctx context.Context, saveID, nodeID string) (*Save, error) {
	sv, err := s.Get(ctx, saveID)
	if err != nil {
		return nil, err
	}
	line, err := s.ProgressLineAt(ctx, saveID, nodeID)
	if err != nil {
		return nil, err
	}
	texts, err := s.TranscriptTexts(ctx, saveID)
	if err != nil {
		return nil, err
	}
	last := line[len(line)-1]
	now := store.NowMillis()
	turnNo := 0
	for _, n := range line {
		if n.Kind == KindTurn {
			turnNo = n.TurnNo
		}
	}
	nsv := &Save{
		ID: ids.New(), WorkID: sv.WorkID, WorkVersionID: sv.WorkVersionID, WorkTitle: sv.WorkTitle,
		Settings: sv.Settings, CreatedAt: now, UpdatedAt: now, LastPlayedAt: now, TurnCount: turnNo,
		ForkedFrom: &ForkInfo{SaveID: sv.ID, NodeID: last.ID, TurnNo: turnNo, Title: sv.Title},
	}
	newID := map[string]string{}
	for _, n := range line {
		newID[n.ID] = ids.New()
	}
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		nsv.HeadNodeID = newID[last.ID]
		if err := insertSave(ctx, tx, nsv); err != nil {
			return err
		}
		for _, n := range line {
			cp := *n
			cp.ID, cp.SaveID, cp.ParentID = newID[n.ID], nsv.ID, newID[n.ParentID]
			cp.KeepFrom = newID[n.KeepFrom]
			cp.Meta.EditedFrom = "" // 指向原存档中的兄弟节点，复制后没有意义
			if err := insertNode(ctx, tx, &cp); err != nil {
				return err
			}
			if err := insertTranscript(ctx, tx, nsv.ID, cp.ID, texts[n.ID]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("复制存档失败: %w", err)
	}
	slog.InfoContext(ctx, "已复制为新存档", "from", saveID, "node", nodeID, "save_id", nsv.ID, "nodes", len(line))
	return nsv, nil
}

func marshalFork(f *ForkInfo) any {
	if f == nil {
		return nil
	}
	return string(jsonx.MustMarshal(f))
}
