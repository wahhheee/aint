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
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/state"
	"github.com/wahhheee/aint/internal/usage"
	"github.com/wahhheee/aint/internal/work"
)

func operationSave(t *testing.T, e *env) (*save.Save, *save.Node) {
	t.Helper()
	files := work.NewFileSet()
	for name, body := range map[string]string{
		"work.yaml":  "format: 1\nid: test.operations\nversion: 0.1.0\ntitle: 雨夜集市\nopenings:\n  - {id: default, title: 雨夜, mode: generate}\nfeatures:\n  force_choices: false\n",
		"state.yaml": "fields:\n  gold: {type: number, default: 10, desc: 玩家持有的金币，购买后扣除实际价格}\n",
	} {
		if err := files.Add(name, []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	works := work.NewService(e.db, e.blobs)
	if _, err := works.ImportFileSet(t.Context(), files, work.Source{Type: "dir"}); err != nil {
		t.Fatal(err)
	}
	sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "test.operations", Settings: save.Settings{Pace: "150-250", KeepRecent: 100}})
	if err != nil {
		t.Fatal(err)
	}
	root, err := e.saves.Node(t.Context(), sv.ID, sv.HeadNodeID)
	if err != nil {
		t.Fatal(err)
	}
	return sv, root
}

func operation(t *testing.T, e *env, saveID, nodeID, kind, instructions string) string {
	t.Helper()
	sv, err := e.saves.Get(t.Context(), saveID)
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(saveID), 0)
	defer sub.Close()
	id, err := e.mgr.Operate(t.Context(), saveID, save.TurnOperation{Kind: kind, NodeID: nodeID, Instructions: instructions, ExpectedHead: sv.HeadNodeID})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	return id
}

func TestRewriteIsolation(t *testing.T) {
	var reqs []*llm.Request
	script := &fake.Script{Responses: []fake.Response{
		{Text: "你花两枚金币买下红灯。", ToolCalls: []fake.ToolCall{
			{Name: "update_state", Args: map[string]any{"op": "inc", "path": "gold", "value": -2}},
			{Name: "write", Args: map[string]any{"path": "/memory/purchase.md", "content": "买下红灯"}},
		}},
		{Text: "你提着红灯离开摊位。"},
		{Text: "你没有购物，留在原地。", ToolCalls: []fake.ToolCall{
			{Name: "get_state", Args: map[string]any{}},
			{Name: "list", Args: map[string]any{"path": "/memory"}},
			{Name: "search", Args: map[string]any{"query": "红灯"}},
		}},
		{Text: "雨声填满沉默。"},
		{Text: "你望向远处。"},
	}}
	e := setupWith(t, recordingProvider{Provider: fake.New(script), reqs: &reqs})
	sv, root := operationSave(t, e)
	original := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	old, _ := e.saves.Node(t.Context(), sv.ID, original)
	newID := operation(t, e, sv.ID, original, "rewrite", "取消购买，保留雨夜气氛")
	n, _ := e.saves.Node(t.Context(), sv.ID, newID)
	if n.ParentID != root.ID || n.StateHash != root.StateHash || n.MemoryHash != root.MemoryHash || n.TurnNo != old.TurnNo {
		t.Fatalf("重写未恢复起点快照：%+v", n)
	}
	if got := memoryText(t, e, sv.ID, old.ID, "/memory/purchase.md"); got != "买下红灯" {
		t.Fatalf("原版记忆被改动：%s", got)
	}
	request := reqs[2].Messages[len(reqs[2].Messages)-1].TextOf()
	if !strings.Contains(request, "红灯") || !strings.Contains(request, "取消购买") {
		t.Fatal("重写请求缺少原文或修改要求")
	}
	results := toolResults(reqs[3].Messages[len(reqs[3].Messages)-1])
	if len(results) != 3 || strings.Contains(string(jsonx.MustMarshal(results)), "买下红灯") {
		t.Fatal("原版事件泄漏进重写工具环境")
	}
	msgs, _ := e.saves.LoadMessages(t.Context(), n.MessagesHash)
	if strings.Contains(string(jsonx.MustMarshal(msgs)), "rewrite_request") {
		t.Fatal("临时参考进入了新分支历史")
	}
	tl, _ := e.saves.Timeline(t.Context(), sv.ID, "", 20)
	if len(tl.Nodes) != 1 || tl.Nodes[0].Versions.Count != 2 || strings.Contains(timelineTurns(t, e, sv.ID), "红灯") {
		t.Fatalf("正文或版本：%+v", tl)
	}
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "我继续观察"})
	if strings.Contains(string(jsonx.MustMarshal(reqs[4].Messages)), "花两枚金币买下红灯") {
		t.Fatal("后续叙事仍收到旧版")
	}
}

