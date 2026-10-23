package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/usage"
)

// pricedModels 让所有存档都使用配置了价格的假模型。
type pricedModels struct {
	p     llm.Provider
	price *llm.Price
}

func (f pricedModels) Resolve(context.Context, string) (llm.Provider, *llm.Model, error) {
	m := fake.Model()
	m.Price = f.price
	return f.p, m, nil
}

// 本回合费用：turn.usage 推送的累计用量与费用，与提交后 usage_log 中该回合的统计一致。
func TestTurnUsageCost(t *testing.T) {
	in, out := 2.0, 8.0
	price := &llm.Price{Input: &in, CacheRead: &in, CacheWrite: &in, Output: &out, Currency: "CNY"}
	ctx := context.Background()
	e, err := openEnvWith(ctx, t.TempDir(), pricedModels{fake.New(quickScript()), price})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })

	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.first-story", Settings: save.Settings{Pace: "short"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	turnID, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputOpening})
	if err != nil {
		t.Fatal(err)
	}

	var last *event.TurnUsageData
	timeout := time.After(5 * time.Second)
wait:
	for {
		select {
		case ev := <-sub.C:
			switch ev.Type {
			case event.TurnUsage:
				d := ev.Data.(event.TurnUsageData)
				last = &d
			case event.TurnCommitted:
				break wait
			case event.TurnFailed:
				t.Fatalf("回合失败：%+v", ev.Data)
			}
		case <-timeout:
			t.Fatal("等待回合提交超时")
		}
	}
	if last == nil {
		t.Fatal("回合中应推送 turn.usage")
	}
	if last.Costs["CNY"] <= 0 || last.Unpriced != 0 {
		t.Fatalf("配置了价格时应给出本回合费用：%+v", last)
	}

	sum, err := usage.Summarize(ctx, e.db.Read(), usage.Filter{SaveID: sv.ID, NodeID: turnID})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Total.Usage != last.Usage.(llm.Usage) || sum.Total.Costs["CNY"] != last.Costs["CNY"] || sum.ByKind["narrate"].Calls == 0 {
		t.Errorf("用量日志 %+v 与 turn.usage %+v 不一致", sum.Total, last)
	}
}
