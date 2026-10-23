package runtime_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/runtime"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/settings"
)

// compactScript：每回合写一段较长的正文并给出选项；整理时先写一份章节纪要，再给出交接摘要。
func compactScript() *fake.Script {
	filler := strings.Repeat("雾从海面上漫过来，码头的灯一盏接一盏地暗下去。", 8)
	return &fake.Script{
		Responses: []fake.Response{{
			Text: "回应：{{input}}\n\n" + filler, ChunkRunes: 400, DelayMS: 1,
			ToolCalls: []fake.ToolCall{{Name: "offer_choices", Args: map[string]any{
				"choices": []any{map[string]any{"id": "a", "label": "继续向前"}, map[string]any{"id": "b", "label": "停下"}},
			}}},
		}},
		Compact: []fake.Response{
			{DelayMS: 1, ToolCalls: []fake.ToolCall{
				{Name: "write", Args: map[string]any{"path": "/memory/chronicle/01.md", "content": "# 第一章\n雾夜里一路向前。\n"}},
				{Name: "offer_choices", Args: map[string]any{"choices": []any{map[string]any{"id": "a", "label": "不该出现"}}}},
			}},
			{Text: "## 故事至今\n玩家在雾夜里一路向前。\n## 当前场景\n码头。", ChunkRunes: 400, DelayMS: 1},
		},
	}
}

// 整理：回合提交后上下文达到阈值时在后台整理（期间的输入排队），之后的请求以新阶段块开头、后接保留的回合；
// 整理中可以写记忆，不能给选项；上一回合待答的选项在整理后照常作答。
func TestCompaction(t *testing.T) {
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(compactScript()), reqs: &reqs})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.first-story",
		Settings: save.Settings{Pace: "short", ContextBudget: 4000, KeepRecent: 300}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()

	// 一直玩到触发整理
	inputs := []save.TurnInput{{Kind: save.InputOpening}}
	var done event.CompactionDoneData
	for i := 0; ; i++ {
		if i > 30 {
			t.Fatal("30 个回合内没有触发整理")
		}
		in := save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}
		if i < len(inputs) {
			in = inputs[i]
		}
		if _, err := e.mgr.Submit(ctx, sv.ID, in); err != nil {
			t.Fatal(err)
		}
		waitAny(t, sub, event.TurnCommitted)
		if e.mgr.Compacting(sv.ID) {
			// 整理期间提交的输入排队，整理完成后开始
			if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "b"}); err != nil {
				t.Fatalf("整理期间的输入应排队：%v", err)
			}
			done = waitFor(t, sub, event.CompactionDone).Data.(event.CompactionDoneData)
			waitFor(t, sub, event.TurnCommitted)
			break
		}
	}

	_, line, _ := e.saves.ProgressLine(ctx, sv.ID)
	var c *save.Node
	for _, n := range line {
		if n.Kind == save.KindCompaction {
			c = n
		}
	}
	if c == nil || c.ID != done.NodeID || c.KeepFrom == "" || c.Meta.ContextTokens == 0 {
		t.Fatalf("整理节点：%+v", c)
	}
	// 整理中：写记忆可以，给选项不行；待答的选项先补一条工具结果
	var compactReq *llm.Request
	for _, r := range reqs {
		if r.Purpose == "compact" {
			compactReq = r
		}
	}
	task := compactReq.Messages[len(compactReq.Messages)-3] // 指令、助手（写记忆 + 选项）、工具结果之后是摘要调用
	if task.Parts[0].Kind != llm.PartToolResult || task.Parts[0].ToolResult.Parts[0].Text != "（玩家还没有作答，系统先整理上下文）" ||
		!strings.Contains(task.TextOf(), `<system_task kind="compaction">`) {
		t.Fatalf("整理指令消息：%+v", task)
	}
	results := toolResults(compactReq.Messages[len(compactReq.Messages)-1])
	if len(results) != 2 || !strings.HasPrefix(results[0].Parts[0].Text, "# write /memory/chronicle/01.md") ||
		results[1].Parts[0].Text != "整理阶段暂不使用这个工具。" {
		t.Fatalf("整理中的工具结果：%+v", results)
	}
	if mf, _ := e.saves.LoadMemory(ctx, c.MemoryHash); mf.Files["/memory/chronicle/01.md"].Blob == "" {
		t.Fatal("整理节点应带落盘后的记忆")
	}

	// 整理后的第一个回合：首条消息是新阶段块，后接保留的回合；选择照常作为工具结果回填
	next := reqs[len(reqs)-1]
	head := next.Messages[0].TextOf()
	if !strings.Contains(head, `<epoch n="1" from_turn=`) || !strings.Contains(head, "<summary>\n## 故事至今\n玩家在雾夜里一路向前。") ||
		!strings.Contains(head, "/memory/chronicle/01.md — 第一章") {
		t.Fatalf("新阶段块：\n%s", head)
	}
	if strings.Contains(next.Messages[1].TextOf(), "开场") {
		t.Fatal("开场回合应已被摘要掉")
	}
	last := next.Messages[len(next.Messages)-1]
	if last.Parts[0].Kind != llm.PartToolResult || !strings.Contains(last.Parts[0].ToolResult.Parts[0].Text, "停下") {
		t.Fatalf("整理后的选择应作为工具结果：%+v", last.Parts[0])
	}
	for _, m := range next.Messages {
		if strings.Contains(m.TextOf(), "<system_task") {
			t.Fatal("整理过程的消息不进入之后的上下文")
		}
	}
	tl, _ := e.saves.Timeline(ctx, sv.ID, "", 100)
	kinds := map[save.NodeKind]int{}
	for _, n := range tl.Nodes {
		kinds[n.Kind]++
	}
	if kinds[save.KindCompaction] != 1 {
		t.Fatalf("时间线应标出整理：%v", kinds)
	}

	// 手动整理
	if err := e.mgr.Compact(ctx, sv.ID); err != nil {
		t.Fatal(err)
	}
	ev := waitAny(t, sub, event.CompactionDone, event.CompactionFailed)
	if ev.Type == event.CompactionFailed {
		if msg := ev.Data.(event.CompactionFailedData).Message; msg != "内容还不多，暂时不需要整理" {
			t.Fatalf("手动整理失败：%s", msg)
		}
	}
}

