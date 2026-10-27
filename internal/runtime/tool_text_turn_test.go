package runtime_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/usage"
)

func TestToolTextRecovery(t *testing.T) {
	const badChoices = `<offer_choices prompt="第一夜"><choice id="a" label="邮局"/><choice id="b" label="灯塔"/></offer_choices>`
	const badState = `<update_state>{"op":"inc","path":"day","value":1}</update_state>`
	const story = "<say who=\"lin\">雾港的灯亮了。</say>\n"
	update := fake.ToolCall{Name: "update_state", Args: map[string]any{"op": "inc", "path": "day", "value": 1}}
	good := choiceResponse()
	goodState := choiceResponse()
	goodState.ToolCalls = append([]fake.ToolCall{update}, goodState.ToolCalls...)
	bad := fake.Response{Text: story + badChoices}
	for _, tc := range []struct {
		name, stop          string
		responses           []fake.Response
		calls, maxCalls     int
		state, mixed, clean bool
	}{
		{name: "选项标签纠正为按钮", responses: []fake.Response{bad, good}, calls: 2, stop: "choices"},
		{name: "状态和选项一起纠正", responses: []fake.Response{{Text: story + badState + badChoices}, goodState}, calls: 2, stop: "choices", state: true},
		{name: "混合原生写操作不重复执行", responses: []fake.Response{{Text: story + badChoices, ToolCalls: []fake.ToolCall{update}}, goodState}, calls: 2, stop: "choices", state: true, mixed: true},
		{name: "第二次纠正成功", responses: []fake.Response{bad, bad, good}, calls: 3, stop: "choices"},
		{name: "反复错误有上限", responses: []fake.Response{bad}, calls: 3, stop: "tool_format"},
		{name: "纠正后没有实际调用", responses: []fake.Response{bad, {Text: "灯光很暗。"}}, calls: 2, stop: "tool_format"},
		{name: "漏掉状态操作", responses: []fake.Response{{Text: story + badState + badChoices}, good}, calls: 2, stop: "tool_format"},
		{name: "截断伪调用直接纠正", responses: []fake.Response{{Text: story + `<offer_choices choices="`, Stop: "max_tokens"}, good}, calls: 2, stop: "choices"},
		{name: "拒答不重试", responses: []fake.Response{{Text: story + badChoices, Stop: "refusal"}}, calls: 1, stop: "tool_format"},
		{name: "纠正受总预算限制", responses: []fake.Response{bad, good}, maxCalls: 1, calls: 1, stop: "budget"},
		{name: "纠正请求失败保留正文", responses: []fake.Response{bad, {Error: string(llm.ErrNetwork)}}, calls: 2, stop: "error"},
		{name: "正常标签与调用", responses: []fake.Response{{Text: story, ToolCalls: good.ToolCalls}}, calls: 1, stop: "choices", clean: true},
		{name: "代码示例保持原文", responses: []fake.Response{{Text: story + "`<offer_choices/>`", ToolCalls: good.ToolCalls}}, calls: 1, stop: "choices", clean: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.responses {
				tc.responses[i].ChunkRunes, tc.responses[i].DelayMS = 3, 1
				tc.responses[i].Usage = &llm.Usage{InputUncached: 10, Output: 5}
			}
			var reqs []*llm.Request
			p := recordingProvider{Provider: toolTextNativeProvider{fake.New(&fake.Script{Responses: tc.responses})}, reqs: &reqs}
			e, err := openEnvWith(t.Context(), t.TempDir(), fakeModels{p}, func(d *runtime.Deps) {
				d.Budget = runtime.DefaultBudget
				if tc.maxCalls > 0 {
					d.Budget.MaxCalls = tc.maxCalls
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = e.close() })
			sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "aint.misty-harbor"})
			if err != nil {
				t.Fatal(err)
			}
			_, before, err := e.saves.ProgressLine(t.Context(), sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			initial, err := e.saves.LoadState(t.Context(), before[len(before)-1].StateHash)
			if err != nil {
				t.Fatal(err)
			}
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			if _, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputFree, Text: "检查信件柜"}); err != nil {
				t.Fatal(err)
			}
			var streamed strings.Builder
			waiting := true
			for waiting {
				select {
				case ev := <-sub.C:
					switch ev.Type {
					case event.TurnText:
						streamed.WriteString(ev.Data.(event.TurnTextData).Delta)
					case event.TurnCommitted, event.TurnFailed:
						waiting = false
					}
				case <-time.After(5 * time.Second):
					t.Fatal("纠正回合未结束")
				}
			}
			tl, err := e.saves.Timeline(t.Context(), sv.ID, "", 1)
			if err != nil {
				t.Fatal(err)
			}
			n := tl.Nodes[len(tl.Nodes)-1]
			if len(reqs) != tc.calls || n.Stop != tc.stop {
				t.Fatalf("调用 %d 次，停止 %s；期望 %d、%s", len(reqs), n.Stop, tc.calls, tc.stop)
			}
			var displayed strings.Builder
			for _, item := range n.Display {
				if item.T == "text" {
					displayed.WriteString(item.Markup)
				}
			}
			if displayed.String() != streamed.String() || !strings.Contains(streamed.String(), story) {
				t.Fatalf("流式正文与存储不一致：%q / %q", streamed.String(), displayed.String())
			}
			if !tc.clean && (strings.Contains(streamed.String(), "<offer_choices") || strings.Contains(streamed.String(), "<update_state")) {
				t.Fatal("伪工具标签泄漏到了玩家正文")
			}
			if (n.Status == save.StatusInterrupted) != (tc.stop != "choices") {
				t.Fatalf("回合状态错误：%s", n.Status)
			}
			_, line, err := e.saves.ProgressLine(t.Context(), sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			head := line[len(line)-1]
			if head.Meta.Calls != tc.calls || head.Meta.PromptVersion != prompts.Version {
				t.Fatalf("调用记录错误：%+v", head.Meta)
			}
			sum, err := usage.Summarize(t.Context(), e.db.Read(), usage.Filter{SaveID: sv.ID, NodeID: head.ID})
			charged := tc.calls
			if tc.stop == "error" {
				charged-- // 请求尚未建立时没有供应商用量。
			}
			if err != nil || sum.Total.Calls != charged {
				t.Fatalf("纠正调用未完整计入用量：%+v / %v", sum.Total, err)
			}
			current, err := e.saves.LoadState(t.Context(), head.StateHash)
			if err != nil {
				t.Fatal(err)
			}
			wantDay := initial["day"].(float64)
			if tc.state {
				wantDay++
			}
			if current["day"] != wantDay {
				t.Fatalf("状态更新执行次数错误：%v，应为 %v", current["day"], wantDay)
			}
			if !tc.clean && len(reqs) > 1 {
				req := reqs[1]
				last := req.Messages[len(req.Messages)-1]
				if !strings.Contains(last.TextOf(), "原生 function calling") || req.Messages[len(req.Messages)-2].TextOf() != tc.responses[0].Text {
					t.Fatal("应保留模型原文并追加纠正提示")
				}
				if string(req.Messages[len(req.Messages)-2].Native["test/signed"]) != `{"signature":"保留原文与签名"}` {
					t.Fatal("纠正请求不应改写协议原生内容")
				}
				if tc.mixed && (last.Parts[0].ToolResult == nil || !last.Parts[0].ToolResult.IsError) {
					t.Fatal("被拒绝的原生调用必须有配对错误结果")
				}
			}
			if tc.stop == "choices" {
				// 下一回合仍按真正的 offer_choices 调用 ID 回填玩家选择。
				if _, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}); err != nil {
					t.Fatal(err)
				}
				waitFor(t, sub, event.TurnCommitted)
				turn := reqs[tc.calls].Messages
				last := turn[len(turn)-1]
				if last.Parts[0].ToolResult == nil {
					t.Fatal("玩家作答未回填为真实工具结果")
				}
				if !tc.clean {
					for _, msg := range turn {
						if strings.Contains(msg.TextOf(), "<offer_choices") || strings.Contains(msg.TextOf(), "<update_state") || strings.Contains(msg.TextOf(), "上一条回复把工具") {
							t.Fatal("后续回合不应回传伪调用和纠正示范")
						}
					}
					stored, err := e.saves.LoadMessages(t.Context(), head.MessagesHash)
					if err != nil || !strings.Contains(string(jsonx.MustMarshal(stored)), "<offer_choices") {
						t.Fatal("原始错误输出应继续保留在存储记录中")
					}
				}
			}
		})
	}
}

