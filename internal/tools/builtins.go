package tools

import (
	"context"
	"strings"

	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/work"
)

// builtins 返回内置工具（《02-技术方案》10.2），顺序固定。
func builtins() []*Def {
	return []*Def{
		{Name: "read", Kind: KindRead, Display: DisplaySubtle, Spec: specRead, Handle: handleRead},
		{Name: "list", Kind: KindRead, Display: DisplaySubtle, Spec: specList, Handle: handleList},
		{Name: "search", Kind: KindRead, Display: DisplaySubtle, Spec: specSearch, Handle: handleSearch},
		{Name: "write", Kind: KindWrite, Display: DisplayHidden, Spec: specWrite, Handle: handleWrite},
		{Name: "edit", Kind: KindWrite, Display: DisplayHidden, Spec: specEdit, Handle: handleEdit},
		{Name: "remove", Kind: KindWrite, Display: DisplayHidden, Spec: specRemove, Handle: handleRemove},
		{Name: "get_state", Kind: KindRead, Display: DisplayHidden, When: prompts.Requires(prompts.FState),
			Spec: specGetState, Handle: handleGetState},
		{Name: "update_state", Kind: KindWrite, Display: DisplayHidden, When: prompts.Requires(prompts.FState),
			Spec: specUpdateState, Handle: handleUpdateState},
		{Name: "offer_choices", Kind: KindTerminal, Display: DisplayCustom, Spec: specOfferChoices, Handle: handleOfferChoices},
		// roll_dice 归入写类（串行）：它在正文流中追加骰子卡片，按调用顺序执行，卡片顺序与调用顺序一致
		{Name: "roll_dice", Kind: KindWrite, Display: DisplayCard, When: prompts.Requires(prompts.FDice),
			Spec: specRollDice, Handle: handleRollDice},
		{Name: "delegate", Kind: KindDelegate, Display: DisplaySubtle, When: prompts.Requires(prompts.FHelpers),
			Spec: specDelegate, Handle: delegate},
	}
}

// notYet 是尚未实现的工具：返回事实性说明，模型可以据此继续写作。

// ---------------------------------------------------------------------------
// JSON Schema 的小型构造函数（保持定义紧凑；序列化经 jsonx 确定性输出）

