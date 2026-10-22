package runtime

import (
	"strconv"
	"strings"
	"time"

	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/tools"
	"github.com/wahhheee/yanyi/internal/usage"
	"github.com/wahhheee/yanyi/internal/work"
)

// draft 是进行中回合的内存草稿。只由回合 goroutine 访问（工具的读并行只读取不可变字段）。
// 提交前所有变化都只在这里；崩溃即丢弃，不会产生半提交。
type draft struct {
	id      string
	started time.Time
	release func() // 让出存档的单一写者槽位；发布终态事件前调用（见 Manager.Submit）

	// 只读上下文（prepare 阶段确定）
	save     *save.Save
	parent   *save.Node // 本回合的父节点（= 提交前的 head）
	turnNo   int
	input    save.TurnInput
	work     *work.Model
	features prompts.FeatureSet
	provider llm.Provider
	model    *llm.Model
	built    *contextx.Built

	// 本回合产生的内容
	req      *llm.Request  // 随调用追加消息
	turnMsgs []llm.Message // 本回合的消息（玩家消息 + 助手 + 工具结果），提交时存为节点 messages
	display  []save.DisplayItem
	choices  *save.DisplayItem
	seq      int // turn.text 事件序号
	records  []usage.Record
	toolLog  []tools.CallRecord

	// 计数与结果
	calls, toolCalls, continuations int
	continuing                      bool // 下一次调用是截断后的续写（只对紧接着的那一次调用有效）
	usage                           llm.Usage
	stop, stopRaw, errMsg           string
	err                             error // 原始错误（保留类型，用于给玩家的错误分类）；errMsg 是它的文字
	status                          save.NodeStatus
}

// ---------------------------------------------------------------------------
// tools.Env 实现

var _ tools.Env = (*draft)(nil)

func (d *draft) Features() prompts.FeatureSet { return d.features }
func (d *draft) Work() *work.Model            { return d.work }
func (d *draft) OfferChoices(item save.DisplayItem) {
	d.choices = &item
}

// ---------------------------------------------------------------------------
// 显示项

// appendText 追加正文增量：与上一个显示项同为正文时合并，否则新开一个正文项。
func (d *draft) appendText(delta string) {
	if n := len(d.display); n > 0 && d.display[n-1].T == "text" {
		d.display[n-1].Markup += delta
		return
	}
	d.display = append(d.display, save.DisplayItem{T: "text", Markup: delta})
}

// lastText 返回最近一个正文显示项的内容（续写去重时与新输出的开头比较）。
func (d *draft) lastText() string {
	for i := len(d.display) - 1; i >= 0; i-- {
		if d.display[i].T == "text" {
			return d.display[i].Markup
		}
	}
	return ""
}

// addToolSummary 在玩家界面上合并显示读取类工具："翻阅了 N 份资料"（10.3）。
func (d *draft) addToolSummary(n int) {
	if n <= 0 {
		return
	}
	d.display = append(d.display, save.DisplayItem{T: "tools", Summary: "翻阅了 " + strconv.Itoa(n) + " 份资料"})
}

func (d *draft) addNotice(level, text string) {
	d.display = append(d.display, save.DisplayItem{T: "notice", Level: level, Text: text})
}

// hasContent 表示本回合是否产生了值得保留的内容（中断时决定是否提交）。
func (d *draft) hasContent() bool {
	for _, it := range d.display {
		if it.T == "text" && strings.TrimSpace(it.Markup) != "" {
			return true
		}
	}
	return d.choices != nil
}

// markupText 汇总本回合正文的标记源码（提交时转纯文本写入原文索引）。
func (d *draft) markupText() string {
	var b strings.Builder
	for _, it := range d.display {
		if it.T == "text" {
			b.WriteString(it.Markup)
			b.WriteString("\n\n")
		}
	}
	return b.String()
}
