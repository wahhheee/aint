// Package tools 是主笔可用的工具：注册表、参数裁切、执行顺序、内置工具实现。
//
// 约定（《02-技术方案》第 10 节）：
//   - 工具集合在一个阶段内固定：由作品版本、features、助手开关决定（前缀稳定）；
//     工具"当前不可用"（如整理阶段）时返回说明，不从列表移除；
//   - 同一条助手消息中的多个调用：read 类并行、write 类按原顺序串行、terminal 最后；结果按原顺序返回；
//   - 未执行时结果以"未执行："开头，陈述原因并给出可行写法（D-21：结果即事实）；
//   - 工具只能通过 Env 访问本存档的数据（不变量：只能访问 VFS，写操作仅限 /memory）。
package tools

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/vfs"
	"github.com/wahhheee/yanyi/internal/work"
)

// Kind 决定执行顺序。
type Kind string

const (
	KindRead     Kind = "read"     // 并行
	KindWrite    Kind = "write"    // 按原顺序串行
	KindTerminal Kind = "terminal" // 最后执行，执行后结束回合
	KindDelegate Kind = "delegate" // 后台助手（research 并行、curate 串行；目前统一串行）
)

// Display 是工具调用在玩家界面上的显示策略（10.3）。
type Display string

const (
	DisplayHidden Display = "hidden" // 默认隐藏（记忆写入）
	DisplaySubtle Display = "subtle" // 低调提示："翻阅了 3 份资料"
	DisplayCard   Display = "card"   // 卡片（骰子）
	DisplayCustom Display = "custom" // 专用组件（选项）
)

// MaxResultChars 是工具结果的大小上限，超出截断并注明。
const MaxResultChars = 8000

// Env 是工具的执行环境，由 runtime 的回合草稿实现。工具只能通过它访问存档数据。
type Env interface {
	Features() prompts.FeatureSet
	Work() *work.Model
	// OfferChoices 记录本回合给出的选项（offer_choices）。
	OfferChoices(item save.DisplayItem)
	// ShowCard 在正文流中追加一个展示项（骰子卡片等），并实时推送给玩家。只由写类工具调用（串行）。
	ShowCard(item save.DisplayItem)
	// VFS 是本回合的虚拟文件系统（/work /memory /transcript /uploads）。
	// TODO(M3-2)：写入（write / edit / remove 进回合覆盖层）。
	VFS() vfs.FS
	// State 返回本回合当前的状态（以父节点快照为起点，含本回合已生效的修改）。只读：不得修改返回的映射。
	State() map[string]any
	// UpdateState 原子地执行状态操作（state.Apply）；成功后本回合之后的读取看到新状态，回合提交时写入新快照。
	UpdateState(ops []state.Op) ([]state.Change, error)
}

// Result 是一次工具调用的结果。
type Result struct {
	Parts    []llm.Part // 只允许 text 与 image
	IsError  bool
	Terminal bool // 终止型工具：执行后结束回合，结果在下一回合由玩家的输入回填
}

// Text 构造文本结果。
func Text(format string, args ...any) Result {
	return Result{Parts: []llm.Part{llm.Text(fmt.Sprintf(format, args...))}}
}

// NotExecuted 构造"未执行"结果：陈述原因，并给出接下来可以怎么写。
func NotExecuted(format string, args ...any) Result {
	return Result{Parts: []llm.Part{llm.Text("未执行：" + fmt.Sprintf(format, args...))}, IsError: true}
}

// Def 是一个工具定义。
type Def struct {
	Name    string
	Kind    Kind
	Display Display
	When    prompts.Cond // 可用条件；不满足则不注册（连同提到它的提示词一起消失）
	// Spec 按能力画像生成描述与参数（裁切：没有命令就没有 command 参数……）。
	Spec   func(fs prompts.FeatureSet, m *work.Model) (description string, schema map[string]any)
	Handle func(ctx context.Context, env Env, args jsonx.Value) Result
}

// Registry 是工具注册表。注册顺序即发给模型的顺序（固定，保证前缀稳定）。
type Registry struct {
	defs   []*Def
	byName map[string]*Def
}

// NewRegistry 创建注册表并注册全部内置工具。
func NewRegistry() *Registry {
	r := &Registry{byName: map[string]*Def{}}
	for _, d := range builtins() {
		r.defs = append(r.defs, d)
		r.byName[d.Name] = d
	}
	return r
}

