package runtime_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/settings"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"

	"github.com/wahhheee/aint/internal/save"
)

func TestRetryInterruptedChoice(t *testing.T) {
	e := setup(t, &fake.Script{Responses: []fake.Response{
		{Text: "正文已生成", ChunkRunes: 100, DelayMS: 1, ToolCalls: []fake.ToolCall{{Name: "read", Args: map[string]any{"path": "/memory/index.md"}}}},
		{Error: string(llm.ErrNetwork)},
		{Text: "重试成功", ChunkRunes: 100, DelayMS: 1},
	}})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor", Form: map[string]any{"player.name": "阿遥"}, Settings: save.Settings{NarratorModel: "fake", Pace: "short", Reasoning: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	tl, err := e.saves.Timeline(ctx, sv.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	choice := ""
	for _, it := range tl.Nodes[0].Display {
		if it.T == "choices" {
			choice = it.Items[0].ID
		}
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: choice}); err != nil {
		t.Fatal(err)
	}
	failed := waitFor(t, sub, event.TurnFailed)
	data := failed.Data.(event.TurnFailedData)
	if data.NodeID == "" || data.ParentID != sv.HeadNodeID {
		t.Fatalf("缺少重试来源：%+v", data)
	}
	if _, err := e.mgr.Regenerate(ctx, sv.ID, data.NodeID, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	tl, err = e.saves.Timeline(ctx, sv.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(tl.Nodes) != 2 || tl.Nodes[1].Versions.Count != 2 {
		t.Fatalf("重试应产生同父节点的版本：%+v", tl)
	}
}

func TestSaveSettingsBusyAndBudget(t *testing.T) {
	gate := make(chan struct{})
	p := &purposeProvider{Provider: gatedProvider{Provider: fake.New(quickScript()), open: gate}}
	e := setupWith(t, p)
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.first-story", Settings: save.Settings{NarratorModel: "fake", Pace: "short", Reasoning: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.PatchSettings(ctx, sv.ID, []byte(`{"reasoning":"high","max_calls":2,"max_output":1024}`)); err != nil {
		t.Fatal(err)
	}
	current, err := e.saves.Get(ctx, sv.ID)
	if err != nil || current.Settings.Reasoning != "high" || current.Settings.MaxOutput != 1024 {
		t.Fatalf("设置未保存：%+v %v", current, err)
	}
	if err := e.mgr.PatchSettings(ctx, sv.ID, []byte(`{"reasoning":"invalid"}`)); err == nil {
		t.Fatal("非法设置不能保存")
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputContinue}); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.PatchSettings(ctx, sv.ID, []byte(`{"reasoning":"low"}`)); err == nil {
		t.Error("运行中不能修改设置")
	}
	close(gate)
	waitFor(t, sub, event.TurnCommitted)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) == 0 || p.requests[0].Reasoning != llm.ReasoningHigh || p.requests[0].MaxOutput != 1024 {
		t.Fatalf("请求未使用存档参数：%+v", p.requests)
	}
}

// purposeProvider 为后台任务分配独立脚本，并采集请求验证隔离与权限。
type purposeProvider struct {
	llm.Provider
	helper   llm.Provider
	title    llm.Provider
	mu       sync.Mutex
	requests []*llm.Request
}

func (p *purposeProvider) Stream(ctx context.Context, m *llm.Model, r *llm.Request) (llm.Stream, error) {
	p.mu.Lock()
	p.requests = append(p.requests, r)
	p.mu.Unlock()
	switch r.Purpose {
	case "helper":
		return p.helper.Stream(ctx, m, r)
	case "title":
		return p.title.Stream(ctx, m, r)
	}
	return p.Provider.Stream(ctx, m, r)
}

