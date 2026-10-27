// Package save 负责存档：存档条目、节点树（分支）、进度线、回合提交、时间线、标题。
//
// 核心模型（《02-技术方案》第 6 节，决策 D-09）：
//
//		root ─ t1(开场) ─ t2 ─ t3 ─┬─ t4 ─ compaction ─ t5 … (head)
//		                           ├─ t4′（重新生成的另一版本）
//		                           └─ t4″ ─ t5″（回到 t3 后继续形成的分支）
//
//	  - 每回合提交即自动保存并移动 head；游玩、重新生成、分支都在存档内部进行，不产生新存档条目；
//	  - 进度线 = root 到 head 的路径；状态、记忆、原文检索都只看进度线（分支隔离，不变量 3）；
//	  - 节点内容（消息、显示项、状态快照、记忆清单、阶段块）以 blob 哈希引用，分支间自然共享。
//
// 本包只做存储与查询，不调用模型；回合怎么跑由 runtime 负责，跑完通过 CommitTurn 一次性提交。
package save

import (
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/state"
)

// NodeKind 是节点类型。
type NodeKind string

const (
	KindRoot       NodeKind = "root"       // 新建存档：初始状态、初始记忆、第 0 阶段块
	KindTurn       NodeKind = "turn"       // 一个回合（含开场）
	KindCompaction NodeKind = "compaction" // 上下文整理（含存档升级）
	KindEdit       NodeKind = "edit"       // 玩家手动改状态 / 记忆
	KindChoices    NodeKind = "choices"    // 当前回合的补充选项，不增加回合号
)

// NodeStatus 是节点状态。
type NodeStatus string

const (
	StatusCommitted   NodeStatus = "committed"
	StatusInterrupted NodeStatus = "interrupted" // 停止 / 断网 / 出错：保留已生成内容，可继续 / 重新生成 / 丢弃
)

// Settings 是存档设置（saves.settings）。创建时由全局设置与作品推荐值合成，之后可单独修改。
type Settings struct {
	MaxCalls      int    `json:"max_calls,omitzero"`
	MaxToolCalls  int    `json:"max_tool_calls,omitzero"`
	MaxOutput     int    `json:"max_output,omitzero"`
	NarratorModel string `json:"narrator_model"`     // models.id；假模型模式下为 "fake"
	Pace          string `json:"pace"`               // short | medium | long | "<最少>-<最多>"
	Reasoning     string `json:"reasoning"`          // off | low | medium | high
	Helpers       bool   `json:"helpers"`            // 后台助手（P1）
	CacheTTL      string `json:"cache_ttl,omitzero"` // short | long（Anthropic 5 分钟 / 1 小时；OpenAI 30 分钟 / 12 小时）
	// 上下文整理的存档级覆盖；零值表示沿用全局设置（settings.Context）。
	ContextBudget int    `json:"context_budget,omitzero"`
	KeepRecent    int    `json:"keep_recent,omitzero"`
	CompactMode   string `json:"compact_mode,omitzero"` // inline | separate
}

// Save 是一个存档条目。
type Save struct {
	OpeningID     string         `json:"opening_id"`
	OpeningLocked bool           `json:"opening_locked"` // 首次玩家交互成功后永久锁定，不随 head 回退
	InitialForm   map[string]any `json:"-"`              // 建档时有效的开局表单，切换开场时重新应用
	ID            string         `json:"id"`
	WorkID        string         `json:"work_id"`
	WorkVersionID string         `json:"work_version_id"`
	WorkTitle     string         `json:"work_title"`
	Title         string         `json:"title"`        // 为空时界面显示"未命名存档 · 最近游玩时间"
	TitleSource   string         `json:"title_source"` // generated | manual | ""
	HeadNodeID    string         `json:"head_node_id"`
	TurnCount     int            `json:"turn_count"`   // head 所在进度线的回合数
	BranchCount   int            `json:"branch_count"` // 分支数（叶子数）
	ForkedFrom    *ForkInfo      `json:"forked_from,omitzero"`
	Settings      Settings       `json:"settings"`
	CreatedAt     int64          `json:"created_at"`
	UpdatedAt     int64          `json:"updated_at"`
	LastPlayedAt  int64          `json:"last_played_at"`
}

// Node 是存档树上的一个节点。
type Node struct {
	ID            string     `json:"id"`
	SaveID        string     `json:"save_id"`
	ParentID      string     `json:"parent_id,omitzero"`
	Depth         int        `json:"depth"`
	Kind          NodeKind   `json:"kind"`
	Status        NodeStatus `json:"status"`
	TurnNo        int        `json:"turn_no,omitzero"` // turn：回合序号；edit / compaction：所跟随的回合
	Input         *TurnInput `json:"input,omitzero"`
	MessagesHash  string     `json:"messages_hash,omitzero"`
	DisplayHash   string     `json:"display_hash,omitzero"`
	EpochHash     string     `json:"epoch_hash,omitzero"`
	KeepFrom      string     `json:"keep_from,omitzero"`
	StateHash     string     `json:"state_hash"`
	MemoryHash    string     `json:"memory_hash"`
	WorkVersionID string     `json:"work_version_id"`
	Notes         []string   `json:"notes,omitzero"`
	Usage         *llm.Usage `json:"usage,omitzero"`
	Meta          NodeMeta   `json:"meta"`
	CreatedAt     int64      `json:"created_at"`
}

