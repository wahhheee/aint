package runtime

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/markup"
	"github.com/wahhheee/aint/internal/media"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/state"
	"github.com/wahhheee/aint/internal/tools"
	"github.com/wahhheee/aint/internal/usage"
	"github.com/wahhheee/aint/internal/vfs"
	"github.com/wahhheee/aint/internal/work"
)

// draft 是进行中回合的内存草稿。只由回合 goroutine 访问（工具的读并行只读取不可变字段）。
// 提交前所有变化都只在这里；崩溃即丢弃，不会产生半提交。
type draft struct {
	budget   Budget
	delegate func(context.Context, string, string) (string, error)
	id       string
	started  time.Time
	slot     *turnSlot // 写者槽位：提交后需要整理时交给整理任务
	release  func()    // 让出存档的单一写者槽位；发布终态事件前调用（见 Manager.Submit）

	// 只读上下文（prepare 阶段确定）
	save      *save.Save
	parent    *save.Node // 本回合的父节点（= 提交前的 head；回合内整理后为整理节点）
	root      *save.Node
	initState map[string]any // 存档的初始状态（执笔指令中的 Liquid 以它渲染）
	turnNo    int
	input     save.TurnInput
	opening   bool // 开场及中断开场的续写，不计为玩家首轮交互
	work      *work.Model
	features  prompts.FeatureSet
	provider  llm.Provider
	model     *llm.Model
	built     *contextx.Built
	vfs       *vfs.Table   // 本回合的挂载表（buildVFS）
	images    *media.Store // 发送给模型的图片（read 看图）
	memory    *vfs.Overlay // 记忆覆盖层：write / edit / remove 写在这里，提交时 Dirty 才生成新清单

	// 状态：以父节点快照为起点，update_state 成功后整体替换（state.Apply 不修改原映射，读取方拿到的快照不会再变）
	state        map[string]any
	stateChanged bool
	migration    []state.MigrationValue
	stateLog     []state.Change
	onState      func(changes []state.Change) // 状态变化后的通知：推送 turn.state、去抖重渲染面板
	onCard       func(item save.DisplayItem)  // 追加展示项后的通知：推送 turn.display

	// 本回合产生的内容
	req        *llm.Request  // 随调用追加消息
	turnMsgs   []llm.Message // 本回合的消息（玩家消息 + 助手 + 工具结果），提交时存为节点 messages
	display    []save.DisplayItem
	choices    *save.DisplayItem
	seq        int // turn.text 事件序号
	records    []usage.Record
	toolLog    []tools.CallRecord
	inspection *debugRecorder

	// 上下文规模（token）：threshold 是整理阈值（用量环的分母），hardLimit 是回合内必须先整理的硬上限；
	// lastInput / lastOutput 是最近一次调用的实际用量，提交时记为节点的上下文规模。
	threshold, hardLimit  int
	lastInput, lastOutput int
	compacted             bool // 本回合已经在回合内整理过一次（每回合至多一次）
	sinceCompaction       int  // 上次整理之后的回合数（含本回合）；还没有整理过时为全部回合数

	// 计数与结果
	calls, toolCalls, continuations int
	helperCalls                     int
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
func (d *draft) VFS() vfs.FS                  { return d.vfs }
func (d *draft) OfferChoices(item save.DisplayItem) {
	d.choices = &item
}
func (d *draft) ShowCard(item save.DisplayItem) {
	d.display = append(d.display, item)
	if d.onCard != nil {
		d.onCard(item)
	}
}

func (d *draft) State() map[string]any { return d.state }

func (d *draft) ModelImage(ctx context.Context, blob string) (llm.ImageRef, error) {
	return d.images.ForModel(ctx, blob, d.model.Caps.ImageMaxEdge)
}

func (d *draft) UpdateState(ops []state.Op) ([]state.Change, error) {
	next, changes, err := state.Apply(d.work.State, d.state, ops)
	if err != nil {
		return nil, err
	}
	d.state, d.stateChanged = next, true
	d.stateLog = append(d.stateLog, changes...)
	if d.onState != nil {
		d.onState(changes)
	}
	return changes, nil
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

// hasNarrative 判断本回合是否有故事正文；场外回应与玩家输入不计入。
func (d *draft) hasNarrative() bool {
	var text strings.Builder
	for _, it := range d.display {
		if it.T == "text" {
			text.WriteString(it.Markup)
			text.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(markup.PlainText(text.String())) != ""
}

func (d *draft) Delegate(ctx context.Context, kind, task string) (string, error) {
	return d.delegate(ctx, kind, task)
}
