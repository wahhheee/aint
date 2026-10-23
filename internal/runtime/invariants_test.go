package runtime_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/save"
)

// longScript：每回合写一段正文、把玩家输入记进 /memory/log.md、给出选项；整理时写一份纪要再给出摘要。
func longScript() *fake.Script {
	filler := strings.Repeat("海风把灯塔的光切成一段一段。", 6)
	return &fake.Script{
		Responses: []fake.Response{
			{Text: "第一段：{{input}}\n\n" + filler, ChunkRunes: 500, DelayMS: 1, ToolCalls: []fake.ToolCall{
				{Name: "write", Args: map[string]any{"path": "/memory/log.md", "content": "{{input}}"}},
			}},
			{Text: filler, ChunkRunes: 500, DelayMS: 1, ToolCalls: []fake.ToolCall{{Name: "offer_choices", Args: map[string]any{
				"choices": []any{map[string]any{"id": "a", "label": "向前"}, map[string]any{"id": "b", "label": "回头"}},
			}}}},
		},
		Compact: []fake.Response{
			{DelayMS: 1, ToolCalls: []fake.ToolCall{
				{Name: "write", Args: map[string]any{"path": "/memory/chronicle/latest.md", "content": "# 纪要\n一路向前。\n"}},
			}},
			{Text: "## 故事至今\n一路向前。\n## 当前场景\n灯塔下。", ChunkRunes: 500, DelayMS: 1},
		},
	}
}

// 200 回合不变量（M3-10）：反复整理、重新生成、回到更早的回合开出新分支，全程检查——
// 同一阶段内相邻回合的请求逐条是前缀（前缀稳定）；上下文规模受阈值约束；进度线上的回合号连续；
// 整理节点的 keep_from 是进度线上的祖先；记忆与原文检索只反映当前分支（分支隔离）；存档条目不增加。
func TestLongPlayInvariants(t *testing.T) {
	if testing.Short() {
		t.Skip("长测试")
	}
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(longScript()), reqs: &reqs})
	ctx := context.Background()
	const budget = 6000
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.first-story",
		Settings: save.Settings{Pace: "short", ContextBudget: budget, KeepRecent: 600}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()

	waitIdle := func() {
		t.Helper()
		for e.mgr.Running(sv.ID) != "" || e.mgr.Compacting(sv.ID) {
			waitAny(t, sub, event.TurnCommitted, event.TurnFailed, event.CompactionDone, event.CompactionFailed)
		}
	}
	compactionsOnLine := func() int {
		_, line, _ := e.saves.ProgressLine(ctx, sv.ID)
		n := 0
		for _, x := range line {
			if x.Kind == save.KindCompaction {
				n++
			}
		}
		return n
	}

	type turnReqs struct {
		start, end int // 本回合在 reqs 中的区间
		epoch      int // 本回合开始时进度线上的整理次数
		plain      bool
	}
	var history []turnReqs
	for i := range 200 {
		epoch := compactionsOnLine()
		start := len(reqs)
		plain := true
		switch {
		case i > 0 && i%23 == 0: // 重新生成最后一个回合
			_, line, _ := e.saves.ProgressLine(ctx, sv.ID)
			last := lastTurnOf(line)
			if _, err := e.mgr.Regenerate(ctx, sv.ID, last.ID, nil); err != nil {
				t.Fatalf("第 %d 步重新生成：%v", i, err)
			}
			plain = false
		case i > 0 && i%41 == 0: // 回到三个回合之前，从那里开出新分支
			_, line, _ := e.saves.ProgressLine(ctx, sv.ID)
			var turns []*save.Node
			for _, x := range line {
				if x.Kind == save.KindTurn {
					turns = append(turns, x)
				}
			}
			back := turns[len(turns)-4]
			if err := e.mgr.Exclusive(ctx, sv.ID, "head", func() error {
				_, err := e.saves.SetHead(ctx, sv.ID, back.ID, save.HeadHere)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputFree, Text: fmt.Sprintf("分支-%d", i)}); err != nil {
				t.Fatal(err)
			}
			plain = false
		default:
			in := save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}
			if i == 0 {
				in = save.TurnInput{Kind: save.InputOpening}
			} else if i%5 == 0 {
				in = save.TurnInput{Kind: save.InputFree, Text: fmt.Sprintf("第 %d 步", i)}
			}
			if _, err := e.mgr.Submit(ctx, sv.ID, in); err != nil {
				t.Fatalf("第 %d 步提交：%v", i, err)
			}
		}
		waitIdle()
		history = append(history, turnReqs{start: start, end: len(reqs), epoch: epoch, plain: plain})
	}

	// 前缀稳定：相邻两步都是普通回合、且中间没有整理时，后一步的首个请求以前一步的最后一个请求为前缀
	narrate := func(from, to int) []*llm.Request {
		var out []*llm.Request
		for _, r := range reqs[from:to] {
			if r.Purpose == "narrate" {
				out = append(out, r)
			}
		}
		return out
	}
	checked := 0
	for k := 1; k < len(history); k++ {
		a, b := history[k-1], history[k]
		if !a.plain || !b.plain || a.epoch != b.epoch {
			continue
		}
		ra, rb := narrate(a.start, a.end), narrate(b.start, b.end)
		if len(ra) == 0 || len(rb) == 0 {
			continue
		}
		prev, next := ra[len(ra)-1], rb[0]
		if len(next.Messages) <= len(prev.Messages) {
			continue // 前一步结束后发生了整理（在本步开始前完成），不在同一阶段
		}
		if string(jsonx.MustMarshal(prev.Messages[0])) != string(jsonx.MustMarshal(next.Messages[0])) {
			continue
		}
		for i := range len(prev.Messages) - 1 {
			if string(jsonx.MustMarshal(prev.Messages[i])) != string(jsonx.MustMarshal(next.Messages[i])) {
				t.Fatalf("第 %d 步：同一阶段内第 %d 条消息变了", k, i)
			}
		}
		checked++
	}
	if checked < 50 {
		t.Fatalf("只检查了 %d 对相邻回合的前缀", checked)
	}

	// 上下文受阈值约束：每次叙事请求的输入（假模型按字数报用量）不超过阈值 + 一个回合的增量
	for i, r := range reqs {
		if r.Purpose != "narrate" {
			continue
		}
		var n int
		for _, m := range r.Messages {
			n += len([]rune(m.TextOf()))
		}
		if n > budget*2 {
			t.Fatalf("第 %d 个请求的上下文过大：%d 字", i, n)
		}
	}

	sv2, line, err := e.saves.ProgressLine(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	onLine := map[string]bool{}
	turnNo := 0
	compactions := 0
	for _, n := range line {
		onLine[n.ID] = true
		switch n.Kind {
		case save.KindTurn:
			if n.TurnNo != turnNo+1 {
				t.Fatalf("进度线上的回合号不连续：%d 之后是 %d", turnNo, n.TurnNo)
			}
			turnNo = n.TurnNo
		case save.KindCompaction:
			compactions++
			if !onLine[n.KeepFrom] {
				t.Fatalf("整理节点的 keep_from 不在进度线上：%s", n.ID)
			}
			if n.Meta.ContextTokens >= budget {
				t.Fatalf("整理后的上下文仍超过阈值：%d", n.Meta.ContextTokens)
			}
		}
	}
	if compactions < 5 || sv2.BranchCount < 5 {
		t.Fatalf("整理 %d 次、分支 %d 个，不足以覆盖本测试的场景", compactions, sv2.BranchCount)
	}

	// 分支隔离：原文索引中当前分支以外的回合不出现在 /transcript；记忆是当前分支最后一次写入的内容
	texts, _ := e.saves.TranscriptTexts(ctx, sv.ID)
	offLine := 0
	for id := range texts {
		if !onLine[id] {
			offLine++
		}
	}
	if offLine == 0 {
		t.Fatal("测试应产生其他分支上的回合")
	}
	head := line[len(line)-1]
	mf, _ := e.saves.LoadMemory(ctx, head.MemoryHash)
	logData, _ := e.saves.Blob(ctx, mf.Files["/memory/log.md"].Blob)
	lastTurn := lastTurnOf(line)
	if lastTurn.Input != nil && lastTurn.Input.Kind == save.InputFree && string(logData) != lastTurn.Input.Text {
		t.Fatalf("head 处的记忆应是最后一个回合写入的内容：%q / %q", logData, lastTurn.Input.Text)
	}

	// 存档不膨胀：游玩、重新生成、分支都在存档内部进行
	if list, _ := e.saves.ListByWork(ctx, "yanyi.first-story"); len(list) != 1 {
		t.Fatalf("存档条目应只有一个：%d", len(list))
	}
	t.Logf("200 步：进度线 %d 回合、整理 %d 次、分支 %d 个、检查前缀 %d 对、其他分支的回合 %d 个",
		turnNo, compactions, sv2.BranchCount, checked, offLine)
}

