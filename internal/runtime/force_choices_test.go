package runtime_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/runtime"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/usage"
	"github.com/wahhheee/yanyi/internal/work"
)

func choicesSave(t *testing.T, e *env, features string) *save.Save {
	t.Helper()
	set := work.NewFileSet()
	if err := set.Add("work.yaml", []byte("format: 1\nid: test.choices\nversion: 0.1.0\ntitle: 选项测试\n"+features)); err != nil {
		t.Fatal(err)
	}
	works := work.NewService(e.db, e.blobs)
	if _, err := works.ImportFileSet(t.Context(), set, work.Source{Type: "dir"}); err != nil {
		t.Fatal(err)
	}
	sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "test.choices"})
	if err != nil {
		t.Fatal(err)
	}
	return sv
}

func choiceResponse() fake.Response {
	return fake.Response{ToolCalls: []fake.ToolCall{{Name: "offer_choices", Args: map[string]any{
		"choices": []any{map[string]any{"id": "a", "label": "推开门"}, map[string]any{"id": "b", "label": "留在原地"}},
	}}}}
}

func TestForceChoices(t *testing.T) {
	const enabled = "features:\n  force_choices: true\n"
	story := fake.Response{Text: "门后传来了脚步声。"}
	read := fake.Response{ToolCalls: []fake.ToolCall{{Name: "list", Args: map[string]any{"path": "/memory"}}}}
	invalid := fake.Response{ToolCalls: []fake.ToolCall{{Name: "offer_choices", Args: map[string]any{"choices": []any{}}}}}
	for _, tc := range []struct {
		name, features, stop string
		responses            []fake.Response
		kind                 string
		calls, nudgeAt       int
		maxCalls             int
		choices, interrupted bool
		warning              string
	}{
		{name: "默认关闭", responses: []fake.Response{story}, calls: 1, stop: "end"},
		{name: "禁用自由输入不隐式开启", features: "features:\n  free_input: false\n  force_choices: false\n", responses: []fake.Response{story}, calls: 1, stop: "end"},
		{name: "正文之后补选项", features: enabled, responses: []fake.Response{story, choiceResponse()}, calls: 2, nudgeAt: 1, stop: "choices", choices: true},
		{name: "文字选项仍需工具调用", features: enabled, responses: []fake.Response{{Text: "门后传来了脚步声。\n（可选行动：A 开门；B 等候）"}, choiceResponse()}, calls: 2, nudgeAt: 1, stop: "choices", choices: true},
		{name: "HTML选项仍需工具调用", features: enabled, responses: []fake.Response{{Text: "门后传来了脚步声。\n<button>开门</button><button>等候</button>"}, choiceResponse()}, calls: 2, nudgeAt: 1, stop: "choices", choices: true},
		{name: "已有选项直接结束", features: enabled, responses: []fake.Response{{Text: story.Text, ToolCalls: choiceResponse().ToolCalls}}, calls: 1, stop: "choices", choices: true},
		{name: "补调用保留工具往返", features: enabled, responses: []fake.Response{read, story, read, choiceResponse()}, calls: 4, nudgeAt: 2, stop: "choices", choices: true},
		{name: "选项参数错误后重试", features: enabled, responses: []fake.Response{story, invalid, choiceResponse()}, calls: 3, nudgeAt: 1, stop: "choices", choices: true},
		{name: "忽略提示只补调一次", features: enabled, responses: []fake.Response{story}, calls: 2, nudgeAt: 1, stop: "choices_missing", interrupted: true, warning: "模型仍未给出选项"},
		{name: "截断续写完成后补选项", features: enabled, responses: []fake.Response{{Text: "门后传来了", Stop: "max_tokens"}, {Text: "脚步声。"}, choiceResponse()}, calls: 3, nudgeAt: 2, stop: "choices", choices: true},
		{name: "纯场外回应不补调", features: enabled, kind: save.InputOOC, responses: []fake.Response{{Text: "<ooc>接下来放慢节奏。</ooc>"}}, calls: 1, stop: "end"},
		{name: "场外后有正文仍补选项", features: enabled, kind: save.InputOOC, responses: []fake.Response{{Text: "<ooc>接下来放慢节奏。</ooc>\n\n" + story.Text}, choiceResponse()}, calls: 2, nudgeAt: 1, stop: "choices", choices: true},
		{name: "空回复不补调", features: enabled, responses: []fake.Response{{}}, calls: 1, stop: "end"},
		{name: "拒答不补调", features: enabled, responses: []fake.Response{{Text: story.Text, Stop: "refusal"}}, calls: 1, stop: "refusal"},
		{name: "补调用失败保留正文", features: enabled, responses: []fake.Response{story, {Error: string(llm.ErrNetwork)}}, calls: 2, nudgeAt: 1, stop: "error", interrupted: true},
		{name: "补调用受回合预算限制", features: enabled, responses: []fake.Response{story, choiceResponse()}, maxCalls: 1, calls: 1, stop: "budget", warning: "已达本回合模型调用上限"},
		{name: "空工具调用按自然结束处理", features: enabled, responses: []fake.Response{{Text: story.Text, Stop: "tool_use"}, choiceResponse()}, calls: 2, nudgeAt: 1, stop: "choices", choices: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.responses {
				tc.responses[i].ChunkRunes, tc.responses[i].DelayMS = 1000, 1
			}
			var reqs []*llm.Request
			provider := recordingProvider{Provider: fake.New(&fake.Script{Responses: tc.responses}), reqs: &reqs}
			e, err := openEnvWith(t.Context(), t.TempDir(), fakeModels{provider}, func(d *runtime.Deps) {
				d.Budget = runtime.DefaultBudget
				if tc.maxCalls > 0 {
					d.Budget.MaxCalls = tc.maxCalls
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = e.close() })
			sv := choicesSave(t, e, tc.features)
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			kind := tc.kind
			if kind == "" {
				kind = save.InputOpening
			}
			if _, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: kind, Text: "放慢节奏"}); err != nil {
				t.Fatal(err)
			}
			waitFor(t, sub, event.TurnCommitted)
			if len(reqs) != tc.calls {
				t.Fatalf("调用 %d 次，应为 %d 次", len(reqs), tc.calls)
			}
			tl, err := e.saves.Timeline(t.Context(), sv.ID, "", 50)
			if err != nil {
				t.Fatal(err)
			}
			node := tl.Nodes[len(tl.Nodes)-1]
			if node.Stop != tc.stop || (node.Status == save.StatusInterrupted) != tc.interrupted {
				t.Errorf("回合结果不符：stop=%s status=%s", node.Stop, node.Status)
			}
			var shown, notices strings.Builder
			hasChoices := false
			for _, it := range node.Display {
				switch it.T {
				case "text":
					shown.WriteString(it.Markup)
				case "choices":
					hasChoices = true
				case "notice":
					notices.WriteString(it.Text)
				}
			}
			if hasChoices != tc.choices || !strings.Contains(shown.String(), tc.responses[0].Text) {
				t.Errorf("显示项应保留正文与选项：%+v", node.Display)
			}
			if tc.warning != "" && !strings.Contains(notices.String(), tc.warning) {
				t.Errorf("缺少提示 %q：%s", tc.warning, notices.String())
			}
			if tc.nudgeAt > 0 {
				req := reqs[tc.nudgeAt]
				last := req.Messages[len(req.Messages)-1]
				if last.Role != llm.RoleUser || last.TextOf() != prompts.ForceChoicesNotice {
					t.Fatalf("正文自然结束后应追加工具提示：%+v", last)
				}
			}
			if len(reqs) > 1 {
				checkAdapterPrefix(t, reqs, storeBlobs{e})
			}
			_, line, err := e.saves.ProgressLine(t.Context(), sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			msgs, err := e.saves.LoadMessages(t.Context(), line[len(line)-1].MessagesHash)
			if err != nil {
				t.Fatal(err)
			}
			nudges := 0
			for _, msg := range msgs {
				if msg.Role == llm.RoleUser && msg.TextOf() == prompts.ForceChoicesNotice {
					nudges++
				}
			}
			// 预算在追加提示之后检查，未发出的提示也随本回合消息保存。
			wantNudges := 0
			if tc.nudgeAt > 0 || tc.maxCalls > 0 {
				wantNudges = 1
			}
			if nudges != wantNudges {
				t.Errorf("回合应保存 %d 条补调用提示，得到 %d 条", wantNudges, nudges)
			}
		})
	}
}

