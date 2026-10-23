package save

import (
	"context"
	"database/sql"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// Segment 是分支视图中的一段：一串没有分叉的节点（《04-交互与界面设计》8.11）。
// 分支视图只展示分叉点与各分支摘要，不展开整棵树。
type Segment struct {
	StartID     string     `json:"start_id"`             // 段内第一个节点
	EndID       string     `json:"end_id"`               // 段内最后一个节点（分叉点或叶子）
	LeafID      string     `json:"leaf_id"`              // 这一段及其后续中最近创建的叶子：切换到这条分支时 head 去的地方
	FromTurn    int        `json:"from_turn"`            // 段内第一个回合的序号（没有回合时为 0）
	ToTurn      int        `json:"to_turn"`              // 段内最后一个回合的序号
	Turns       int        `json:"turns"`                // 段内回合数
	Summary     string     `json:"summary"`              // 段内第一个回合的输入说明
	Compactions int        `json:"compactions,omitzero"` // 段内整理次数
	Edits       int        `json:"edits,omitzero"`       // 段内玩家修改次数
	Current     bool       `json:"current"`              // 当前进度（head）在这一段或其后续中
	HeadTurn    int        `json:"head_turn,omitzero"`   // head 在这一段内时，head 所在的回合
	HeadInside  bool       `json:"head_inside,omitzero"` // head 在这一段内
	LatestAt    int64      `json:"latest_at"`            // 这一段及其后续中最新节点的创建时间
	Children    []*Segment `json:"children,omitzero"`    // 分叉出的各分支，按创建先后排列
}

// Tree 是存档的分支结构（GET /saves/{id}/tree）。
type Tree struct {
	SaveID string   `json:"save_id"`
	HeadID string   `json:"head_id"`
	Root   *Segment `json:"root"`
}

type treeNode struct {
	id, parent string
	kind       NodeKind
	turnNo     int
	input      *TurnInput
	createdAt  int64
}

// Tree 返回存档的分支结构。
func (s *Service) Tree(ctx context.Context, saveID string) (*Tree, error) {
	sv, err := s.Get(ctx, saveID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Read().QueryContext(ctx, `
		SELECT id, COALESCE(parent_id, ''), kind, COALESCE(turn_no, 0), input, created_at
		FROM nodes WHERE save_id = ? ORDER BY created_at, id`, saveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	children := map[string][]*treeNode{}
	var root *treeNode
	for rows.Next() {
		n := &treeNode{}
		var input sql.NullString
		if err := rows.Scan(&n.id, &n.parent, &n.kind, &n.turnNo, &input, &n.createdAt); err != nil {
			return nil, err
		}
		if input.Valid {
			n.input = &TurnInput{}
			if err := jsonx.Unmarshal([]byte(input.String), n.input); err != nil {
				return nil, err
			}
		}
		if n.parent == "" {
			root = n
			continue
		}
		children[n.parent] = append(children[n.parent], n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if root == nil {
		return &Tree{SaveID: saveID, HeadID: sv.HeadNodeID}, nil
	}
	line, err := pathToRoot(ctx, s.db.Read(), sv.HeadNodeID)
	if err != nil {
		return nil, err
	}
	onLine := map[string]bool{}
	for _, n := range line {
		onLine[n.ID] = true
	}
	b := treeBuilder{children: children, onLine: onLine, head: sv.HeadNodeID}
	return &Tree{SaveID: saveID, HeadID: sv.HeadNodeID, Root: b.segment(root)}, nil
}

type treeBuilder struct {
	children map[string][]*treeNode
	onLine   map[string]bool
	head     string
}

// segment 从 start 开始沿唯一的子节点走到分叉点或叶子，生成一段；分叉出的各分支递归生成。
func (b treeBuilder) segment(start *treeNode) *Segment {
	seg := &Segment{StartID: start.id}
	cur := start
	for {
		switch cur.kind {
		case KindTurn:
			if seg.Turns == 0 {
				seg.FromTurn, seg.Summary = cur.turnNo, InputSummary(cur.input)
			}
			seg.ToTurn = cur.turnNo
			seg.Turns++
		case KindCompaction:
			seg.Compactions++
		case KindEdit:
			seg.Edits++
		}
		seg.Current = seg.Current || b.onLine[cur.id]
		if cur.id == b.head {
			seg.HeadInside, seg.HeadTurn = true, cur.turnNo
		}
		seg.LatestAt = max(seg.LatestAt, cur.createdAt)
		kids := b.children[cur.id]
		if len(kids) != 1 {
			break
		}
		cur = kids[0]
	}
	seg.EndID, seg.LeafID = cur.id, cur.id
	var latest *Segment
	for _, k := range b.children[cur.id] {
		child := b.segment(k)
		seg.Children = append(seg.Children, child)
		if latest == nil || child.LatestAt > latest.LatestAt {
			latest = child
		}
	}
	if latest != nil {
		seg.LeafID = latest.LeafID
		seg.LatestAt = max(seg.LatestAt, latest.LatestAt)
	}
	return seg
}