type toolTextNativeProvider struct{ llm.Provider }

func (p toolTextNativeProvider) Stream(ctx context.Context, model *llm.Model, req *llm.Request) (llm.Stream, error) {
	s, err := p.Provider.Stream(ctx, model, req)
	if err != nil {
		return nil, err
	}
	return toolTextNativeStream{s}, nil
}

type toolTextNativeStream struct{ llm.Stream }

func (s toolTextNativeStream) Next() (llm.Event, error) {
	ev, err := s.Stream.Next()
	if done, ok := ev.(llm.Done); ok {
		done.Message.Native = map[string]jsonx.Value{"test/signed": jsonx.Value(`{"signature":"保留原文与签名"}`)}
		ev = done
	}
	return ev, err
}

func TestToolTextCancel(t *testing.T) {
	const story = "雾港的灯亮了。"
	p := fake.New(&fake.Script{Responses: []fake.Response{{
		Text: story + `<offer_choices>` + strings.Repeat("参数", 200), ChunkRunes: 20, DelayMS: 5,
	}, choiceResponse()}})
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: p, reqs: &reqs})
	sv := choicesSave(t, e, "")
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	id, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputOpening})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnText)
	if err := e.mgr.Cancel(sv.ID, id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	tl, err := e.saves.Timeline(t.Context(), sv.ID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	n := tl.Nodes[0]
	if n.Stop != "canceled" || n.Status != save.StatusInterrupted || len(reqs) != 1 {
		t.Fatalf("取消后不应继续纠正：%s / %s，调用 %d 次", n.Stop, n.Status, len(reqs))
	}
	for _, item := range n.Display {
		if item.T == "text" && item.Markup != story {
			t.Fatalf("取消后仅应保留故事正文：%q", item.Markup)
		}
	}
}