func TestForceChoicesHistoryAndUsage(t *testing.T) {
	u1 := llm.Usage{InputUncached: 100, Output: 20}
	u2 := llm.Usage{InputUncached: 180, Output: 10}
	choices := choiceResponse()
	choices.Usage = &u2
	script := &fake.Script{Responses: []fake.Response{
		{Text: "门后传来了脚步声。", Usage: &u1, ChunkRunes: 1000, DelayMS: 1},
		choices,
		{Text: "你留在原地。", ToolCalls: choiceResponse().ToolCalls, ChunkRunes: 1000, DelayMS: 1},
	}}
	var reqs []*llm.Request
	provider := recordingProvider{Provider: fake.New(script), reqs: &reqs}
	inputPrice, outputPrice := 2.0, 8.0
	e, err := openEnvWith(t.Context(), t.TempDir(), pricedModels{provider, &llm.Price{Input: &inputPrice, Output: &outputPrice, Currency: "CNY"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })
	sv := choicesSave(t, e, "features:\n  force_choices: true\n")
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	id, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputOpening})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	sum, err := usage.Summarize(t.Context(), e.db.Read(), usage.Filter{SaveID: sv.ID, NodeID: id})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Total.Calls != 2 || sum.Total.Usage != u1.Add(u2) || sum.Total.Costs["CNY"] != 800 {
		t.Fatalf("正文与补调用应合计用量和费用：%+v", sum.Total)
	}
	if _, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "b"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	if len(reqs) != 3 {
		t.Fatalf("两回合应调用 3 次模型，得到 %d 次", len(reqs))
	}
	msgs := reqs[2].Messages
	last := msgs[len(msgs)-1]
	if last.Parts[0].ToolResult == nil || last.Parts[0].ToolResult.CallID != "call_1_1" || !strings.Contains(last.Parts[0].ToolResult.Parts[0].Text, "留在原地") {
		t.Fatalf("玩家选择应回填到补调用的 offer_choices：%+v", last)
	}
	checkAdapterPrefix(t, reqs, storeBlobs{e})
}

