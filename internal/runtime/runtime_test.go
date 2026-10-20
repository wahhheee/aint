package runtime_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/yanyi/examples"
	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/runtime"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/store"
	"github.com/wahhheee/yanyi/internal/tmpl"
	"github.com/wahhheee/yanyi/internal/tools"
	"github.com/wahhheee/yanyi/internal/work"
)

// fakeModels 让所有存档都使用同一个假模型。
type fakeModels struct{ p llm.Provider }

func (f fakeModels) Resolve(context.Context, string) (llm.Provider, *llm.Model, error) {
	return f.p, fake.Model(), nil
}

type env struct {
	saves *save.Service
	mgr   *runtime.Manager
	bus   *event.Bus
}

func setup(t *testing.T, script *fake.Script) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "yanyi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	blobs := store.NewBlobs(filepath.Join(dir, "blobs"))
	tm := tmpl.New()
	works := work.NewService(db, blobs)
	works.EnsureBuiltins(ctx, examples.Works())
	saves := save.NewService(db, blobs, works, tm)
	bus := event.NewBus(0)
	mgr := runtime.NewManager(runtime.Deps{
		Saves: saves, Works: works, Models: fakeModels{fake.New(script)}, Bus: bus,
		Builder: contextx.NewBuilder(tm), Tools: tools.NewRegistry(), Tmpl: tm,
	})
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })
	return &env{saves: saves, mgr: mgr, bus: bus}
}

// waitFor 等待存档主题上出现指定类型的事件。
func waitFor(t *testing.T, sub *event.Subscription, typ string) event.Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				t.Fatal("订阅被关闭")
			}
			if ev.Type == event.TurnFailed && typ != event.TurnFailed {
				t.Fatalf("回合失败：%+v", ev.Data)
			}
			if ev.Type == typ {
				return ev
			}
		case <-timeout:
			t.Fatalf("等待事件 %s 超时", typ)
		}
	}
}

func quickScript() *fake.Script {
	return &fake.Script{Responses: []fake.Response{{
		Text: "林舟把帽子挂上衣架。\n\n<gold>老陈在午夜前从未离开过柜台。</gold>", ChunkRunes: 50, DelayMS: 1,
		ToolCalls: []fake.ToolCall{{Name: "offer_choices", Args: map[string]any{
			"choices": []any{map[string]any{"id": "a", "label": "追问"}, map[string]any{"id": "b", "label": "离开"}},
		}}},
	}}}
}

// 竖切：静态开场 → 选择选项 → 流式正文 → offer_choices → 提交 → 时间线。
func TestTurnVerticalSlice(t *testing.T) {
	e := setup(t, quickScript())
	ctx := context.Background()

	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.misty-harbor", Form: map[string]any{"player.name": "阿遥"},
		Settings: save.Settings{Pace: "medium", Reasoning: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	if sv.TurnCount != 1 {
		t.Fatalf("静态开场应生成第 1 回合，得到 %d", sv.TurnCount)
	}
	tl, err := e.saves.Timeline(ctx, sv.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tl.Nodes[0].Display[0].Markup, "阿遥——信封上") {
		t.Fatal("开场应以开局表单的值渲染 Liquid")
	}

	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	turnID, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "再来"}); err == nil {
		t.Error("回合进行中再次提交应返回冲突（单一写者）")
	}
	waitFor(t, sub, event.TurnText)
	waitFor(t, sub, event.TurnChoices)
	committed := waitFor(t, sub, event.TurnCommitted).Data.(event.TurnCommittedData)
	if committed.NodeID != turnID {
		t.Fatalf("节点 ID 应等于回合 ID")
	}

	tl, err = e.saves.Timeline(ctx, sv.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	last := tl.Nodes[len(tl.Nodes)-1]
	kinds := []string{}
	for _, it := range last.Display {
		kinds = append(kinds, it.T)
	}
	if strings.Join(kinds, ",") != "input,text,choices" || last.Display[0].Text != "拆开那封信" || last.Stop != "choices" {
		t.Fatalf("第 2 回合的显示项不对：%v / %+v", kinds, last)
	}

	// 下一回合：上一回合以 offer_choices 结束，玩家选择应作为工具结果回填。
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "b"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	_, line, _ := e.saves.ProgressLine(ctx, sv.ID)
	msgs, _ := e.saves.LoadMessages(ctx, line[len(line)-1].MessagesHash)
	first := msgs[0]
	if first.Parts[0].Kind != llm.PartToolResult || !strings.Contains(first.Parts[0].ToolResult.Parts[0].Text, "离开") {
		t.Fatalf("玩家的选择应作为 offer_choices 的工具结果：%+v", first.Parts[0])
	}
}

// 停止：已生成的内容以 interrupted 状态保存。
func TestCancelKeepsContent(t *testing.T) {
	slow := &fake.Script{Responses: []fake.Response{{Text: strings.Repeat("雾越来越浓。", 200), ChunkRunes: 2, DelayMS: 5}}}
	e := setup(t, slow)
	ctx := context.Background()
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
	waitFor(t, sub, event.TurnText)
	if err := e.mgr.Cancel(sv.ID, turnID); err != nil {
		t.Fatal(err)
	}
	c := waitFor(t, sub, event.TurnCommitted).Data.(event.TurnCommittedData)
	if c.Status != string(save.StatusInterrupted) {
		t.Fatalf("停止后的回合应为 interrupted，得到 %s", c.Status)
	}
}