func TestRequestedChoicesReadOnly(t *testing.T) {
	var reqs []*llm.Request
	choice := choiceResponse()
	choice.Text = "好的，这是你的选项。Let me offer choices."
	choice.ToolCalls = append([]fake.ToolCall{
		{Name: "update_state", Args: map[string]any{"op": "inc", "path": "gold", "value": -9}},
		{Name: "write", Args: map[string]any{"path": "/memory/forbidden.md", "content": "不应写入"}},
		{Name: "roll_dice", Args: map[string]any{"expr": "1d6"}},
		{Name: "delegate", Args: map[string]any{"kind": "curate", "task": "不应执行"}},
	}, choice.ToolCalls...)
	e := setupWith(t, recordingProvider{Provider: fake.New(&fake.Script{Responses: []fake.Response{{Text: "门后传来脚步声。"}, choice, {Text: "你推开了门。"}}}), reqs: &reqs})
	sv, _ := operationSave(t, e)
	id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	before, _ := e.saves.Node(t.Context(), sv.ID, id)
	added := operation(t, e, sv.ID, id, "choices", "")
	n, _ := e.saves.Node(t.Context(), sv.ID, added)
	if n.Kind != save.KindChoices || n.TurnNo != before.TurnNo || n.StateHash != before.StateHash || n.MemoryHash != before.MemoryHash {
		t.Fatal("补选项改变了故事状态")
	}
	tl, _ := e.saves.Timeline(t.Context(), sv.ID, "", 1)
	if len(tl.Nodes) != 1 || len(tl.Nodes[0].Display) != 2 || tl.Nodes[0].Display[1].T != "choices" {
		t.Fatalf("补选项未附着到当前正文：%+v", tl)
	}
	msgs, _ := e.saves.LoadMessages(t.Context(), n.MessagesHash)
	if len(msgs) != 1 || len(msgs[0].ToolCalls()) != 1 || msgs[0].TextOf() != "" {
		t.Fatalf("操作旁白或拒绝的工具进入历史：%+v", msgs)
	}
	if _, err := e.mgr.Operate(t.Context(), sv.ID, save.TurnOperation{Kind: "choices", NodeID: id, ExpectedHead: added}); err == nil {
		t.Fatal("已有选项仍发起生成")
	}
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"})
	last := reqs[2].Messages[len(reqs[2].Messages)-1]
	if len(toolResults(last)) != 1 || !strings.Contains(toolResults(last)[0].Parts[0].Text, "推开门") {
		t.Fatal("选择没有回填给补充选项")
	}
	if strings.Contains(string(jsonx.MustMarshal(reqs[2].Messages)), "Let me") {
		t.Fatal("补选项旁白污染后续历史")
	}
	if _, err := e.saves.SetHead(t.Context(), sv.ID, id, save.HeadHere); err != nil {
		t.Fatal(err)
	}
	tl, _ = e.saves.Timeline(t.Context(), sv.ID, "", 1)
	if len(tl.Nodes[0].Display) != 1 {
		t.Fatal("补充选项泄漏到操作前的分支")
	}
}

type operationFailureProvider struct {
	llm.Provider
	gate       <-chan struct{}
	overflow   *bool
	fail       bool
	failReview bool
	allow      *int
	entered    chan struct{}
}

