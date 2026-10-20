package llm

import "github.com/wahhheee/yanyi/internal/pkg/jsonx"

// Stream 是一次流式调用的结果。Next 在结束时返回 io.EOF；调用方必须 Close。
type Stream interface {
	Next() (Event, error)
	Close() error
}

// Event 是流式事件，具体类型见下方（用 type switch 处理）。
type Event interface{ isEvent() }

// TextDelta 是正文增量。
type TextDelta struct{ Text string }

// ToolCallStart 表示开始一个工具调用（参数尚未完整）。
type ToolCallStart struct{ ID, Name string }

// ToolCallArgsDelta 是工具参数的增量 JSON 片段。
type ToolCallArgsDelta struct{ ID, Delta string }

// ToolCallDone 表示工具调用参数已完整。
type ToolCallDone struct {
	ID, Name string
	Args     jsonx.Value
}

// ReasoningSummaryDelta 是推理摘要增量（可展示给玩家的"思考中"文本，通常不展示）。
type ReasoningSummaryDelta struct{ Text string }

// Done 是流的最后一个事件：停止原因、用量、完整的助手消息（含 Native）。
type Done struct {
	Stop    StopReason
	Usage   Usage
	Message Message
	// Raw 是协议原始的停止原因（调试用，写入节点 meta 与检查器）。
	Raw string
}

func (TextDelta) isEvent()             {}
func (ToolCallStart) isEvent()         {}
func (ToolCallArgsDelta) isEvent()     {}
func (ToolCallDone) isEvent()          {}
func (ReasoningSummaryDelta) isEvent() {}
func (Done) isEvent()                  {}

// EventName 返回事件类型名（日志与调试追踪用）。
func EventName(ev Event) string {
	switch ev.(type) {
	case TextDelta:
		return "text_delta"
	case ToolCallStart:
		return "tool_call_start"
	case ToolCallArgsDelta:
		return "tool_call_args_delta"
	case ToolCallDone:
		return "tool_call_done"
	case ReasoningSummaryDelta:
		return "reasoning_summary_delta"
	case Done:
		return "done"
	default:
		return "unknown"
	}
}
