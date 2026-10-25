package event

// 事件类型与载荷——这是后端与前端之间的 SSE 契约（《02-技术方案》18.3）。
// 修改此文件时必须同步 web/src/api/events.ts。
//
// 载荷中涉及业务结构的字段（显示项、选项、用量……）用 any 承载，具体结构见注释中指向的类型；
// 这样本包保持在基础层，不反向依赖业务包。

const (
	// 存档主题（SaveTopic）
	// SessionSnapshot 只发送给首次连接或无法续传的订阅者，载荷见 runtime.SessionSnapshot。
	SessionSnapshot = "session.snapshot"
	TurnPending     = "turn.pending" // TurnStartedData：输入已接受，正在准备或等待整理

	TurnStarted   = "turn.started"   // TurnStartedData
	TurnText      = "turn.text"      // TurnTextData：已去重的标记文本增量
	TurnTool      = "turn.tool"      // TurnToolData
	TurnDisplay   = "turn.display"   // TurnDisplayData：骰子卡等展示项
	TurnChoices   = "turn.choices"   // TurnChoicesData
	TurnState     = "turn.state"     // TurnStateData：回合内的状态修改（草稿）
	TurnUsage     = "turn.usage"     // TurnUsageData
	TurnCommitted = "turn.committed" // TurnCommittedData：回合已提交，前端应以时间线接口为准
	TurnFailed    = "turn.failed"    // TurnFailedData
	Panels        = "panels"         // PanelsData
	// TimelineChanged：回合以外的操作改了存档（移动 head、修改正文、丢弃、删除分支、编辑记忆或状态），
	// 前端应重新拉取时间线与存档信息。
	TimelineChanged = "timeline.changed" // TimelineChangedData

	CompactionStarted = "compaction.started" // CompactionStartedData
	CompactionDone    = "compaction.done"    // CompactionDoneData
	CompactionFailed  = "compaction.failed"  // CompactionFailedData
	HelperProgress    = "helper.progress"    // HelperProgressData

	// 全局主题（GlobalTopic）

	WorkReloaded    = "work.reloaded"     // WorkReloadedData：开发目录中的作品重新加载
	SaveTitleFailed = "save.title_failed" // SaveTitleFailedData：标题后台任务未完成
	SaveTitled      = "save.titled"       // SaveTitledData：存档标题生成完成

	// Resync 不经过总线，由 SSE 端点在无法补发时直接发出，客户端应重新拉取时间线。
	Resync = "resync"
)

type TurnStartedData struct {
	TurnID   string `json:"turn_id"`
	ParentID string `json:"parent_id"`
	Input    any    `json:"input"` // save.TurnInput
}

type TurnTextData struct {
	TurnID string `json:"turn_id"`
	Seq    int    `json:"seq"` // 回合内递增，前端据此去重与检测丢失
	Delta  string `json:"delta"`
}

type TurnToolData struct {
	TurnID  string `json:"turn_id"`
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	Phase   string `json:"phase"`            // start | done | error
	Display string `json:"display,omitzero"` // tools.DisplayPolicy：hidden | subtle | card | custom
}

type TurnDisplayData struct {
	TurnID string `json:"turn_id"`
	Item   any    `json:"item"` // save.DisplayItem
}

type TurnChoicesData struct {
	TurnID  string `json:"turn_id"`
	Choices any    `json:"choices"` // save.DisplayItem（t=choices）
}

// TurnStateData 是回合内的一次状态修改（update_state 成功后发出；回合提交前为草稿）。
type TurnStateData struct {
	TurnID  string `json:"turn_id"`
	Changes any    `json:"changes"` // []state.Change：每个操作的路径与前后值
}

// TurnUsageData 是本回合到目前为止的累计用量与费用（每次模型调用结束时推送）。
type TurnUsageData struct {
	TurnID string `json:"turn_id"`
	Usage  any    `json:"usage"` // llm.Usage
	// Costs 按货币分别累计（单位为该货币的百万分之一，D-34）；Unpriced 是未配置价格、无法估算费用的调用次数。
	Costs    map[string]int64 `json:"costs"`
	Unpriced int              `json:"unpriced"`
	Context  ContextUsage     `json:"context"`
}

// ContextUsage 是上下文用量指示（顶栏用量环）。
type ContextUsage struct {
	Used      int `json:"used"`
	Threshold int `json:"threshold"`
}

type TurnCommittedData struct {
	TurnID string `json:"turn_id"`
	NodeID string `json:"node_id"`
	Status string `json:"status"` // committed | interrupted
}

type TurnFailedData struct {
	TurnID string `json:"turn_id"`
	// NodeID / ParentID 非空表示已保存中断回合，重试应在其父节点重新生成。
	NodeID      string `json:"node_id,omitzero"`
	ParentID    string `json:"parent_id,omitzero"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
}

type PanelsData struct {
	Panels []PanelRender `json:"panels"`
}

// PanelRender 是一个面板的渲染结果；前端按 Hash 做差量更新。
type PanelRender struct {
	ID   string `json:"id"`
	HTML string `json:"html"`
	Hash string `json:"hash"`
}

type TimelineChangedData struct {
	HeadID string `json:"head_id"`
	Reason string `json:"reason"` // head | rewrite | discard | delete_branch | edit_memory | edit_state | compaction
}

type CompactionStartedData struct {
	Reason string `json:"reason"` // threshold | manual | overflow | upgrade
}

type CompactionDoneData struct {
	NodeID string `json:"node_id"`
	Stats  any    `json:"stats"`
}

type CompactionFailedData struct {
	Message string `json:"message"` // 给玩家的说明；为空表示不需要提示（自动触发但没有可以整理的内容）
}

type HelperProgressData struct {
	TurnID string `json:"turn_id"`
	Kind   string `json:"kind"`  // research | curate
	Phase  string `json:"phase"` // start | done | error
}

type WorkReloadedData struct {
	WorkID string `json:"work_id"`
	Report any    `json:"report"` // work.Report
}

type SaveTitledData struct {
	SaveID string `json:"save_id"`
	Title  string `json:"title"`
}

type SaveTitleFailedData struct {
	SaveID string `json:"save_id"`
}
