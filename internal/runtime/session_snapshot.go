package runtime

import (
	"slices"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/save"
)

// SessionSnapshot 是已发布事件的当前会话投影，保留正在运行的操作和最近的中断提示。
// 它不读取回合 goroutine 的可变草稿；新连接可从快照继续消费事件。
type SessionSnapshot struct {
	Failure        *event.TurnFailedData       `json:"failure,omitempty"`
	CompactFailure *event.CompactionFailedData `json:"compact_failure,omitempty"`
	Turn           *LiveTurn                   `json:"turn"`
	Compacting     bool                        `json:"compacting"`
	Helper         string                      `json:"helper,omitzero"`
	Usage          *event.TurnUsageData        `json:"usage,omitempty"`
}

// LiveTurn 包含排队、准备或生成中的回合。Started 为 false 时尚无正文与确定的父节点。
type LiveTurn struct {
	RegenerateID string             `json:"-"`
	Operation    any                `json:"operation,omitempty"`
	TurnID       string             `json:"turn_id"`
	ParentID     string             `json:"parent_id"`
	Input        save.TurnInput     `json:"input"`
	Started      bool               `json:"started"`
	Items        []save.DisplayItem `json:"items"`
	Seq          int                `json:"seq"`
	ToolsRunning int                `json:"tools_running"`
}

// SubscribeEvents 原子取得事件订阅和会话快照。首次连接或无法续传时，用当前快照替代历史事件；
// 快照的 ID 是其包含的最后一条事件，订阅通道只含此边界之后的事件。
func (m *Manager) SubscribeEvents(saveID string, lastID uint64) (*event.Subscription, []event.Event, bool) {
	m.liveMu.Lock()
	defer m.liveMu.Unlock()
	topic := event.SaveTopic(saveID)
	sub, replay, resync := m.d.Bus.Subscribe(topic, lastID)
	if lastID == 0 || resync {
		snapshot := SessionSnapshot{}
		if current := m.live[saveID]; current != nil {
			snapshot = *current
			if current.Turn != nil {
				turn := *current.Turn
				turn.Items = slices.Clone(turn.Items)
				snapshot.Turn = &turn
			}
		}
		replay = []event.Event{{ID: m.d.Bus.LastID(topic), Type: event.SessionSnapshot, Data: snapshot}}
	}
	return sub, replay, resync
}

// updateSnapshot 的调用方持有 liveMu。显示项的嵌套内容与用量载荷在发布后不可修改。
func (m *Manager) updateSnapshot(saveID string, data any, typ string) any {
	s := m.live[saveID]
	if s == nil {
		s = &SessionSnapshot{}
		m.live[saveID] = s
	}
	switch v := data.(type) {
	case event.TurnStartedData:
		s.Failure = nil
		if typ == event.TurnPending && !s.Compacting {
			s.CompactFailure = nil
		}
		if typ == event.TurnStarted && s.Turn != nil && s.Turn.TurnID == v.TurnID {
			v.RegenerateID = s.Turn.RegenerateID
		}
		s.Turn = &LiveTurn{TurnID: v.TurnID, ParentID: v.ParentID, Input: v.Input.(save.TurnInput), Operation: v.Operation, Started: typ == event.TurnStarted, Items: []save.DisplayItem{}}
		s.Turn.RegenerateID = v.RegenerateID
		s.Helper, s.Usage = "", nil
	case event.TurnTextData:
		if d := s.Turn; d != nil && d.TurnID == v.TurnID {
			d.Seq = v.Seq
			if n := len(d.Items); n > 0 && d.Items[n-1].T == "text" {
				d.Items[n-1].Markup += v.Delta
			} else {
				d.Items = append(d.Items, save.DisplayItem{T: "text", Markup: v.Delta})
			}
		}
	case event.TurnDisplayData:
		if d := s.Turn; d != nil && d.TurnID == v.TurnID {
			d.Items = append(d.Items, v.Item.(save.DisplayItem))
		}
	case event.TurnChoicesData:
		if d := s.Turn; d != nil && d.TurnID == v.TurnID {
			d.Items = append(d.Items, v.Choices.(save.DisplayItem))
		}
	case event.TurnToolData:
		if d := s.Turn; d != nil && d.TurnID == v.TurnID && v.Display == "subtle" {
			if v.Phase == "start" {
				d.ToolsRunning++
			} else {
				d.ToolsRunning = max(0, d.ToolsRunning-1)
			}
		}
	case event.TurnUsageData:
		if s.Turn != nil && s.Turn.TurnID == v.TurnID {
			s.Usage = &v
		}
	case event.HelperProgressData:
		if s.Turn != nil && s.Turn.TurnID == v.TurnID {
			s.Helper = ""
			if v.Phase == "start" {
				s.Helper = v.Kind
			}
		}
	case event.TurnCommittedData:
		if s.Turn != nil && s.Turn.TurnID == v.TurnID {
			s.Turn, s.Usage, s.Helper = nil, nil, ""
		}
	case event.TurnFailedData:
		if s.Turn != nil && s.Turn.TurnID != v.TurnID {
			break
		}
		if s.Turn != nil && s.Turn.TurnID == v.TurnID {
			v.Input, v.Operation, v.RegenerateID = s.Turn.Input, s.Turn.Operation, s.Turn.RegenerateID
			if v.NodeID == "" {
				v.ParentID = s.Turn.ParentID
			}
			s.Turn, s.Usage, s.Helper = nil, nil, ""
		}
		if v.Message != "" {
			s.Failure = &v
		}
		data = v
	case event.TimelineChangedData:
		s.Failure, s.CompactFailure = nil, nil
	case event.CompactionStartedData:
		s.CompactFailure = nil
		s.Compacting = true
	case event.CompactionDoneData:
		s.CompactFailure = nil
		s.Compacting = false
	case event.CompactionFailedData:
		if v.Message != "" {
			if v.AttemptID == "" {
				v.AttemptID = ids.New()
			}
			s.CompactFailure = &v
			data = v
		}
		s.Compacting = false
	}
	if s.Turn == nil && !s.Compacting && s.Failure == nil && s.CompactFailure == nil {
		delete(m.live, saveID)
	}
	return data
}