// waitAny 等待任一类型的事件（不把 turn.failed 当作失败）。
func waitAny(t *testing.T, sub *event.Subscription, types ...string) event.Event {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				t.Fatal("订阅被关闭")
			}
			if slices.Contains(types, ev.Type) {
				return ev
			}
		case <-timeout:
			t.Fatalf("等待事件 %v 超时", types)
		}
	}
}

// staticSettings 是固定的全局设置。
type staticSettings struct{ s settings.Settings }

func (s staticSettings) Get(context.Context) (settings.Settings, error) { return s.s, nil }

// separate 模式：轻量模型单独整理，输入是本阶段原文；整理后的阶段内，相邻请求的消息序列逐条是前缀（前缀稳定）。
func TestCompactionSeparateAndPrefix(t *testing.T) {
	var reqs []*llm.Request
	g := settings.Defaults()
	g.Models.Light = "light"
	g.Context.Mode = "separate"
	e, err := openEnvWith(context.Background(), t.TempDir(), fakeModels{recordingProvider{Provider: fake.New(compactScript()), reqs: &reqs}},
		func(d *runtime.Deps) { d.Settings = staticSettings{g} })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.first-story",
		Settings: save.Settings{Pace: "short", ContextBudget: 4000, KeepRecent: 300}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	compactions := 0
	for i := 0; compactions < 2; i++ {
		if i > 60 {
			t.Fatal("60 个回合内没有整理两次")
		}
		in := save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}
		if i == 0 {
			in = save.TurnInput{Kind: save.InputOpening}
		}
		if _, err := e.mgr.Submit(ctx, sv.ID, in); err != nil {
			t.Fatal(err)
		}
		waitAny(t, sub, event.TurnCommitted)
		if e.mgr.Compacting(sv.ID) {
			waitAny(t, sub, event.CompactionDone)
			compactions++
		}
	}
	var sep *llm.Request
	for _, r := range reqs {
		if r.Purpose == "compact" {
			sep = r
		}
	}
	if sep.System[0].Text != prompts.CompactionSeparateSystem || !strings.Contains(sep.Messages[0].TextOf(), "<transcript>") ||
		!strings.Contains(sep.Messages[0].TextOf(), "<previous_summary>\n## 故事至今") {
		t.Fatalf("separate 整理的请求：%s", sep.Messages[0].TextOf())
	}
	for _, s := range sep.Tools {
		if s.Name == "offer_choices" {
			t.Fatal("separate 整理只提供记忆与资料工具")
		}
	}
	// 第二次整理之后再玩两个回合：两次请求的消息逐条字节相等（只追加）
	for range 2 {
		if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}); err != nil {
			t.Fatal(err)
		}
		waitAny(t, sub, event.TurnCommitted)
	}
	var narrate []*llm.Request
	for _, r := range reqs {
		if r.Purpose == "narrate" {
			narrate = append(narrate, r)
		}
	}
	a, b := narrate[len(narrate)-2], narrate[len(narrate)-1]
	for i := range len(a.Messages) - 1 { // a 的最后一条是 a 回合的玩家消息，b 中它后面接着助手消息
		if string(jsonx.MustMarshal(a.Messages[i])) != string(jsonx.MustMarshal(b.Messages[i])) {
			t.Fatalf("整理后的阶段内第 %d 条消息变了", i)
		}
	}
}

