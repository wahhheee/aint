package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
)

// stalledProvider 的首个流输出可选正文后等待上下文结束，之后的调用使用假模型。
type stalledProvider struct {
	llm.Provider
	text string
	used bool
}

func (p *stalledProvider) Stream(ctx context.Context, model *llm.Model, req *llm.Request) (llm.Stream, error) {
	if p.used {
		return p.Provider.Stream(ctx, model, req)
	}
	p.used = true
	return &stalledStream{ctx: ctx, text: p.text}, nil
}

type stalledStream struct {
	ctx  context.Context
	text string
}

func (s *stalledStream) Next() (llm.Event, error) {
	if s.text != "" {
		text := s.text
		s.text = ""
		return llm.TextDelta{Text: text}, nil
	}
	<-s.ctx.Done()
	return nil, s.ctx.Err()
}

func (s *stalledStream) Close() error { return nil }

func TestReentryCanCancelRunningTurn(t *testing.T) {
	e := setupWith(t, &stalledProvider{Provider: fake.New(quickScript()), text: "雾中有人推开了门。"})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := e.mgr.SubscribeEvents(sv.ID, 0)
	id, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "推门"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, first, event.TurnText)
	first.Close()
	second, replay, _ := e.mgr.SubscribeEvents(sv.ID, 0)
	defer second.Close()
	snapshot := replay[0].Data.(runtime.SessionSnapshot)
	if snapshot.Turn == nil || snapshot.Turn.TurnID != id || snapshot.Turn.Items[0].Markup != "雾中有人推开了门。" {
		t.Fatalf("重新进入没有恢复正文：%+v", snapshot)
	}
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputContinue}); !errs.Is(err, errs.CodeConflict) {
		t.Fatalf("正在生成时应拒绝并行写入：%v", err)
	}
	if err := e.mgr.Cancel(sv.ID, snapshot.Turn.TurnID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, second, event.TurnCommitted)
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputContinue}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, second, event.TurnCommitted)
}

func TestTurnDeadlineReleasesWriter(t *testing.T) {
	for _, text := range []string{"", "雾中有人推开了门。"} {
		t.Run(text, func(t *testing.T) {
			ctx := context.Background()
			p := &stalledProvider{Provider: fake.New(quickScript()), text: text}
			e, err := openEnvWith(ctx, t.TempDir(), fakeModels{p}, func(d *runtime.Deps) {
				d.Budget = runtime.DefaultBudget
				d.Budget.WallClock = 300 * time.Millisecond
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = e.close() })
			sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor"})
			if err != nil {
				t.Fatal(err)
			}
			sub, _, _ := e.mgr.SubscribeEvents(sv.ID, 0)
			defer sub.Close()
			id, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "推门"})
			if err != nil {
				t.Fatal(err)
			}
			if text == "" {
				failed := waitFor(t, sub, event.TurnFailed).Data.(event.TurnFailedData)
				if failed.Code != "budget" {
					t.Fatalf("超时提示：%+v", failed)
				}
			} else {
				committed := waitFor(t, sub, event.TurnCommitted).Data.(event.TurnCommittedData)
				n, err := e.saves.Node(ctx, sv.ID, id)
				if err != nil {
					t.Fatal(err)
				}
				if committed.Status != "interrupted" || n.Meta.Stop != "budget" {
					t.Fatalf("超时内容未按中断保存：%+v", n)
				}
				display, err := e.saves.LoadDisplay(ctx, n.DisplayHash)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, item := range display {
					if item.T == "text" && item.Markup == text {
						found = true
					}
				}
				if !found {
					t.Fatal("超时后正文丢失")
				}
			}
			if e.mgr.Running(sv.ID) != "" {
				t.Fatal("超时后仍占用写者")
			}
			if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputContinue}); err != nil {
				t.Fatal(err)
			}
			waitFor(t, sub, event.TurnCommitted)
		})
	}
}