func (p operationFailureProvider) Stream(ctx context.Context, model *llm.Model, req *llm.Request) (llm.Stream, error) {
	if p.failReview && req.Purpose == "rewrite_memory" {
		return nil, &llm.APIError{Kind: llm.ErrNetwork, Message: "核对时断网"}
	}
	if req.Purpose == "rewrite" || req.Purpose == "choices" {
		if p.allow != nil && *p.allow > 0 {
			*p.allow--
			return p.Provider.Stream(ctx, model, req)
		}
		if p.entered != nil {
			p.entered <- struct{}{}
		}
		if p.overflow != nil && !*p.overflow {
			*p.overflow = true
			return nil, &llm.APIError{Kind: llm.ErrContextOverflow, Message: "超限"}
		}
		if p.gate != nil {
			select {
			case <-p.gate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if p.fail {
			return nil, &llm.APIError{Kind: llm.ErrNetwork, Message: "断网"}
		}
	}
	return p.Provider.Stream(ctx, model, req)
}

func TestRewriteFailureAfterWrites(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{true: "取消", false: "失败"}[cancel], func(t *testing.T) {
			allow := 1
			entered := make(chan struct{}, 1)
			var gate <-chan struct{}
			if cancel {
				gate = make(chan struct{})
			}
			e := setupWith(t, operationFailureProvider{Provider: fake.New(&fake.Script{Responses: []fake.Response{
				{Text: "你站在雨中。"},
				{Text: "你买下蓝伞。", ToolCalls: []fake.ToolCall{
					{Name: "update_state", Args: map[string]any{"op": "inc", "path": "gold", "value": -3}},
					{Name: "write", Args: map[string]any{"path": "/memory/purchase.md", "content": "买下蓝伞"}},
				}},
			}}), gate: gate, fail: !cancel, allow: &allow, entered: entered})
			sv, _ := operationSave(t, e)
			id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			job, err := e.mgr.Operate(t.Context(), sv.ID, save.TurnOperation{Kind: "rewrite", NodeID: id, ExpectedHead: id, Instructions: "买蓝伞"})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("没有执行写入")
			}
			if cancel {
				if err := e.mgr.Cancel(sv.ID, job); err != nil {
					t.Fatal(err)
				}
			}
			waitFor(t, sub, event.TurnFailed)
			after, line, err := e.saves.ProgressLine(t.Context(), sv.ID)
			if err != nil || after.HeadNodeID != id || len(line) != 2 {
				t.Fatal("失败留下了新版进度")
			}
			st, _ := e.saves.LoadState(t.Context(), line[1].StateHash)
			mf, _ := e.saves.LoadMemory(t.Context(), line[1].MemoryHash)
			if st["gold"] != float64(10) || len(mf.Files) != 0 {
				t.Fatal("草稿写入污染了原版")
			}
			sum, err := usage.Summarize(t.Context(), e.db.Read(), usage.Filter{SaveID: sv.ID})
			if err != nil || sum.ByKind["rewrite"].Calls != 1 {
				t.Fatal("失败操作的实际用量丢失")
			}
		})
	}
}

func TestRewriteIgnoresLaterEdits(t *testing.T) {
	e := setupWith(t, fake.New(&fake.Script{Responses: []fake.Response{{Text: "你站在雨中。"}, {Text: "你留在檐下。"}}}))
	sv, root := operationSave(t, e)
	id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	if _, _, err := e.saves.EditState(t.Context(), sv.ID, []state.Op{{Op: "set", Path: "gold", Value: 99}}); err != nil {
		t.Fatal(err)
	}
	text := "原版之后的私人笔记"
	edited, err := e.saves.EditMemory(t.Context(), sv.ID, "/memory/later.md", &text)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Operate(t.Context(), sv.ID, save.TurnOperation{Kind: "rewrite", NodeID: id, ExpectedHead: id, Instructions: "停在檐下"}); err == nil {
		t.Fatal("接受了过期进度")
	}
	next := operation(t, e, sv.ID, id, "rewrite", "停在檐下")
	n, _ := e.saves.Node(t.Context(), sv.ID, next)
	if n.StateHash != root.StateHash || n.MemoryHash != root.MemoryHash {
		t.Fatal("后续编辑泄漏进重写")
	}
	if memoryText(t, e, sv.ID, edited.ID, "/memory/later.md") != text {
		t.Fatal("原分支的编辑丢失")
	}
}