func TestHelperPermissionsAndUsage(t *testing.T) {
	for _, kind := range []string{"research", "curate"} {
		t.Run(kind, func(t *testing.T) {
			p := &purposeProvider{
				Provider: fake.New(&fake.Script{Responses: []fake.Response{
					{ToolCalls: []fake.ToolCall{{Name: "delegate", Args: map[string]any{"kind": kind, "task": "查阅记忆"}}}, DelayMS: 1},
					{Text: "故事继续", ChunkRunes: 100, DelayMS: 1},
				}}),
				helper: fake.New(&fake.Script{Responses: []fake.Response{
					{ToolCalls: []fake.ToolCall{
						{Name: "write", Args: map[string]any{"path": "/memory/helper.md", "content": "助手笔记"}},
						{Name: "update_state", Args: map[string]any{"op": "set", "path": "forbidden", "value": "bad"}},
					}, DelayMS: 1},
					{Text: "简报：/memory/index.md 是记忆索引", ChunkRunes: 100, DelayMS: 1},
				}}),
			}
			g := settings.Defaults()
			g.Models.Light = "light"
			e, err := openEnvWith(context.Background(), t.TempDir(), fakeModels{p}, func(d *runtime.Deps) { d.Settings = staticSettings{g} })
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = e.close() })
			ctx := context.Background()
			sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.first-story", Settings: save.Settings{NarratorModel: "fake", Pace: "short", Reasoning: "low", Helpers: true}})
			if err != nil {
				t.Fatal(err)
			}
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputOpening}); err != nil {
				t.Fatal(err)
			}
			waitFor(t, sub, event.TurnCommitted)
			sv, err = e.saves.Get(ctx, sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			node, err := e.saves.Node(ctx, sv.ID, sv.HeadNodeID)
			if err != nil {
				t.Fatal(err)
			}
			mem, err := e.saves.LoadMemory(ctx, node.MemoryHash)
			if err != nil {
				t.Fatal(err)
			}
			_, written := mem.Files["/memory/helper.md"]
			if written != (kind == "curate") {
				t.Fatalf("助手写权限不符：%s %v", kind, written)
			}
			var count int
			if err := e.db.Read().QueryRowContext(ctx, `SELECT count(*) FROM usage_log WHERE save_id = ? AND call_kind = 'helper'`, sv.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("助手用量次数：%d", count)
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			for _, req := range p.requests {
				if req.Purpose == "helper" {
					for _, tool := range req.Tools {
						if tool.Name == "update_state" || tool.Name == "delegate" || tool.Name == "offer_choices" {
							t.Fatalf("助手越权工具：%s", tool.Name)
						}
					}
					if strings.Contains(req.Messages[0].TextOf(), "<epoch") {
						t.Fatal("助手不应看到主笔上下文")
					}
					break
				}
			}
		})
	}
}

func TestTitleDefaultOffAndManual(t *testing.T) {
	ctx := context.Background()
	p := &purposeProvider{Provider: fake.New(quickScript()), title: fake.New(&fake.Script{Responses: []fake.Response{{Text: "雾港的来信", ChunkRunes: 100, DelayMS: 1}}})}
	g := settings.Defaults()
	g.Models.Light = "light"
	e, err := openEnvWith(ctx, t.TempDir(), fakeModels{p}, func(d *runtime.Deps) { d.Settings = staticSettings{g} })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor", Form: map[string]any{"player.name": "阿遥"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.GenerateTitle(ctx, sv.ID, true); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	n := len(p.requests)
	p.mu.Unlock()
	if n != 0 {
		t.Fatal("默认关闭不应调用")
	}
	sub, _, _ := e.bus.Subscribe(event.GlobalTopic, 0)
	defer sub.Close()
	if err := e.mgr.GenerateTitle(ctx, sv.ID, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.SaveTitled)
	named, err := e.saves.Get(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if named.Title != "雾港的来信" || named.TitleSource != "generated" {
		t.Fatalf("标题不符：%+v", named)
	}
	current, err := e.saves.TitleCurrent(ctx, sv.ID, sv.HeadNodeID)
	if err != nil || !current {
		t.Fatalf("标题没有关联进度：%v %v", current, err)
	}
}

func TestAutomaticTitleDeduplicatesAfterRestart(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	p := &purposeProvider{Provider: fake.New(quickScript()), title: fake.New(&fake.Script{Responses: []fake.Response{{Text: "码头的灯", ChunkRunes: 100, DelayMS: 1}}})}
	g := settings.Defaults()
	g.Models.Light = "light"
	g.Play.TitleOnExit = "auto"
	configure := func(d *runtime.Deps) { d.Settings = staticSettings{g} }
	e, err := openEnvWith(ctx, dir, fakeModels{p}, configure)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor", Form: map[string]any{"player.name": "阿遥"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.GlobalTopic, 0)
	if err := e.mgr.GenerateTitle(ctx, sv.ID, true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.SaveTitled)
	sub.Close()
	if err := e.close(); err != nil {
		t.Fatal(err)
	}
	e2, err := openEnvWith(ctx, dir, fakeModels{p}, configure)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e2.close() })
	if err := e2.mgr.GenerateTitle(ctx, sv.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := e2.close(); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) != 1 {
		t.Fatalf("同一进度重复计费：%d", len(p.requests))
	}
}