// NodeMeta 是随节点保存的排查信息，记录停止原因、调用次数、耗时与上下文规模。
type NodeMeta struct {
	Operation     *TurnOperation         `json:"operation,omitzero"`
	VersionParent string                 `json:"version_parent,omitzero"` // 回合版本共同的分叉点，回合内整理不改变它
	Opening       bool                   `json:"opening,omitzero"`        // 开场及中断开场的续写
	Migration     []state.MigrationValue `json:"migration,omitzero"`
	WorkHash      string                 `json:"work_hash,omitzero"`
	Model         string                 `json:"model,omitzero"`
	Stop          string                 `json:"stop,omitzero"`     // 归一停止原因（或 budget / canceled / choices）
	StopRaw       string                 `json:"stop_raw,omitzero"` // 协议原始停止原因
	Calls         int                    `json:"calls,omitzero"`
	ToolCalls     int                    `json:"tool_calls,omitzero"`
	Continuations int                    `json:"continuations,omitzero"`
	DurationMS    int64                  `json:"duration_ms,omitzero"`
	Error         string                 `json:"error,omitzero"`
	PrefixHash    string                 `json:"prefix_hash,omitzero"` // L0–L3 + 工具的哈希；变化即缓存重建
	PromptVersion string                 `json:"prompt_version,omitzero"`
	EditedFrom    string                 `json:"edited_from,omitzero"`
	// ContextTokens 是这个节点之后的上下文规模（token）：回合为最后一次调用的输入 + 输出（实测），
	// 整理节点为新阶段的估算。下一次请求在此基础上追加新输入；整理阈值与界面的用量环都以它为准。
	ContextTokens int `json:"context_tokens,omitzero"`
	// 整理节点：本次整理摘要掉的回合区间与保留原文的起点回合
	CompactReason string `json:"compact_reason,omitzero"`
	SummaryFrom   int    `json:"summary_from,omitzero"`
	SummaryTo     int    `json:"summary_to,omitzero"`
	KeepFromTurn  int    `json:"keep_from_turn,omitzero"`
}

// TurnOperation 是附着在已有回合上的操作；不属于玩家在故事中的行动。
type TurnOperation struct {
	Kind         string `json:"kind"` // rewrite | choices
	NodeID       string `json:"node_id"`
	Instructions string `json:"instructions,omitzero"`
	ExpectedHead string `json:"expected_head"`
}

// TurnInput 是玩家一回合的输入（《02-技术方案》9.1）。
type TurnInput struct {
	ExpectedHeadID string       `json:"expected_head_id,omitzero"` // 自动开场依据的 head，防止旧页面启动已切换的开场
	Kind           string       `json:"kind"`                      // free | choice | command | continue | opening | ooc
	Text           string       `json:"text,omitzero"`
	Mode           string       `json:"mode,omitzero"`         // 输入模式（present: mode 的命令名）
	ChoiceID       string       `json:"choice_id,omitzero"`    // 所选选项的 ID，选项附带命令时也保留
	ChoiceLabel    string       `json:"choice_label,omitzero"` // 服务端根据上一回合的选项补全，用于显示与工具结果
	Command        *CommandCall `json:"command,omitzero"`
	Attachments    []string     `json:"attachments,omitzero"` // 上传图片 blob 哈希
	Pace           string       `json:"pace,omitzero"`        // 本回合篇幅覆盖
}

// CommandCall 是命令调用。
type CommandCall struct {
	Name string            `json:"name"`
	Args map[string]string `json:"args,omitzero"`
}

// 输入类型。
const (
	InputFree     = "free"
	InputChoice   = "choice"
	InputCommand  = "command"
	InputContinue = "continue"
	InputOpening  = "opening"
	InputOOC      = "ooc"
)

// DisplayItem 是正文流中的一个显示项（《02-技术方案》16.4），按 T 使用对应字段。
// 显示项是"玩家看到什么"的权威记录：时间线接口直接返回，前端据此渲染。
type DisplayItem struct {
	T string `json:"t"` // input | text | tools | dice | image | choices | notice

	// input
	Kind        string   `json:"kind,omitzero"`
	Mode        string   `json:"mode,omitzero"`
	Attachments []string `json:"attachments,omitzero"`
	// input / notice：文字；dice：掷骰原因
	Text string `json:"text,omitzero"`
	// text：叙事标记源码
	Markup string `json:"markup,omitzero"`
	// tools："翻阅了 2 份资料"
	Summary string `json:"summary,omitzero"`
	// dice
	Expr  string `json:"expr,omitzero"`
	Rolls []int  `json:"rolls,omitzero"`
	Total int    `json:"total,omitzero"`
	// image
	Source string `json:"source,omitzero"` // work | upload | generated
	Asset  string `json:"asset,omitzero"`
	Status string `json:"status,omitzero"` // pending | ready | failed
	// choices
	TimeoutSeconds int          `json:"timeout_seconds,omitzero"`
	DefaultChoice  string       `json:"default_choice,omitzero"`
	Prompt         string       `json:"prompt,omitzero"`
	Items          []ChoiceItem `json:"items,omitzero"`
	Free           bool         `json:"free,omitzero"` // 是否允许自由输入
	Selected       string       `json:"selected,omitzero"`
	// notice
	Level string `json:"level,omitzero"` // info | warn | error
}

// ChoiceItem 是一个选项。
type ChoiceItem struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Detail  string `json:"detail,omitzero"`
	Style   string `json:"style,omitzero"`
	Command string `json:"command,omitzero"`
}

// MemoryManifest 是记忆清单（《02-技术方案》4.3）。节点未改记忆时复用父节点的哈希。
type MemoryManifest struct {
	V     int                   `json:"v"`
	Files map[string]MemoryFile `json:"files"`
}

// MemoryFile 是记忆清单中的一个文件。
type MemoryFile struct {
	Blob string `json:"blob"`
	Size int    `json:"size"`
	Turn int    `json:"turn"` // 最后修改该文件的回合号
}
