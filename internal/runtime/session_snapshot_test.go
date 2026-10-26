package runtime

import (
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/save"
)

func TestSessionSnapshotBoundary(t *testing.T) {
	m := NewManager(Deps{Bus: event.NewBus(2)})
	in := save.TurnInput{Kind: save.InputFree, Text: "推门"}
	m.publish("s", event.TurnPending, event.TurnStartedData{TurnID: "t", Input: in})
	m.publish("s", event.TurnStarted, event.TurnStartedData{TurnID: "t", ParentID: "p", Input: in})
	m.publish("s", event.TurnText, event.TurnTextData{TurnID: "t", Seq: 1, Delta: "雾"})
	m.publish("s", event.TurnText, event.TurnTextData{TurnID: "t", Seq: 2, Delta: "很浓"})
	for _, cursor := range []uint64{0, 1, 999} {
		sub, replay, resync := m.SubscribeEvents("s", cursor)
		if resync != (cursor != 0) || len(replay) != 1 || replay[0].Type != event.SessionSnapshot {
			t.Fatalf("快照结果：%+v resync=%v", replay, resync)
		}
		s := replay[0].Data.(SessionSnapshot)
		if replay[0].ID != 4 || s.Turn.Seq != 2 || s.Turn.Items[0].Markup != "雾很浓" || s.Turn.ParentID != "p" {
			t.Fatalf("快照边界：%+v", s.Turn)
		}
		sub.Close()
	}
	sub, replay, _ := m.SubscribeEvents("s", 0)
	defer sub.Close()
	snapshot := replay[0].Data.(SessionSnapshot)
	m.publish("s", event.TurnText, event.TurnTextData{TurnID: "t", Seq: 3, Delta: "。"})
	if snapshot.Turn.Items[0].Markup != "雾很浓" || snapshot.Turn.Seq != 2 {
		t.Fatal("发布后修改了已交付的快照")
	}
	if next := <-sub.C; next.ID != replay[0].ID+1 {
		t.Fatalf("快照与增量间存在缺口：%+v", next)
	}
	m.publish("s", event.TurnCommitted, event.TurnCommittedData{TurnID: "t", NodeID: "t", Status: "committed"})
	idle, replay, _ := m.SubscribeEvents("s", 0)
	defer idle.Close()
	if replay[0].Data.(SessionSnapshot).Turn != nil || len(m.live) != 0 {
		t.Fatal("结束后仍保留运行回合")
	}
}

func TestSessionSnapshotPendingAndCompaction(t *testing.T) {
	m := NewManager(Deps{Bus: event.NewBus(0)})
	m.publish("s", event.CompactionStarted, event.CompactionStartedData{Reason: "manual"})
	m.publish("s", event.TurnPending, event.TurnStartedData{TurnID: "queued", Input: save.TurnInput{Kind: save.InputContinue}})
	sub, replay, _ := m.SubscribeEvents("s", 0)
	defer sub.Close()
	s := replay[0].Data.(SessionSnapshot)
	if !s.Compacting || s.Turn.Started || s.Turn.TurnID != "queued" {
		t.Fatalf("排队快照：%+v", s)
	}
	m.publish("s", event.TurnFailed, event.TurnFailedData{TurnID: "previous"})
	if m.live["s"].Turn == nil {
		t.Fatal("其他回合的终态清除了排队输入")
	}
	m.publish("s", event.TurnFailed, event.TurnFailedData{TurnID: "queued"})
	m.publish("s", event.CompactionFailed, event.CompactionFailedData{})
	if len(m.live) != 0 {
		t.Fatal("空闲会话未清理")
	}
}
