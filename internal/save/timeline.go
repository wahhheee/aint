package save

import (
	"context"
	"slices"
)

// Versions 是某回合在兄弟节点（同一父节点的多个版本）中的位置，用于"‹ 2/3 ›"切换。
type Versions struct {
	Index int `json:"index"` // 1 起
	Count int `json:"count"`
}

// TimelineNode 是时间线中的一个节点（只含展示所需字段）。
type TimelineNode struct {
	ID        string        `json:"id"`
	ParentID  string        `json:"parent_id"`
	Kind      NodeKind      `json:"kind"`
	Status    NodeStatus    `json:"status"`
	TurnNo    int           `json:"turn_no"`
	CreatedAt int64         `json:"created_at"`
	Display   []DisplayItem `json:"display"`
	Stop      string        `json:"stop,omitzero"`
	Error     string        `json:"error,omitzero"`
	Versions  Versions      `json:"versions"`
}

// Timeline 是当前分支的时间线（分页，向前加载）。
type Timeline struct {
	SaveID  string         `json:"save_id"`
	HeadID  string         `json:"head_id"`
	Nodes   []TimelineNode `json:"nodes"`
	HasMore bool           `json:"has_more"` // 更早的回合还有
}

// Timeline 返回进度线上的回合，before 为空时取最新的 limit 个，否则取 before 之前的 limit 个。
//
// TODO(M5)：2,000 回合级别的存档需要按深度直接分页查询（目前先取整条进度线再切片），并批量查询兄弟数。
func (s *Service) Timeline(ctx context.Context, saveID, before string, limit int) (*Timeline, error) {
	sv, line, err := s.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, err
	}
	turns := slices.DeleteFunc(slices.Clone(line), func(n *Node) bool { return n.Kind != KindTurn })
	if before != "" {
		if i := slices.IndexFunc(turns, func(n *Node) bool { return n.ID == before }); i >= 0 {
			turns = turns[:i]
		}
	}
	tl := &Timeline{SaveID: saveID, HeadID: sv.HeadNodeID, Nodes: []TimelineNode{}}
	if limit > 0 && len(turns) > limit {
		tl.HasMore = true
		turns = turns[len(turns)-limit:]
	}
	for _, n := range turns {
		display, err := s.LoadDisplay(ctx, n.DisplayHash)
		if err != nil {
			return nil, err
		}
		v, err := s.versions(ctx, n)
		if err != nil {
			return nil, err
		}
		tl.Nodes = append(tl.Nodes, TimelineNode{
			ID: n.ID, ParentID: n.ParentID, Kind: n.Kind, Status: n.Status, TurnNo: n.TurnNo,
			CreatedAt: n.CreatedAt, Display: display, Stop: n.Meta.Stop, Error: n.Meta.Error, Versions: v,
		})
	}
	return tl, nil
}

func (s *Service) versions(ctx context.Context, n *Node) (Versions, error) {
	rows, err := s.db.Read().QueryContext(ctx,
		`SELECT id FROM nodes WHERE parent_id = ? AND kind = 'turn' ORDER BY created_at`, n.ParentID)
	if err != nil {
		return Versions{}, err
	}
	defer rows.Close()
	v := Versions{Index: 1}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return v, err
		}
		v.Count++
		if id == n.ID {
			v.Index = v.Count
		}
	}
	return v, rows.Err()
}