func object(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

// ---------------------------------------------------------------------------
// 资料与记忆（VFS）

func specRead(fs prompts.FeatureSet, _ *work.Model) (string, map[string]any) {
	desc := "需要资料细节时，读取 path 指向的作品资料、记忆或原文（如 /transcript/0042）；长文本可用 offset、limit 分段按行读取。"
	if fs.Has(prompts.FVision) {
		desc += "读取图片即可看图。"
	}
	return desc, object([]string{"path"}, map[string]any{
		"path":   str("文件路径"),
		"offset": integer("起始行（1 起），默认 1"),
		"limit":  integer("行数，默认 400，上限 1000"),
	})
}

func specList(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "不清楚有哪些文件时，列出 path 目录的内容，每项附一行说明；depth 控制展开层数。", object([]string{"path"}, map[string]any{
		"path":  str("目录路径，如 /memory"),
		"depth": integer("展开层数 1–3，默认 1"),
	})
}

func specSearch(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "用 query 在记忆、作品资料与已发生的原文中检索文字，结果附出处；可用 path 限定范围。", object([]string{"query"}, map[string]any{
		"query": str("要查找的文字"),
		"path":  str("限定路径前缀，默认 /"),
		"regex": boolean("按正则匹配，默认 false"),
		"limit": integer("结果条数，默认 20，上限 50"),
	})
}

func specWrite(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "在 /memory 下新建或覆盖 path 指向的记忆文件（.md / .txt / .yaml）；content 是完整内容，局部修改可用 edit。", object([]string{"path", "content"}, map[string]any{
		"path":    str("文件路径，如 /memory/characters/lin.md"),
		"content": str("完整内容"),
	})
}

func specEdit(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "局部修改记忆时，把 path 文件中唯一匹配的 old_text 替换为 new_text；需要替换全部匹配时设置 replace_all。", object([]string{"path", "old_text", "new_text"}, map[string]any{
		"path":        str("文件路径"),
		"old_text":    str("原文字（需唯一匹配，除非 replace_all）"),
		"new_text":    str("新文字"),
		"replace_all": boolean("替换全部匹配，默认 false"),
	})
}

func specRemove(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "删除 path 指向的记忆文件，用于移除已无用的笔记。", object([]string{"path"}, map[string]any{"path": str("文件路径")})
}

// ---------------------------------------------------------------------------
// 状态

func specGetState(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "需要确认当前状态时读取 paths 指定的字段；省略 paths 则读取全部字段。", object(nil, map[string]any{
		"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "字段路径；缺省为全部字段"},
	})
}

// specUpdateState 每次调用改一项。参数刻意不用"操作数组"：实测 Claude Sonnet 5.5 会把对象数组参数写成
// （内部引号未转义的）JSON 字符串，而单个操作的参数总是正确；多项变化在同一次回复中多次调用即可。
func specUpdateState(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "故事中相应事实变化时，用 op、path、value 更新一项作品状态，并以返回结果确认是否生效；多项变化可在同一次回复中多次调用。", object([]string{"op", "path"}, map[string]any{
		"op": map[string]any{"type": "string", "enum": []string{"set", "inc", "append", "remove", "delete"},
			"description": "set 设值；inc 数字加减；append / remove 列表追加 / 按值移除；delete 删除映射的键，其他字段恢复默认值"},
		"path":   str("字段路径，如 day、trust.林舟"),
		"value":  map[string]any{"description": "新值（set）、增量（inc）或列表元素（append / remove）"},
		"reason": str("简短说明（只用于调试记录）"),
	})
}

// ---------------------------------------------------------------------------
// 交还决定权

func specOfferChoices(fs prompts.FeatureSet, m *work.Model) (string, map[string]any) {
	item := map[string]any{
		"id":     str("短 ID，如 a、b、c"),
		"label":  str("选项文字"),
		"detail": str("补充说明（可选）"),
	}
	// style 只取作品样式标签，平台不内置选项样式（D-27）
	if fs.Has(prompts.FWorkTags) {
		var styles []string
		for _, t := range m.Tags {
			styles = append(styles, t.Name)
		}
		item["style"] = map[string]any{"type": "string", "enum": styles, "description": "选项样式（可选）"}
	}
	if fs.Has(prompts.FCommands) {
		item["command"] = str(`选中即执行的命令，如 "/follow 林舟"（可选）`)
	}
	props := map[string]any{
		"prompt":  str("一句引导语（可选）"),
		"choices": map[string]any{"type": "array", "items": object([]string{"id", "label"}, item), "minItems": 1, "maxItems": 6},
	}
	if fs.Has(prompts.FFreeInput) {
		props["placeholder"] = str("自由输入框的提示语（可选）")
	}
	if fs.Has(prompts.FTimed) {
		props["timeout_seconds"] = integer("限时秒数（可选）")
		props["default_choice"] = str("超时后默认选中的选项 ID")
	}
	return "在正文之后调用，将选项放入 choices 参数，平台会生成选项按钮；在正文中输出列表或 HTML 不会创建按钮。调用成功后结束本回合，等待玩家的作答作为此调用的工具结果返回。", object([]string{"choices"}, props)
}

type offerArgs struct {
	Prompt      string            `json:"prompt"`
	Choices     []save.ChoiceItem `json:"choices"`
	Placeholder string            `json:"placeholder"`
	Timeout     int               `json:"timeout_seconds"`
	Default     string            `json:"default_choice"`
}

func handleOfferChoices(_ context.Context, env Env, raw jsonx.Value) Result {
	var a offerArgs
	if err := decodeArgs(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），choices 可以写成 [{\"id\": \"a\", \"label\": \"……\"}]。", err)
	}
	if len(a.Choices) == 0 || len(a.Choices) > 6 {
		return NotExecuted("choices 需要 1–6 项，这次给了 %d 项。", len(a.Choices))
	}
	seen := map[string]bool{}
	for i, c := range a.Choices {
		id := strings.TrimSpace(c.ID)
		if id == "" || strings.TrimSpace(c.Label) == "" {
			return NotExecuted("第 %d 个选项缺少 id 或 label。", i+1)
		}
		if seen[id] {
			return NotExecuted("选项 id %q 重复了，可以用 a、b、c 依次编号。", id)
		}
		seen[id] = true
	}
	if a.Timeout < 0 || a.Timeout > 3600 {
		return NotExecuted("选项时限需在 0–3600 秒之间")
	}
	if a.Timeout > 0 && (!env.Features().Has(prompts.FTimed) || !seen[a.Default]) {
		return NotExecuted("限时选项需要作品启用限时，并填写有效的 default_choice")
	}
	env.OfferChoices(save.DisplayItem{
		T: "choices", TimeoutSeconds: a.Timeout, DefaultChoice: a.Default, Prompt: a.Prompt, Items: a.Choices, Free: env.Features().Has(prompts.FFreeInput),
	})
	return Result{Terminal: true}
}

// ---------------------------------------------------------------------------
// 其他

func specRollDice(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "需要随机结果时按 expr 掷骰，用 reason 说明原因；返回每颗骰子的点数与总和，按实际结果续写，玩家会看到骰子卡片。", object([]string{"expr"}, map[string]any{
		"expr":   str(`骰子表达式，如 "2d6+1"、"d100"、"1d20-2"`),
		"reason": str("为什么掷骰（显示在骰子卡片上）"),
	})
}

func specDelegate(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "用 kind 选择查找资料（research）或整理记忆（curate），task 描述任务；后台助手完成后带着出处汇报。", object([]string{"kind", "task"}, map[string]any{
		"kind": map[string]any{"type": "string", "enum": []string{"research", "curate"}},
		"task": str("任务说明"),
	})
}

// delegate 仅在支持委托的回合环境中执行，其他环境不会调用模型。
func delegate(ctx context.Context, env Env, raw jsonx.Value) Result {
	var args struct {
		Kind string `json:"kind"`
		Task string `json:"task"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return NotExecuted("参数不正确：%v", err)
	}
	if (args.Kind != "research" && args.Kind != "curate") || strings.TrimSpace(args.Task) == "" || len([]rune(args.Task)) > 4000 {
		return NotExecuted("kind 需为 research 或 curate，task 需为 1–4000 字")
	}
	runner, ok := env.(interface {
		Delegate(context.Context, string, string) (string, error)
	})
	if !ok {
		return NotExecuted("当前环境没有后台助手")
	}
	result, err := runner.Delegate(ctx, args.Kind, args.Task)
	if err != nil {
		return NotExecuted("助手未完成：%v", err)
	}
	return Text("%s", result)
}
