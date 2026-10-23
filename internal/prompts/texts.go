package prompts

// 固定文案（《02-技术方案》20.6）。工具返回、注记等同样遵循"结果即事实、正向帮助"的写法。
const (
	// ContinueNotice 截断续写时追加的 user 消息（9.4）。
	ContinueNotice = "<notes>上一段输出到达了单次长度上限，已写的内容都保留着。请从断点处接着写下去。</notes>"
	// CompactionPendingChoice 整理时为尚未作答的 offer_choices 补齐的工具结果（13.3）。
	CompactionPendingChoice = "（玩家还没有作答，系统先整理上下文）"
	// CompactionToolUnavailable 整理阶段调用了不使用的工具（13.3）。
	CompactionToolUnavailable = "整理阶段暂不使用这个工具。"
	// ChoiceNotAnswered 选项待答时玩家用了命令 / 场外 / 继续：offer_choices 的工具结果（8.3，D-29）。
	ChoiceNotAnswered = "（玩家没有从选项中作答，而是另有输入，见下方）"
	// StaticOpeningInput 静态开场节点中合成的 user 消息，保证历史以 user 开头（9.8，D-33）。
	StaticOpeningInput = "<input kind=\"opening\">（开场）</input>"
	// OrphanChoiceResult 整理后保留原文的起点处，对应调用已不在上下文中的工具结果改写为文字（8.4）。
	OrphanChoiceResult = "（玩家对上一轮选项的回应：%s）"
)

// Pace 是篇幅档位的默认字数（8.3）。字数是给模型的目标，平台不截断。
var Pace = map[string]struct {
	Label    string
	Min, Max int
}{
	"short":  {"短", 300, 600},
	"medium": {"中", 800, 1500},
	"long":   {"长", 2000, 4000},
}

// CompactionTask 是整理指令（20.3）。text/template；可用 has 与 .SummaryChars、.MemoryLarge、.Guide。
// inline 模式下它是接在当前上下文末尾的一条 user 消息；separate 模式下接在本阶段原文之后。
const CompactionTask = `<system_task kind="compaction">
接下来要整理上下文：此前的对话会由一份交接摘要接替，最近几个回合的原文会保留下来。请按顺序完成两步。

第一步，记下要紧的事。回顾本阶段，把今后还用得上、而 /memory 里还没有的内容写进记忆：新的设定事实，人物处境、关系与认知的变化，伏笔、承诺与悬念，玩家的重要选择。再为本阶段写一份章节纪要，放在 /memory/chronicle/。
{{- if .MemoryLarge}}
记忆总量已经较大，可以顺便合并重复、删去过时的内容。
{{- end}}

第二步，写交接摘要。这一步直接输出摘要本身即可，结构如下：
## 故事至今
## 当前场景
## 人物与关系
## 线索·伏笔·承诺
## 玩家
## 写作备忘

写法：在上一份摘要的基础上更新；较早的情节可以压缩成章节级的一句话，并注明对应的记忆文件；"当前场景"与最新正文保持一致；尚未确定的事标注"未确认"；总长约 {{.SummaryChars}} 字。
{{- if has "compaction.guide"}}
{{.Guide}}
{{- end}}
</system_task>`

// 整理过程中的固定文案（13.3）。
const (
	// CompactionSummaryNudge 落盘调用次数用完、仍未给出摘要时追加的 user 消息。
	CompactionSummaryNudge = "记忆就记到这里，现在请直接输出交接摘要。"
	// CompactionSummaryShorter 摘要超过硬上限时追加的 user 消息（%d 为目标字数）。
	CompactionSummaryShorter = "这份摘要偏长，请压缩到约 %d 字，直接输出摘要本身。"
	// CompactionSummaryEmpty 没有给出摘要时追加的 user 消息。
	CompactionSummaryEmpty = "还没有看到交接摘要，请按第二步的结构直接输出摘要本身。"
)

// CompactionSeparateSystem 是 separate 模式（轻量模型单独整理）的系统提示。
const CompactionSeparateSystem = `你在为一部互动小说的执笔者整理上下文。执笔者与玩家已经写了很长一段故事，接下来要用一份交接摘要接替较早的对话。
你会看到作品标题、上一份交接摘要（如有）、本阶段的故事原文、当前状态与记忆目录；/work 是作品资料，/memory 是执笔者的记忆笔记，/transcript 是全部原文，可以用 read、list、search 查阅，用 write、edit、remove 整理记忆。
请按消息末尾 <system_task> 的要求完成：先把要紧的事写进 /memory，再直接输出交接摘要本身。`

// EpochIntro 是整理后阶段块的首句（13.4）：%d、%d 为保留原文的起止回合。
const EpochIntro = "以下是上下文整理后的交接信息，其后保留了第 %d–%d 回合的原文。"

// EpochIntroNoKept 是没有保留原文时（极少见：阶段内只有整理节点之后的回合）阶段块的首句。
const EpochIntroNoKept = "以下是上下文整理后的交接信息。"

// HelperResearch / HelperCurate 是后台助手提示词（20.4）。TODO(M5-8)：由 helper 包使用。
const (
	HelperResearch = `你是这部互动小说执笔者的资料助手。请按任务查阅{{if has "lore"}}/work、{{end}}/memory、/transcript{{if has "vision"}}、/uploads{{end}}，整理成一份简洁的调查结果：每条结论附上出处（文件路径或回合号），并标明哪些是原文明确写到的、哪些是推测。1500 字以内即可。`
	HelperCurate   = `你是这部互动小说执笔者的记忆整理助手。请按任务整理记忆文件：合并重复、补充修订，让每个文件保持简短，并同步更新 /memory/index.md。仍然有用的信息请保留，不确定的内容可以标注"未确认"。完成后用 300 字以内说明改动。`
)

// SaveTitle 是存档标题提示词（20.5）。TODO(M5-13)：由 save 的标题生成使用。
const SaveTitle = `请根据下面的作品与最新进度，为这个存档起一个便于辨认的标题：16 字以内，点出当前所处的情节或场景，写已经发生的内容。直接输出标题即可。

作品：{{.WorkTitle}}
{{- if .CurrentScene}}
当前场景：{{.CurrentScene}}
{{- end}}
最近进展：
{{.RecentTurns}}`
