package event

// 事件类型与载荷——这是后端与前端之间的 SSE 契约（《02-技术方案》18.3）。
// 修改此文件时必须同步 web/src/api/events.ts。
//
// 载荷中涉及业务结构的字段（显示项、选项、用量……）用 any 承载，具体结构见注释中指向的类型；
// 这样本包保持在基础层，不反向依赖业务包。

const (
	// 存档主题（SaveTopic）

	TurnStarted   = "turn.started"   // TurnStartedData
	TurnText      = "turn.text"      // TurnTextData：已去重的标记文本增量
	TurnTool      = "turn.tool"      // TurnToolData
	TurnDisplay   = "turn.display"   // TurnDisplayData：骰子卡等展示项
	TurnChoices   = "turn.choices"   // TurnChoicesData
	TurnState     = "turn.state"     // TurnStateData：状态草稿变更
	TurnUsage     = "turn.usage"     // TurnUsageData
	TurnCommitted = "turn.committed" // TurnCommittedData：回合已提交，前端应以时间线接口为准
	TurnFailed    = "turn.failed"    // TurnFailedData
	Panels        = "panels"         // PanelsData

	CompactionStarted = "compaction.started" // CompactionStartedData
	CompactionDone    = "compaction.done"    // CompactionDoneData
	CompactionFailed  = "compaction.failed"  // CompactionFailedData
	HelperProgress    = "helper.progress"    // HelperProgressData

	// 全局主题（GlobalTopic）

	WorkReloaded = "work.reloaded" // WorkReloadedData：开发模式下作品重新加载
	SaveTitled   = "save.titled"   // SaveTitledData：存档标题生成完成

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

type TurnStateData struct {
	TurnID string `json:"turn_id"`
	Patch  any    `json:"patch"` // TODO(M2)：JSON Patch（RFC 6902）
}

type TurnUsageData struct {
	TurnID  string       `json:"turn_id"`
	Usage   any          `json:"usage"` // llm.Usage
	Context ContextUsage `json:"context"`
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
	TurnID      string `json:"turn_id"`
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

type CompactionStartedData struct {
	Reason string `json:"reason"` // threshold | manual | overflow | upgrade
}

type CompactionDoneData struct {
	NodeID string `json:"node_id"`
	Stats  any    `json:"stats"`
}

type CompactionFailedData struct {
	Message string `json:"message"`
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
