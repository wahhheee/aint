package prompts

// 固定文案（《02-技术方案》20.6）。工具返回、注记等同样遵循"结果即事实、正向帮助"的写法。
const (
	// ContinueNotice 截断续写时追加的 user 消息（9.4）。
	ContinueNotice = "<notes>上一段输出到达了单次长度上限，已写的内容都保留着。请从断点处接着写下去。</notes>"
	// CompactionPendingChoice 整理时为尚未作答的 offer_choices 补齐的工具结果（13.3）。
	CompactionPendingChoice = "（玩家还没有作答，系统先整理上下文）"
	// CompactionToolUnavailable 整理阶段调用了不使用的工具（13.3）。
	CompactionToolUnavailable = "整理阶段暂不使用这个工具。"
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

// CompactionTask 是整理指令（20.3）。text/template；可用 has 与 .SummaryChars、.Guide。
//
// TODO(M3-6)：由 compact 包使用。
const CompactionTask = `<system_task kind="compaction">
接下来要整理上下文：此前的对话会由一份交接摘要接替，最近几个回合的原文会保留下来。请按顺序完成两步。

第一步，记下要紧的事。回顾本阶段，把今后还用得上、而 /memory 里还没有的内容写进记忆：新的设定事实，人物处境、关系与认知的变化，伏笔、承诺与悬念，玩家的重要选择。再为本阶段写一份章节纪要，放在 /memory/chronicle/。

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