func lastTurnOf(line []*save.Node) *save.Node {
	for i := len(line) - 1; i >= 0; i-- {
		if line[i].Kind == save.KindTurn {
			return line[i]
		}
	}
	return nil
}

// 故障注入：整理进行中进程关闭（取消）时不留下整理节点，head 不变；重新打开后可以照常整理与游玩。
func TestCompactionInterrupted(t *testing.T) {
	gate := make(chan struct{})
	dir := t.TempDir()
	e, err := openEnv(context.Background(), dir, compactGate{Provider: fake.New(compactScript()), open: gate})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.first-story",
		Settings: save.Settings{Pace: "short", ContextBudget: 4000, KeepRecent: 300}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	for i := 0; !e.mgr.Compacting(sv.ID); i++ {
		if i > 30 {
			t.Fatal("没有触发整理")
		}
		in := save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}
		if i == 0 {
			in = save.TurnInput{Kind: save.InputOpening}
		}
		if _, err := e.mgr.Submit(ctx, sv.ID, in); err != nil {
			t.Fatal(err)
		}
		waitAny(t, sub, event.TurnCommitted)
	}
	sub.Close()
	before, _ := e.saves.Get(ctx, sv.ID)
	if err := e.close(); err != nil { // 整理卡在模型调用上：关闭进程即取消整理
		t.Fatal(err)
	}

	close(gate)
	e2 := setupAt(t, dir, fake.New(compactScript()))
	after, err := e2.saves.Get(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, line, _ := e2.saves.ProgressLine(ctx, sv.ID)
	for _, n := range line {
		if n.Kind == save.KindCompaction {
			t.Fatal("被取消的整理不应留下整理节点")
		}
	}
	if after.HeadNodeID != before.HeadNodeID {
		t.Fatal("被取消的整理不应移动 head")
	}
	sub2, _, _ := e2.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub2.Close()
	if err := e2.mgr.Compact(ctx, sv.ID); err != nil {
		t.Fatal(err)
	}
	waitAny(t, sub2, event.CompactionDone)
}

// compactGate 在 open 关闭之前阻塞整理请求（叙事请求照常）。
type compactGate struct {
	llm.Provider
	open chan struct{}
}

func (g compactGate) Stream(ctx context.Context, m *llm.Model, req *llm.Request) (llm.Stream, error) {
	if req.Purpose == "compact" {
		select {
		case <-g.open:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g.Provider.Stream(ctx, m, req)
}
