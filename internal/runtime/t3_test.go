package runtime_test

// 测试作品 T3（《雾港来信》，玩法型）的回归用例（M2-14）：用假模型走一遍作品用到的声明式原语——
// 开局表单、静态开场与选项、查阅设定、状态更新与面板（含 render 片段与抽屉）、掷骰、带样式与命令的选项、
// 带参数的命令、输入模式、常驻图片与看图、作品主题与样式标签的深色颜色。

import (
	"context"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/save"
)

func TestT3Primitives(t *testing.T) {
	script := &fake.Script{Responses: []fake.Response{
		// 回合 2（选择"拆开那封信"）：查阅主线与地图 → 写正文、更新状态、掷骰 → 带样式与命令的选项
		{DelayMS: 1, ToolCalls: []fake.ToolCall{
			{Name: "read", Args: map[string]any{"path": "/work/lore/outline/02-storm.md"}},
			{Name: "read", Args: map[string]any{"path": "/work/assets/map/harbor.png"}},
		}},
		{Text: "信纸上的字迹很淡。\n\n<gold>信是在明天寄出的。</gold>", ChunkRunes: 50, DelayMS: 1, ToolCalls: []fake.ToolCall{
			{Name: "update_state", Args: map[string]any{"op": "append", "path": "sealed", "value": "信是在明天寄出的。"}},
			{Name: "update_state", Args: map[string]any{"op": "set", "path": "trust.周看守", "value": -2}},
			{Name: "roll_dice", Args: map[string]any{"expr": "1d20", "reason": "辨认褪色的字迹"}},
			{Name: "offer_choices", Args: map[string]any{"choices": []any{
				map[string]any{"id": "a", "label": "推论：信是老陈写的", "style": "hypo"},
				map[string]any{"id": "b", "label": "跟着林舟", "command": "/follow 林舟"},
			}}},
		}},
		// 回合 3（/follow 林舟）
		{Text: "镜头转向林舟。", ChunkRunes: 50, DelayMS: 1, ToolCalls: []fake.ToolCall{
			{Name: "update_state", Args: map[string]any{"op": "set", "path": "follow", "value": "林舟"}},
		}},
		{Text: "他把钥匙攥在手心。", ChunkRunes: 50, DelayMS: 1},
		// 回合 4（推论模式）
		{Text: "<gold>老陈在午夜前从未离开过柜台。</gold>推论不成立。", ChunkRunes: 50, DelayMS: 1},
	}}
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(script), reqs: &reqs})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor",
		Form: map[string]any{"player.name": "阿遥", "player.trait": "记忆"}, Settings: save.Settings{Pace: "medium", Reasoning: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	run := func(in save.TurnInput) {
		t.Helper()
		if _, err := e.mgr.Submit(ctx, sv.ID, in); err != nil {
			t.Fatal(err)
		}
		waitFor(t, sub, event.TurnCommitted)
	}

	// 常驻资料：执笔指令以开局表单渲染、常驻图片（信件扫描件）随前缀发送
	run(save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"})
	first := reqs[0]
	if !strings.Contains(first.System[1].Text, "玩家扮演 阿遥") {
		t.Fatal("执笔指令应以开局表单的值渲染")
	}
	head := first.Messages[0]
	var pinned int
	for _, p := range head.Parts {
		if p.Kind == llm.PartImage {
			pinned++
		}
	}
	if pinned != 1 || !strings.Contains(head.Parts[1].Text, `<image path="/work/assets/letters/letter.webp">`) {
		t.Fatalf("常驻图片应进入 L3：%d 张", pinned)
	}
	if strings.Contains(first.System[1].Text, "cover.webp") || strings.Contains(first.System[1].Text, "fog.webp") {
		t.Fatal("封面与主题背景不应出现在 AI 的素材索引中")
	}

	// 查阅设定（附关联图片的文档、看图）
	results := toolResults(reqs[1].Messages[len(reqs[1].Messages)-1])
	if len(results) != 2 || !strings.Contains(results[0].Parts[0].Text, "[[zhou]](/work/lore/characters/zhou.md)") ||
		len(results[1].Parts) != 2 || results[1].Parts[1].Kind != llm.PartImage {
		t.Fatalf("read 设定与看图：%+v", results)
	}

	tl, _ := e.saves.Timeline(ctx, sv.ID, "", 10)
	turn := tl.Nodes[len(tl.Nodes)-1]
	kinds := []string{}
	for _, it := range turn.Display {
		kinds = append(kinds, it.T)
	}
	if strings.Join(kinds, ",") != "input,tools,text,dice,choices" {
		t.Fatalf("显示项：%v", kinds)
	}
	choices := turn.Display[len(turn.Display)-1].Items
	if choices[0].Style != "hypo" || choices[1].Command != "/follow 林舟" {
		t.Fatalf("选项的样式与命令：%+v", choices)
	}

	// 命令输入附带的选项 ID 同样必须存在于当前选项中。
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputCommand, ChoiceID: "missing",
		Command: &save.CommandCall{Name: "follow", Args: map[string]string{"who": "林舟"}}}); err != nil {
		t.Fatal(err)
	}
	failed := waitFor(t, sub, event.TurnFailed).Data.(event.TurnFailedData)
	if !strings.Contains(failed.Message, "当前没有可选的选项") || len(reqs) != 2 {
		t.Fatalf("无效选项应在调用模型前拒绝：%+v，调用次数 %d", failed, len(reqs))
	}

	// 选项附带的命令保留选项 ID；选项文字由服务端补全，命令参数由前端解析。
	run(save.TurnInput{Kind: save.InputCommand, ChoiceID: "b", ChoiceLabel: "客户端文字", Command: &save.CommandCall{Name: "follow", Args: map[string]string{"who": "林舟"}}})
	cmdMsg := reqs[2].Messages[len(reqs[2].Messages)-1]
	answers := toolResults(cmdMsg)
	if len(answers) != 1 || answers[0].CallID != "call_1_4" || answers[0].Parts[0].Text != "玩家选择：B「跟着林舟」" {
		t.Fatalf("命令选项应回填实际选择：%+v", answers)
	}
	if !strings.Contains(lastText(cmdMsg), "叙述此后跟随 林舟") || !strings.Contains(lastText(cmdMsg), `<input kind="command">`) {
		t.Fatalf("命令应展开为模板文字：%s", lastText(cmdMsg))
	}
	_, line, err := e.saves.ProgressLine(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := line[len(line)-1].Input
	if input == nil || input.ChoiceID != "b" || input.ChoiceLabel != "跟着林舟" || input.Command == nil || input.Command.Name != "follow" {
		t.Fatalf("存档应保留选择与命令，供重新生成使用：%+v", input)
	}

	// 输入模式（推论）
	run(save.TurnInput{Kind: save.InputFree, Mode: "推论", Text: "信是老陈写的"})
	modeMsg := reqs[4].Messages[len(reqs[4].Messages)-1]
	if !strings.Contains(lastText(modeMsg), `mode="推论"`) || !strings.Contains(lastText(modeMsg), "<hypo>信是老陈写的</hypo>") {
		t.Fatalf("输入模式应以模式命令包装：%s", lastText(modeMsg))
	}

	// 状态与面板：封印、信任（新增人物）、跟随；人物抽屉由 render 片段渲染
	st, _ := e.mgr.State(ctx, sv.ID)
	if st.State["follow"] != "林舟" || len(st.State["sealed"].([]any)) != 1 || st.State["trust"].(map[string]any)["周看守"] != float64(-2) {
		t.Fatalf("状态：%+v", st.State)
	}
	panels, err := e.mgr.Panels(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	html := map[string]string{}
	for _, p := range panels {
		html[p.ID] = p.HTML
	}
	if !strings.Contains(html["sealed"], "信是在明天寄出的。") {
		t.Fatalf("封印面板：%s", html["sealed"])
	}
	people := html["people"]
	if !strings.Contains(people, `<ui-image src="lin"`) || !strings.Contains(people, "叙述正跟随林舟") ||
		!strings.Contains(people, `<ui-button command="/follow" args="周看守">`) || strings.Contains(people, "面板渲染失败") {
		t.Fatalf("人物抽屉：%s", people)
	}

	// 呈现信息：主题、深色颜色、封面
	p, err := e.mgr.Presentation(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.UI.Theme.CSS, ".dark .work-root { --paper: #0b0e10") || !p.UI.Theme.Reading.Backdrop {
		t.Fatalf("主题：%+v", p.UI.Theme)
	}
	for _, tag := range p.UI.Tags {
		if tag.Style.Color != "" && tag.Style.DarkColor == "" {
			t.Fatalf("样式标签 %s 缺少深色颜色", tag.Name)
		}
	}
}

func toolResults(msg llm.Message) []*llm.ToolResult {
	var out []*llm.ToolResult
	for _, p := range msg.Parts {
		if p.Kind == llm.PartToolResult {
			out = append(out, p.ToolResult)
		}
	}
	return out
}

func lastText(msg llm.Message) string {
	for i := len(msg.Parts) - 1; i >= 0; i-- {
		if msg.Parts[i].Kind == llm.PartText {
			return msg.Parts[i].Text
		}
	}
	return ""
}
