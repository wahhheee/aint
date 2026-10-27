package prompts

// 固定文案（《02-技术方案》20.6）。工具返回、注记等同样遵循"结果即事实、正向帮助"的写法。
const (
	// RewriteMemoryCheck 只给成稿与草稿记忆，核对时不重放修改要求和旧版。
	RewriteMemoryCheck = `<system_task kind="check_memory">
本轮故事已经写完。下面 story 是唯一已发生的本轮正文，memory 是本轮写入的记忆文件，state 是本轮实际状态。现在核对这些记忆：本轮事件、台词与人物行动必须有 story 的依据，修改要求和写作风格不是玩家在故事中的偏好。若记录了正文没有发生的事情、未说出的台词或没有依据的偏好，请修正；台词宜概括含义。以前回合已经成立的事实保留，必要时可以只读查阅历史。不得修改金币等状态，也不继续创作故事或提供选项。把所有需要修正的文件在同一次回复中通过原生 write 调用批量写入完整内容（不再有下一轮修订）；仅有措辞差异而含义一致的不用修改。全部记录已经符合正文时直接结束，无需确认、汇报或解释。</system_task>`
	// RewriteTurn 把旧版作为参考材料，与已经发生的历史及当前工具状态分开。
	RewriteTurn = `<system_task kind="rewrite">
本次从这一回合开始之前重新写完整的一轮。rewrite_request.reference 是被替换的旧版，只用于理解修改要求；它不是本分支已经发生的历史。原玩家输入仍然有效，instructions 是本轮写作要求，不是角色说的话。
请按顺序完成：
1. 结合原玩家输入、修改要求和作品，确定新版发生的事情。
2. 先完成这些事情对应的状态与记忆操作。当前工具环境已经回到本轮开始前，旧版的操作都没有发生；读当前值后，仅执行新版需要的变化。值得记忆的事实写入 /memory，不记录旧版、修改要求或重写过程。准备改写的对话按新版含义记要点，不把参考稿里的旧台词当作已经发生的事实。这一步实际使用原生工具调用，普通文字不会改变记录。“只写正文”“不要选项”等要求只约束可见内容，工具操作仍须完成。
3. 工具返回后，直接输出新版故事正文，沿用作品的叙事方式，并与刚记录的新版事实保持一致。无需确认要求、解释改动或汇报工具结果。本轮各次回复的正文会依次拼接，若前面已经写了部分正文，只补完后续内容，不重复全文。
</system_task>`
	// RewriteContinueNotice 提醒执笔者工具之前输出的正文已经进入当前版本。
	RewriteContinueNotice = `<notes>前面输出的新版正文已经保留。请只补充尚未完成的内容，不重复已经写过的段落；状态和记忆仍需依据新版事实用原生工具维护，以工具结果确认；所有普通回复仍是故事正文，不输出完成确认、处理结果、修改说明或记账汇报。</notes>`
	// RequestChoices 只指定当前操作的产出与副作用边界，不规定选项的剧情方向。
	RequestChoices = `<notes>玩家希望为当前正文的停笔处获得可选择的行动。请结合当前故事和作品设定，直接调用 offer_choices 生成选项，随后等待玩家选择。本次只生成选项，不输出正文、确认语、操作说明或执行计划；不执行选项、不推进故事，也不修改状态和记忆。需要时可以只读查阅资料或当前状态。</notes>`
	// ContinueNotice 截断续写时追加的 user 消息（9.4）。
	ContinueNotice = "<notes>上一段输出到达了单次长度上限，已写的内容都保留着。请从断点处接着写下去。</notes>"
	// ForceChoicesNotice 正文自然结束但尚未交还选项时追加的 user 消息。
	ForceChoicesNotice = "<notes>本回合的正文已写完，内容都保留着。请通过工具调用功能实际调用 offer_choices，将 2–4 个贴合当前情境的选项放入 choices 参数，平台会生成选项按钮。此步骤无需输出正文、选项列表、代码块或 HTML。</notes>"
	// ToolTextRepairNotice 纠正正文中的伪工具标签；%s 是尚待重新调用的工具名。
	ToolTextRepairNotice = "上一条回复把工具（%s）写成了正文 XML，这些操作没有执行；同一条回复中的原生工具调用也均未执行。故事正文已经保留，无需重复。请使用本次 API 提供的原生 function calling 完成这些操作，参数放在工具调用中。不要输出 XML、JSON 代码块或操作说明。"
	// ToolTextRejectedCall 为混有伪调用的消息补齐未执行的原生工具结果。
	ToolTextRejectedCall = "未执行：本条回复混有正文形式的伪工具调用。请根据随后的纠正提示重新发起原生工具调用。"
	// CompactionPendingChoice 整理时为尚未作答的 offer_choices 补齐的工具结果（13.3）。
	CompactionPendingChoice = "（玩家还没有作答，系统先整理上下文）"
	// CompactionToolUnavailable 整理阶段调用了不使用的工具（13.3）。
	CompactionToolUnavailable = "整理阶段暂不使用这个工具。"
	// ChoiceNotAnswered 选项待答时玩家单独输入了命令 / 场外 / 继续：offer_choices 的工具结果（8.3，D-29）。
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
这些操作通过本次请求提供的工具调用功能执行，具体用法见下方；普通回复中的工具名、JSON 或代码块不会执行工具。
请按消息末尾 <system_task> 的要求完成：先把要紧的事写进 /memory，再直接输出交接摘要本身。`

// EpochIntro 是整理后阶段块的首句（13.4）：%d、%d 为保留原文的起止回合。
const EpochIntro = "以下是上下文整理后的交接信息，其后保留了第 %d–%d 回合的原文。"

// EpochIntroNoKept 是没有保留原文时（极少见：阶段内只有整理节点之后的回合）阶段块的首句。
const EpochIntroNoKept = "以下是上下文整理后的交接信息。"

// HelperResearch / HelperCurate 是后台助手提示词（20.4）。运行时按助手能力渲染。
const (
	HelperResearch = `你是这部互动小说执笔者的资料助手。请按任务查阅{{if has "lore"}}/work、{{end}}/memory、/transcript{{if has "vision"}}、/uploads{{end}}，整理成一份简洁的调查结果：每条结论附上出处（文件路径或回合号），并标明哪些是原文明确写到的、哪些是推测。1500 字以内即可。`
	HelperCurate   = `你是这部互动小说执笔者的记忆整理助手。请按任务整理记忆文件：合并重复、补充修订，让每个文件保持简短，并同步更新 /memory/index.md。仍然有用的信息请保留，不确定的内容可以标注"未确认"。完成后用 300 字以内说明改动。`
)

// SaveTitle 是存档标题提示词（20.5）。由运行时在独立上下文中使用。
const SaveTitle = `请根据下面的作品与最新进度，为这个存档起一个便于辨认的标题：16 字以内，点出当前所处的情节或场景，写已经发生的内容。直接输出标题即可。

作品：{{.WorkTitle}}
{{- if .CurrentScene}}
当前场景：{{.CurrentScene}}
{{- end}}
最近进展：
{{.RecentTurns}}`