// choicesGate 在补调用开始时通知测试，并等待取消。
type choicesGate struct {
	llm.Provider
	ready chan struct{}
}

func (p choicesGate) Stream(ctx context.Context, model *llm.Model, req *llm.Request) (llm.Stream, error) {
	if req.Messages[len(req.Messages)-1].TextOf() == prompts.ForceChoicesNotice {
		close(p.ready)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return p.Provider.Stream(ctx, model, req)
}

func TestCancelForceChoicesKeepsNarrative(t *testing.T) {
	ready := make(chan struct{})
	e := setupWith(t, choicesGate{Provider: fake.New(&fake.Script{Responses: []fake.Response{{Text: "门后传来了脚步声。", ChunkRunes: 1000, DelayMS: 1}}}), ready: ready})
	sv := choicesSave(t, e, "features:\n  force_choices: true\n")
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	id, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputOpening})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("补调用未开始")
	}
	if err := e.mgr.Cancel(sv.ID, id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	tl, err := e.saves.Timeline(t.Context(), sv.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	node := tl.Nodes[len(tl.Nodes)-1]
	if node.Status != save.StatusInterrupted || node.Stop != "canceled" || node.Display[0].Markup != "门后传来了脚步声。" {
		t.Fatalf("补调用取消后应保留正文：%+v", node)
	}
}