func TestRequestedChoicesCopyEditAndCompaction(t *testing.T) {
	var reqs []*llm.Request
	script := compactScript()
	script.Responses = []fake.Response{{Text: "门后传来脚步声。"}, choiceResponse(), {Text: "你推开了门。"}}
	e := setupWith(t, recordingProvider{Provider: fake.New(script), reqs: &reqs})
	sv, _ := operationSave(t, e)
	id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	added := operation(t, e, sv.ID, id, "choices", "")
	cp, err := e.saves.CopyAsNew(t.Context(), sv.ID, added)
	if err != nil {
		t.Fatal(err)
	}
	tl, err := e.saves.Timeline(t.Context(), cp.ID, "", 20)
	if err != nil || len(tl.Nodes) != 1 || tl.Nodes[0].Display[len(tl.Nodes[0].Display)-1].T != "choices" {
		t.Fatal("复制丢失补充选项")
	}
	n, err := e.saves.Rewrite(t.Context(), cp.ID, tl.Nodes[0].ID, "脚步声忽然停了。")
	if err != nil {
		t.Fatal(err)
	}
	items, _ := e.saves.LoadDisplay(t.Context(), n.DisplayHash)
	if items[len(items)-1].T != "choices" {
		t.Fatal("编辑正文丢失补充选项")
	}
	play(t, e, cp.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"})
	last := reqs[len(reqs)-1].Messages
	if len(toolResults(last[len(last)-1])) != 1 {
		t.Fatal("编辑后选项无法配对")
	}
	for range 4 {
		play(t, e, sv.ID, save.TurnInput{Kind: save.InputContinue})
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if err := e.mgr.Compact(t.Context(), sv.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.CompactionDone)
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputContinue})
	tl, err = e.saves.Timeline(t.Context(), sv.ID, "", 50)
	if err != nil || tl.Nodes[0].Display[len(tl.Nodes[0].Display)-1].T != "choices" {
		t.Fatal("整理后历史选项消失")
	}
}

func TestOperationOpeningAndNarrativeBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, body, stop string
		choices          bool
	}{
		{"静态开场", "雨落在屋檐上。", "static_opening", true},
		{"只有场外回应", "<ooc>可以调整篇幅设置。</ooc>", "end", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupWith(t, fake.New(&fake.Script{Responses: []fake.Response{choiceResponse()}}))
			sv, root := operationSave(t, e)
			n, err := e.saves.CommitTurn(t.Context(), save.TurnCommit{
				SaveID: sv.ID, Parent: root, TurnNo: 1, Input: save.TurnInput{Kind: save.InputOpening},
				Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("<input kind=\"opening\">开场</input>")}}, {Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text(tc.body)}}},
				Display:  []save.DisplayItem{{T: "text", Markup: tc.body}}, StateHash: root.StateHash, MemoryHash: root.MemoryHash,
				WorkVersionID: root.WorkVersionID, Status: save.StatusCommitted, Meta: save.NodeMeta{Opening: true, Stop: tc.stop},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.mgr.Operate(t.Context(), sv.ID, save.TurnOperation{Kind: "rewrite", NodeID: n.ID, ExpectedHead: n.ID, Instructions: "重写"}); err == nil {
				t.Fatal("不适用的回合被重写")
			}
			if tc.choices {
				operation(t, e, sv.ID, n.ID, "choices", "")
			} else if _, err := e.mgr.Operate(t.Context(), sv.ID, save.TurnOperation{Kind: "choices", NodeID: n.ID, ExpectedHead: n.ID}); err == nil {
				t.Fatal("场外回应被补选项")
			}
		})
	}
}

