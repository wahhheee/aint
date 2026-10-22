package contextx

import (
	"fmt"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
)

// Envelope 是一个回合的玩家消息（《02-技术方案》8.3）：
//
//	[offer_choices 的工具结果]   ← 上一回合以 offer_choices 结束时
//	<notes>…</notes>             ← 两回合之间的系统注记（无内容时省略）
//	<input kind="…" mode="…">…</input>   或  <ooc>…</ooc>
//	<pace>本回合篇幅：…</pace>   ← 总是附带，篇幅控制留在尾部而不是前缀
type Envelope struct {
	Input       save.TurnInput
	Text        string // 发给模型的输入文字：玩家原文、命令展开后的文字、开场指引等
	ModeLabel   string // 输入模式的显示名（如「推论」）
	Notes       []string
	Pace        string // 生效的篇幅档位
	PendingCall string // 上一回合未作答的 offer_choices 调用 ID
	// Uploads 是本回合附带的图片在 /uploads 中的路径；Images 是发送给模型的图片（与 Uploads 一一对应，
	// 模型不能看图时为空，只写文字说明）。
	Uploads []string
	Images  []llm.ImageRef
}

// Message 生成本回合的 user 消息。
func (e Envelope) Message(fs prompts.FeatureSet) llm.Message {
	var parts []llm.Part
	answered := false
	if e.PendingCall != "" {
		result, ok := e.answer()
		answered = ok
		parts = append(parts, llm.Part{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{
			CallID: e.PendingCall, Parts: []llm.Part{llm.Text(result)},
		}})
	}

	var b strings.Builder
	if len(e.Notes) > 0 {
		b.WriteString("<notes>\n")
		for _, n := range e.Notes {
			b.WriteString("- " + n + "\n")
		}
		b.WriteString("</notes>\n")
	}
	switch {
	case e.Input.Kind == save.InputOOC && fs.Has(prompts.FOOC):
		fmt.Fprintf(&b, "<ooc>\n%s\n</ooc>\n", e.Text)
	case !answered:
		attrs := fmt.Sprintf(` kind="%s"`, e.Input.Kind)
		if e.ModeLabel != "" && fs.Has(prompts.FInputModes) {
			attrs += fmt.Sprintf(` mode="%s"`, e.ModeLabel)
		}
		fmt.Fprintf(&b, "<input%s>\n%s\n</input>\n", attrs, e.inputText())
	}
	if n := len(e.Uploads); n > 0 {
		if len(e.Images) == n {
			fmt.Fprintf(&b, "<attachments>玩家附带了 %d 张图片（%s），见下方。</attachments>\n", n, strings.Join(e.Uploads, "、"))
		} else {
			fmt.Fprintf(&b, "<attachments>玩家附带了 %d 张图片，当前模型不能看图，可以请玩家用文字描述。</attachments>\n", n)
		}
	}
	fmt.Fprintf(&b, "<pace>本回合篇幅：%s</pace>", PaceText(e.Pace))
	parts = append(parts, llm.Text(b.String()))
	for i := range e.Images {
		parts = append(parts, llm.Part{Kind: llm.PartImage, Image: &e.Images[i]})
	}
	return llm.Message{Role: llm.RoleUser, Parts: parts}
}

// answer 把选项或自由输入写成 offer_choices 的工具结果；其他输入（命令、场外等）不算作答。
func (e Envelope) answer() (string, bool) {
	switch e.Input.Kind {
	case save.InputChoice:
		return e.choiceText(), true
	case save.InputFree:
		return "玩家自由输入：" + e.Text, true
	}
	return prompts.ChoiceNotAnswered, false // D-29
}

func (e Envelope) inputText() string {
	switch e.Input.Kind {
	case save.InputChoice:
		return e.choiceText()
	case save.InputContinue:
		if e.Text == "" {
			return "（玩家希望你接着写下去）"
		}
	}
	return e.Text
}

func (e Envelope) choiceText() string {
	return fmt.Sprintf("玩家选择：%s「%s」", strings.ToUpper(e.Input.ChoiceID), e.Input.ChoiceLabel)
}

// PaceText 把篇幅档位写成给模型看的文字："中（约 800–1500 字）"；自定义 "300-800" → "约 300–800 字"。
func PaceText(p string) string {
	if v, ok := prompts.Pace[p]; ok {
		return fmt.Sprintf("%s（约 %d–%d 字）", v.Label, v.Min, v.Max)
	}
	if lo, hi, ok := strings.Cut(p, "-"); ok {
		return fmt.Sprintf("约 %s–%s 字", strings.TrimSpace(lo), strings.TrimSpace(hi))
	}
	v := prompts.Pace["medium"]
	return fmt.Sprintf("%s（约 %d–%d 字）", v.Label, v.Min, v.Max)
}
