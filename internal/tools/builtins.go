package tools

import (
	"context"
	"strings"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/work"
)

// builtins 返回内置工具（《02-技术方案》10.2），顺序固定。
func builtins() []*Def {
	return []*Def{
		{Name: "read", Kind: KindRead, Display: DisplaySubtle, Spec: specRead, Handle: handleRead},
		{Name: "list", Kind: KindRead, Display: DisplaySubtle, Spec: specList, Handle: handleList},
		{Name: "search", Kind: KindRead, Display: DisplaySubtle, Spec: specSearch, Handle: notYet("search", "M3-4")},
		{Name: "write", Kind: KindWrite, Display: DisplayHidden, Spec: specWrite, Handle: notYet("write", "M3-2")},
		{Name: "edit", Kind: KindWrite, Display: DisplayHidden, Spec: specEdit, Handle: notYet("edit", "M3-2")},
		{Name: "remove", Kind: KindWrite, Display: DisplayHidden, Spec: specRemove, Handle: notYet("remove", "M3-2")},
		{Name: "get_state", Kind: KindRead, Display: DisplayHidden, When: prompts.Requires(prompts.FState),
			Spec: specGetState, Handle: handleGetState},
		{Name: "update_state", Kind: KindWrite, Display: DisplayHidden, When: prompts.Requires(prompts.FState),
			Spec: specUpdateState, Handle: handleUpdateState},
		{Name: "offer_choices", Kind: KindTerminal, Display: DisplayCustom, Spec: specOfferChoices, Handle: handleOfferChoices},
		// roll_dice 归入写类（串行）：它在正文流中追加骰子卡片，按调用顺序执行，卡片顺序与调用顺序一致
		{Name: "roll_dice", Kind: KindWrite, Display: DisplayCard, When: prompts.Requires(prompts.FDice),
			Spec: specRollDice, Handle: handleRollDice},
		{Name: "delegate", Kind: KindDelegate, Display: DisplaySubtle, When: prompts.Requires(prompts.FHelpers),
			Spec: specDelegate, Handle: notYet("delegate", "M5-8")},
	}
}

// notYet 是尚未实现的工具：返回事实性说明，模型可以据此继续写作。
func notYet(name, task string) func(context.Context, Env, jsonx.Value) Result {
	return func(context.Context, Env, jsonx.Value) Result {
		_ = task // 对应的实施计划任务编号；git grep 该编号可找到全部相关位置
		return NotExecuted("%s 暂时不可用，可以先依据上下文中已有的信息继续写。", name)
	}
}

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
	desc := "读取文件：作品资料、记忆、原文（如 /transcript/0042）。文本按行读取。"
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
	return "列出目录内容，每项附一行说明。", object([]string{"path"}, map[string]any{
		"path":  str("目录路径，如 /memory"),
		"depth": integer("展开层数 1–3，默认 1"),
	})
}

func specSearch(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "在记忆、作品资料与已发生的原文中检索文字，结果附出处。", object([]string{"query"}, map[string]any{
		"query": str("要查找的文字"),
		"path":  str("限定路径前缀，默认 /"),
		"regex": boolean("按正则匹配，默认 false"),
		"limit": integer("结果条数，默认 20，上限 50"),
	})
}

func specWrite(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "在 /memory 下新建或覆盖一个记忆文件（.md / .txt / .yaml）。", object([]string{"path", "content"}, map[string]any{
		"path":    str("文件路径，如 /memory/characters/lin.md"),
		"content": str("完整内容"),
	})
}

func specEdit(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "精确替换记忆文件中的一段文字。", object([]string{"path", "old_text", "new_text"}, map[string]any{
		"path":        str("文件路径"),
		"old_text":    str("原文字（需唯一匹配，除非 replace_all）"),
		"new_text":    str("新文字"),
		"replace_all": boolean("替换全部匹配，默认 false"),
	})
}

func specRemove(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "删除一个记忆文件。", object([]string{"path"}, map[string]any{"path": str("文件路径")})
}

// ---------------------------------------------------------------------------
// 状态

func specGetState(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "读取作品状态的当前值。", object(nil, map[string]any{
		"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "字段路径；缺省为全部字段"},
	})
}

func specUpdateState(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	op := object([]string{"op", "path"}, map[string]any{
		"op":    map[string]any{"type": "string", "enum": []string{"set", "inc", "append", "remove", "delete"}},
		"path":  str("字段路径，如 day、trust.林舟"),
		"value": map[string]any{"description": "新值（set）、增量（inc）或列表元素（append / remove）"},
	})
	return "更新作品状态。多个操作一起生效。", object([]string{"ops"}, map[string]any{
		"ops":    map[string]any{"type": "array", "items": op, "minItems": 1},
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
	return "给出选项并结束本回合。选项以按钮显示给玩家。", object([]string{"choices"}, props)
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
	if err := jsonx.Unmarshal(raw, &a); err != nil {
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
	// TODO(M5-3)：限时选项（timeout_seconds / default_choice）写入显示项。
	env.OfferChoices(save.DisplayItem{
		T: "choices", Prompt: a.Prompt, Items: a.Choices, Free: env.Features().Has(prompts.FFreeInput),
	})
	return Result{Terminal: true}
}

// ---------------------------------------------------------------------------
// 其他

func specRollDice(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "掷骰，返回每颗骰子的点数与总和；玩家会看到骰子卡片。", object([]string{"expr"}, map[string]any{
		"expr":   str(`骰子表达式，如 "2d6+1"、"d100"、"1d20-2"`),
		"reason": str("为什么掷骰（显示在骰子卡片上）"),
	})
}

func specDelegate(prompts.FeatureSet, *work.Model) (string, map[string]any) {
	return "把查找资料（research）或整理记忆（curate）交给后台助手，它会带着出处回来汇报。", object([]string{"kind", "task"}, map[string]any{
		"kind": map[string]any{"type": "string", "enum": []string{"research", "curate"}},
		"task": str("任务说明"),
	})
}