func TestRequestedChoicesMissingTool(t *testing.T) {
	for _, succeeds := range []bool{false, true} {
		t.Run(map[bool]string{true: "补调用成功", false: "仍无选项"}[succeeds], func(t *testing.T) {
			last := fake.Response{Text: "好的，我会提供选项。"}
			if succeeds {
				last = choiceResponse()
			}
			var reqs []*llm.Request
			e := setupWith(t, recordingProvider{Provider: fake.New(&fake.Script{Responses: []fake.Response{{Text: "门后传来脚步声。"}, {Text: "Let me think."}, last}}), reqs: &reqs})
			sv, _ := operationSave(t, e)
			id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			if _, err := e.mgr.Operate(t.Context(), sv.ID, save.TurnOperation{Kind: "choices", NodeID: id, ExpectedHead: id}); err != nil {
				t.Fatal(err)
			}
			terminal := event.TurnFailed
			if succeeds {
				terminal = event.TurnCommitted
			}
			waitFor(t, sub, terminal)
			tl, err := e.saves.Timeline(t.Context(), sv.ID, "", 20)
			if err != nil || len(tl.Nodes) != 1 || strings.Contains(string(jsonx.MustMarshal(tl)), "Let me") || strings.Contains(string(jsonx.MustMarshal(tl)), "好的") {
				t.Fatal("补调用旁白进入正文")
			}
			if !succeeds && tl.HeadID != id {
				t.Fatal("无选项时仍修改进度")
			}
			if len(reqs) != 3 || strings.Contains(reqs[2].Messages[len(reqs[2].Messages)-1].TextOf(), "2–4") {
				t.Fatal("主动补选项没有沿用自身任务提示")
			}
		})
	}
}

func TestRewriteMemoryReview(t *testing.T) {
	var reqs []*llm.Request
	rewrite := choiceResponse()
	rewrite.Text = "你没有购物，仍站在门口。"
	rewrite.ToolCalls = append([]fake.ToolCall{{Name: "write", Args: map[string]any{"path": "/memory/scene.md", "content": "你买了红灯，并偏好含蓄描写。"}}}, rewrite.ToolCalls...)
	e := setupWith(t, recordingProvider{Provider: fake.New(&fake.Script{Responses: []fake.Response{
		{Text: "你买了红灯。"}, rewrite,
		{Text: "核对中。", ToolCalls: []fake.ToolCall{
			{Name: "update_state", Args: map[string]any{"op": "set", "path": "gold", "value": 1}},
			{Name: "write", Args: map[string]any{"path": "/memory/scene.md", "content": "你仍站在门口。"}},
		}},
		{ToolCalls: []fake.ToolCall{{Name: "get_state", Args: map[string]any{}}}},
		{Text: "核对完成。"}, {Text: "门开了。"},
	}}), reqs: &reqs})
	sv, _ := operationSave(t, e)
	old := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	id := operation(t, e, sv.ID, old, "rewrite", "取消购物，保密写作要求XYZ")
	n, _ := e.saves.Node(t.Context(), sv.ID, id)
	if memoryText(t, e, sv.ID, id, "/memory/scene.md") != "你仍站在门口。" {
		t.Fatal("记忆核对未落盘")
	}
	st, _ := e.saves.LoadState(t.Context(), n.StateHash)
	if st["gold"] != float64(10) {
		t.Fatal("记忆核对修改了状态")
	}
	msgs, _ := e.saves.LoadMessages(t.Context(), n.MessagesHash)
	if strings.Contains(string(jsonx.MustMarshal(msgs)), "核对完成") || strings.Contains(timelineTurns(t, e, sv.ID), "核对") {
		t.Fatal("核对旁白进入故事")
	}
	review := reqs[2]
	if review.Purpose != "rewrite_memory" || strings.Contains(string(jsonx.MustMarshal(review.Messages)), "保密写作要求XYZ") || strings.Contains(string(jsonx.MustMarshal(review.Messages)), "rewrite_request") {
		t.Fatal("核对复用了重写控制上下文")
	}
	if len(msgs[len(msgs)-1].ToolCalls()) != 1 || msgs[len(msgs)-1].ToolCalls()[0].Name != "offer_choices" {
		t.Fatal("核对打乱待答选项")
	}
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"})
	last := reqs[len(reqs)-1].Messages
	if len(toolResults(last[len(last)-1])) != 1 {
		t.Fatal("核对后选项不能回填")
	}
}