// Specs 返回当前可用的工具定义（已裁切，顺序固定）。
func (r *Registry) Specs(fs prompts.FeatureSet, m *work.Model) []llm.ToolSpec {
	var out []llm.ToolSpec
	for _, d := range r.defs {
		if ok, _ := d.When.Eval(fs); !ok {
			continue
		}
		desc, schema := d.Spec(fs, m)
		out = append(out, llm.ToolSpec{Name: d.Name, Description: desc, Schema: jsonx.MustMarshal(schema)})
	}
	return out
}

// Def 按名称查找工具定义（显示策略等）。
func (r *Registry) Def(name string) (*Def, bool) {
	d, ok := r.byName[name]
	return d, ok
}

// CallRecord 是一次调用的排查记录（检查器"工具"页）。
type CallRecord struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Args       jsonx.Value `json:"args"`
	Result     string      `json:"result"`
	IsError    bool        `json:"is_error,omitzero"`
	DurationMS int64       `json:"duration_ms"`
}

// Outcome 是一批工具调用的执行结果。
type Outcome struct {
	Results  []llm.ToolResult // 按原顺序；不含终止型调用（它的结果由下一回合回填）
	Terminal *llm.ToolCall    // 本批中的终止型调用；nil 表示回合继续
	Records  []CallRecord
}

// Execute 执行一批工具调用：读并行、写串行、终止型最后；结果按原顺序返回。
func (r *Registry) Execute(ctx context.Context, env Env, calls []llm.ToolCall) Outcome {
	results := make([]*Result, len(calls))
	durations := make([]time.Duration, len(calls))
	run := func(i int, d *Def) {
		start := time.Now()
		res := safeHandle(ctx, d, env, calls[i])
		results[i], durations[i] = &res, time.Since(start)
	}

	fs := env.Features()
	var reads, writes []int
	terminal := -1
	for i, c := range calls {
		d, ok := r.byName[c.Name]
		if ok {
			if avail, _ := d.When.Eval(fs); !avail {
				ok = false
			}
		}
		switch {
		case !ok:
			res := NotExecuted("没有名为 %s 的工具，可用的工具见工具列表。", c.Name)
			results[i] = &res
		case d.Kind == KindRead:
			reads = append(reads, i)
		case d.Kind == KindTerminal:
			if terminal >= 0 {
				res := NotExecuted("本回合已经给出过选项，这一次调用没有生效。")
				results[i] = &res
			} else {
				terminal = i
			}
		default:
			writes = append(writes, i)
		}
	}

	var wg sync.WaitGroup
	for _, i := range reads {
		wg.Go(func() { run(i, r.byName[calls[i].Name]) })
	}
	wg.Wait()
	for _, i := range writes {
		run(i, r.byName[calls[i].Name])
	}
	if terminal >= 0 {
		run(terminal, r.byName[calls[terminal].Name])
	}

	var out Outcome
	for i, c := range calls {
		res := results[i]
		out.Records = append(out.Records, CallRecord{
			ID: c.ID, Name: c.Name, Args: c.Args, Result: resultText(res), IsError: res.IsError,
			DurationMS: durations[i].Milliseconds(),
		})
		if res.Terminal {
			tc := c
			out.Terminal = &tc
			continue
		}
		out.Results = append(out.Results, llm.ToolResult{CallID: c.ID, Parts: truncate(res.Parts), IsError: res.IsError})
	}
	return out
}

// safeHandle 执行工具并兜住 panic：单个工具的缺陷不应让整个回合崩溃。
func safeHandle(ctx context.Context, d *Def, env Env, c llm.ToolCall) (res Result) {
	defer func() {
		if p := recover(); p != nil {
			slog.ErrorContext(ctx, "工具执行 panic", "tool", c.Name, "panic", p)
			res = NotExecuted("工具 %s 内部出错，可以换一种写法继续。", c.Name)
		}
	}()
	return d.Handle(ctx, env, c.Args)
}

func resultText(r *Result) string {
	for _, p := range r.Parts {
		if p.Kind == llm.PartText {
			return p.Text
		}
	}
	return ""
}

func truncate(parts []llm.Part) []llm.Part {
	out := make([]llm.Part, len(parts))
	copy(out, parts)
	for i, p := range out {
		if p.Kind == llm.PartText && utf8.RuneCountInString(p.Text) > MaxResultChars {
			r := []rune(p.Text)
			out[i].Text = string(r[:MaxResultChars]) + "\n（结果过长，已截断；可以缩小范围后再查）"
		}
	}
	return out
}
