package runtime_test

import (
	"context"
	"errors"
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

// gatedProvider 在 open 关闭之前阻塞 Stream：测试据此确定"回合正在进行"，不依赖时序。
type gatedProvider struct {
	llm.Provider
	open chan struct{}
}

func (g gatedProvider) Stream(ctx context.Context, m *llm.Model, req *llm.Request) (llm.Stream, error) {
	select {
	case <-g.open:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return g.Provider.Stream(ctx, m, req)
}

// env 是测试用的最小应用装配：依赖关系与 app.wire 相同，模型换成假模型。
type env struct {
	db    *store.DB
	blobs *store.Blobs
	saves *save.Service
	mgr   *runtime.Manager
	bus   *event.Bus
}

func setup(t *testing.T, script *fake.Script) *env {
	t.Helper()
	return setupWith(t, fake.New(script))
}

func setupWith(t *testing.T, provider llm.Provider) *env {
	t.Helper()
	e, err := openEnv(context.Background(), t.TempDir(), provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })
	return e
}

// openEnv 在数据目录 dir 上装配服务（目录中已有数据时沿用）。不依赖 testing.T：杀进程测试的子进程也用它。
func openEnv(ctx context.Context, dir string, provider llm.Provider) (*env, error) {
	db, err := store.Open(ctx, filepath.Join(dir, "yanyi.db"))
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		_ = db.Close()
		return nil, err
	}
	blobs := store.NewBlobs(filepath.Join(dir, "blobs"))
	tm := tmpl.New()
	works := work.NewService(db, blobs)
	works.EnsureBuiltins(ctx, examples.Works())
	saves := save.NewService(db, blobs, works, tm)
	bus := event.NewBus(0)
	mgr := runtime.NewManager(runtime.Deps{
		Saves: saves, Works: works, Models: fakeModels{provider}, Bus: bus,
		Builder: contextx.NewBuilder(tm), Tools: tools.NewRegistry(), Tmpl: tm,
	})
	return &env{db: db, blobs: blobs, saves: saves, mgr: mgr, bus: bus}, nil
}

// close 等运行中的回合结束（已生成的内容以 interrupted 提交），再关闭数据库。
func (e *env) close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return errors.Join(e.mgr.Shutdown(ctx), e.db.Close())
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
	gate := make(chan struct{})
	e := setupWith(t, gatedProvider{Provider: fake.New(quickScript()), open: gate})
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
	// 模型调用被闸住，回合一定还在进行：再次提交必须得到冲突。
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "再来"}); err == nil {
		t.Error("回合进行中再次提交应返回冲突（单一写者）")
	}
	close(gate)
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

// 截断续写：显示上去掉续写开头与上一段末尾的重复（9.4），LLM 消息保留模型的原始输出。
func TestContinuationDedup(t *testing.T) {
	script := &fake.Script{Responses: []fake.Response{
		{Text: "雾越来越浓。林舟把帽子挂上衣架，水珠沿着帽檐往下落。", Stop: "max_tokens", ChunkRunes: 3},
		{Text: "水珠沿着帽檐往下落。他转过身来。", ChunkRunes: 3},
	}}
	e := setup(t, script)
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.first-story", Settings: save.Settings{Pace: "short"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputOpening}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)

	tl, err := e.saves.Timeline(ctx, sv.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	var shown strings.Builder
	for _, it := range tl.Nodes[len(tl.Nodes)-1].Display {
		shown.WriteString(it.Markup)
	}
	if want := "雾越来越浓。林舟把帽子挂上衣架，水珠沿着帽檐往下落。他转过身来。"; shown.String() != want {
		t.Fatalf("显示的正文应去掉重复：%q", shown.String())
	}

	_, line, _ := e.saves.ProgressLine(ctx, sv.ID)
	msgs, _ := e.saves.LoadMessages(ctx, line[len(line)-1].MessagesHash)
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant || last.TextOf() != "水珠沿着帽檐往下落。他转过身来。" {
		t.Fatalf("LLM 消息应保留续写的原始输出：%+v", last)
	}
}

// recordingProvider 记录每次请求（只读，不修改）。
type recordingProvider struct {
	llm.Provider
	reqs *[]*llm.Request
}

func (r recordingProvider) Stream(ctx context.Context, m *llm.Model, req *llm.Request) (llm.Stream, error) {
	cp := *req
	cp.Messages = append([]llm.Message(nil), req.Messages...)
	*r.reqs = append(*r.reqs, &cp)
	return r.Provider.Stream(ctx, m, req)
}

// read 与 list：读取设定文档（补全 [[id]] 链接，D-35）、列出记忆目录，结果回填后继续写正文。
func TestReadAndListTools(t *testing.T) {
	script := &fake.Script{Responses: []fake.Response{
		{DelayMS: 1, ToolCalls: []fake.ToolCall{
			{Name: "read", Args: map[string]any{"path": "/work/lore/characters/lin.md"}},
			{Name: "list", Args: map[string]any{"path": "/memory"}},
			{Name: "read", Args: map[string]any{"path": "/work/lore/characters/lin"}},
		}},
		{Text: "林舟想起老陈。", ChunkRunes: 50, DelayMS: 1},
	}}
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(script), reqs: &reqs})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.misty-harbor", Settings: save.Settings{Pace: "medium", Reasoning: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "回忆林舟"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)

	if len(reqs) != 2 {
		t.Fatalf("应调用两次模型，实际 %d 次", len(reqs))
	}
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	var results []string
	for _, p := range last.Parts {
		if p.Kind == llm.PartToolResult {
			results = append(results, p.ToolResult.Parts[0].Text)
		}
	}
	if len(results) != 3 {
		t.Fatalf("工具结果 = %v", results)
	}
	if !strings.HasPrefix(results[0], "# read /work/lore/characters/lin.md（第 1–") || !strings.Contains(results[0], "[[old-chen]](/work/lore/characters/old-chen.md)") {
		t.Fatalf("read 结果应带行范围并补全链接：\n%s", results[0])
	}
	if !strings.HasPrefix(results[1], "# list /memory（共 1 项）\n/memory/index.md") {
		t.Fatalf("list 结果：\n%s", results[1])
	}
	if !strings.HasPrefix(results[2], "未执行：/work/lore/characters/lin 不存在。相近的路径有 /work/lore/characters/lin.md") {
		t.Fatalf("漏写扩展名应给出相近路径：\n%s", results[2])
	}
}