func TestRewriteMemoryReviewFailure(t *testing.T) {
	r := choiceResponse()
	r.Text = "你买了蓝伞。"
	r.ToolCalls = append([]fake.ToolCall{{Name: "write", Args: map[string]any{"path": "/memory/scene.md", "content": "你买了蓝伞。"}}}, r.ToolCalls...)
	e := setupWith(t, operationFailureProvider{Provider: fake.New(&fake.Script{Responses: []fake.Response{{Text: "你站在屋檐下。"}, r}}), failReview: true})
	sv, _ := operationSave(t, e)
	id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if _, err := e.mgr.Operate(t.Context(), sv.ID, save.TurnOperation{Kind: "rewrite", NodeID: id, ExpectedHead: id, Instructions: "买蓝伞"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnFailed)
	after, line, err := e.saves.ProgressLine(t.Context(), sv.ID)
	if err != nil || after.HeadNodeID != id || len(line) != 2 {
		t.Fatal("核对失败采用了草稿")
	}
}

func TestOperationFailureAndCancel(t *testing.T) {
	for _, kind := range []string{"rewrite", "choices"} {
		for _, cancel := range []bool{false, true} {
			t.Run(kind+map[bool]string{true: "取消", false: "失败"}[cancel], func(t *testing.T) {
				var gate <-chan struct{}
				if cancel {
					gate = make(chan struct{})
				}
				e := setupWith(t, operationFailureProvider{Provider: fake.New(&fake.Script{Responses: []fake.Response{{Text: "雨落在石阶上。"}}}), gate: gate, fail: !cancel})
				sv, _ := operationSave(t, e)
				id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
				sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
				defer sub.Close()
				job, err := e.mgr.Operate(t.Context(), sv.ID, save.TurnOperation{Kind: kind, NodeID: id, ExpectedHead: id, Instructions: "放慢节奏"})
				if err != nil {
					t.Fatal(err)
				}
				if cancel {
					waitFor(t, sub, event.TurnStarted)
					if err := e.mgr.Cancel(sv.ID, job); err != nil {
						t.Fatal(err)
					}
				}
				waitFor(t, sub, event.TurnFailed)
				after, line, _ := e.saves.ProgressLine(t.Context(), sv.ID)
				if after.HeadNodeID != id || len(line) != 2 {
					t.Fatal("失败操作改变了进度或留下节点")
				}
			})
		}
	}
}

func TestRewriteStagedCompaction(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{true: "失败回滚", false: "整体提交"}[fail], func(t *testing.T) {
			hit := false
			e := setupWith(t, operationFailureProvider{Provider: fake.New(compactScript()), overflow: &hit, fail: fail})
			sv, _ := operationSave(t, e)
			id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
			for range 5 {
				id = play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "继续观察雨夜"})
			}
			before, line, _ := e.saves.ProgressLine(t.Context(), sv.ID)
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			if _, err := e.mgr.Operate(t.Context(), sv.ID, save.TurnOperation{Kind: "rewrite", NodeID: id, ExpectedHead: before.HeadNodeID, Instructions: "让气氛更安静"}); err != nil {
				t.Fatal(err)
			}
			waitFor(t, sub, event.CompactionDone)
			if fail {
				waitFor(t, sub, event.TurnFailed)
				after, got, _ := e.saves.ProgressLine(t.Context(), sv.ID)
				if after.HeadNodeID != id || len(got) != len(line) {
					t.Fatal("暂存整理改变了失败后的分支")
				}
			} else {
				waitFor(t, sub, event.TurnCommitted)
				_, got, _ := e.saves.ProgressLine(t.Context(), sv.ID)
				if got[len(got)-2].Kind != save.KindCompaction {
					t.Fatal("整理与重写未一起提交")
				}
				tl, _ := e.saves.Timeline(t.Context(), sv.ID, "", 1)
				if tl.Nodes[0].Versions.Count != 2 {
					t.Fatal("紧急整理拆散了回合版本")
				}
			}
			sum, err := usage.Summarize(t.Context(), e.db.Read(), usage.Filter{SaveID: sv.ID})
			if err != nil || sum.ByKind["compact"].Calls == 0 {
				t.Fatal("整理用量未记录")
			}
		})
	}
}