// overflowProvider 在请求的消息数超过 limit 时报一次上下文超限。
type overflowProvider struct {
	llm.Provider
	limit int
	hit   *int
}

func (p overflowProvider) Stream(ctx context.Context, m *llm.Model, req *llm.Request) (llm.Stream, error) {
	if req.Purpose == "narrate" && len(req.Messages) > p.limit && *p.hit == 0 {
		*p.hit++
		return nil, &llm.APIError{Kind: llm.ErrContextOverflow, Message: "prompt is too long"}
	}
	return p.Provider.Stream(ctx, m, req)
}

// 模型报上下文超限：在回合内紧急整理（separate 的输入方式，没有轻量模型时用主笔模型），整理节点成为本回合的父节点，重试成功。
func TestOverflowCompaction(t *testing.T) {
	hit := 0
	e := setupWith(t, overflowProvider{Provider: fake.New(compactScript()), limit: 12, hit: &hit})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.first-story", Settings: save.Settings{Pace: "short", KeepRecent: 300}})
	if err != nil {
		t.Fatal(err)
	}
	var last string
	for i := 0; hit == 0; i++ {
		if i > 20 {
			t.Fatal("没有触发上下文超限")
		}
		in := save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}
		if i == 0 {
			in = save.TurnInput{Kind: save.InputOpening}
		}
		last = play(t, e, sv.ID, in)
	}
	n, err := e.saves.Node(ctx, sv.ID, last)
	if err != nil {
		t.Fatal(err)
	}
	parent, _ := e.saves.Node(ctx, sv.ID, n.ParentID)
	if parent.Kind != save.KindCompaction || parent.Meta.CompactReason != "overflow" || n.Status != save.StatusCommitted {
		t.Fatalf("超限后的回合应在整理节点之下提交：%+v / %+v", n, parent.Meta)
	}
}

// 预算小于常驻部分加保留原文时，每个回合都超过阈值：上次整理之后至少 3 个回合才再整理，不会每个回合都整理。
func TestCompactionNotEveryTurn(t *testing.T) {
	e := setup(t, compactScript())
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.first-story",
		Settings: save.Settings{Pace: "short", ContextBudget: 1000, KeepRecent: 100}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	for i := range 12 {
		in := save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}
		if i == 0 {
			in = save.TurnInput{Kind: save.InputOpening}
		}
		if _, err := e.mgr.Submit(ctx, sv.ID, in); err != nil {
			t.Fatal(err)
		}
		waitAny(t, sub, event.TurnCommitted)
		if e.mgr.Compacting(sv.ID) {
			waitAny(t, sub, event.CompactionDone, event.CompactionFailed)
		}
	}
	_, line, _ := e.saves.ProgressLine(ctx, sv.ID)
	n := 0
	for _, x := range line {
		if x.Kind == save.KindCompaction {
			n++
		}
	}
	if n < 2 || n > 4 {
		t.Fatalf("12 个回合整理了 %d 次（应为每 3 个回合至多一次）", n)
	}
}
