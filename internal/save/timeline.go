package save

import (
	"context"
	"slices"
)

// Versions 是某回合在兄弟节点（同一父节点的多个版本）中的位置，用于"‹ 2/3 ›"切换。
type Versions struct {
	Index int      `json:"index"` // 1 起
	Count int      `json:"count"`
	IDs   []string `json:"ids,omitzero"` // 各版本的节点 ID（按创建先后）；只有一个版本时省略
}

// TimelineNode 是时间线中的一个节点（只含展示所需字段）。
// 回合节点带显示项与版本信息；编辑节点带注记（玩家修改了状态或记忆）；整理节点只标出位置。
type TimelineNode struct {
	ID         string        `json:"id"`
	ParentID   string        `json:"parent_id"`
	Kind       NodeKind      `json:"kind"`
	Status     NodeStatus    `json:"status"`
	TurnNo     int           `json:"turn_no"`
	CreatedAt  int64         `json:"created_at"`
	Display    []DisplayItem `json:"display"`
	Notes      []string      `json:"notes,omitzero"`
	Stop       string        `json:"stop,omitzero"`
	Error      string        `json:"error,omitzero"`
	EditedFrom string        `json:"edited_from,omitzero"` // 修改正文得到的版本：原回合的节点 ID
	Versions   Versions      `json:"versions"`
	// 整理节点：写成交接摘要的回合区间与保留原文的起点
	SummaryFrom  int `json:"summary_from,omitzero"`
	SummaryTo    int `json:"summary_to,omitzero"`
	KeepFromTurn int `json:"keep_from_turn,omitzero"`
}

// Timeline 是当前分支的时间线（分页，向前加载）。
type Timeline struct {
	SaveID   string         `json:"save_id"`
	HeadID   string         `json:"head_id"`
	Nodes    []TimelineNode `json:"nodes"`
	HasMore  bool           `json:"has_more"`  // 更早的回合还有
	HasLater bool           `json:"has_later"` // head 之后还有内容（回到了更早的回合）：继续游玩会开启新分支
}

// Timeline 返回进度线上的节点（回合、编辑、整理），before 为空时取最新的 limit 个，否则取 before 之前的 limit 个。
//
// TODO(M5)：2,000 回合级别的存档需要按深度直接分页查询（目前先取整条进度线再切片），并批量查询兄弟数。
func (s *Service) Timeline(ctx context.Context, saveID, before string, limit int) (*Timeline, error) {
	sv, line, err := s.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, err
	}
	turns := slices.DeleteFunc(slices.Clone(line), func(n *Node) bool { return n.Kind == KindRoot })
	if before != "" {
		if i := slices.IndexFunc(turns, func(n *Node) bool { return n.ID == before }); i >= 0 {
			turns = turns[:i]
		}
	}
	tl := &Timeline{SaveID: saveID, HeadID: sv.HeadNodeID, Nodes: []TimelineNode{}}
	if tl.HasLater, err = hasChildren(ctx, s.db.Read(), saveID, sv.HeadNodeID); err != nil {
		return nil, err
	}
	if limit > 0 && len(turns) > limit {
		tl.HasMore = true
		turns = turns[len(turns)-limit:]
	}
	for _, n := range turns {
		tn := TimelineNode{
			ID: n.ID, ParentID: n.ParentID, Kind: n.Kind, Status: n.Status, TurnNo: n.TurnNo, CreatedAt: n.CreatedAt,
			Display: []DisplayItem{}, Notes: n.Notes, Stop: n.Meta.Stop, Error: n.Meta.Error, EditedFrom: n.Meta.EditedFrom,
			SummaryFrom: n.Meta.SummaryFrom, SummaryTo: n.Meta.SummaryTo, KeepFromTurn: n.Meta.KeepFromTurn,
		}
		if n.Kind == KindTurn {
			if tn.Display, err = s.LoadDisplay(ctx, n.DisplayHash); err != nil {
				return nil, err
			}
			if tn.Versions, err = s.versions(ctx, n); err != nil {
				return nil, err
			}
		}
		tl.Nodes = append(tl.Nodes, tn)
	}
	return tl, nil
}

// Siblings 返回与回合 n 同一父节点的全部回合版本（按创建先后），含 n 自身。
func (s *Service) Siblings(ctx context.Context, n *Node) ([]string, error) {
	rows, err := s.db.Read().QueryContext(ctx,
		`SELECT id FROM nodes WHERE save_id = ? AND parent_id = ? AND kind = 'turn' ORDER BY created_at, id`, n.SaveID, n.ParentID)
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

func (s *Service) versions(ctx context.Context, n *Node) (Versions, error) {
	sibs, err := s.Siblings(ctx, n)
	if err != nil {
		return Versions{}, err
	}
	v := Versions{Index: 1, Count: len(sibs)}
	if i := slices.Index(sibs, n.ID); i >= 0 {
		v.Index = i + 1
	}
	if len(sibs) > 1 {
		v.IDs = sibs
	}
	return v, nil
}
