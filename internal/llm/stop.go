package llm

// StopReason 是归一化后的停止原因（《02-技术方案》7.2）。
// 各协议到归一值的映射在适配器中完成；具体字段名与取值在 M0 核对。
type StopReason string

const (
	StopEnd             StopReason = "end"              // 自然结束且无工具调用
	StopToolUse         StopReason = "tool_use"         // 需要执行工具
	StopMaxTokens       StopReason = "max_tokens"       // 达到单次输出上限 → 截断续写
	StopPause           StopReason = "pause"            // Anthropic pause_turn
	StopRefusal         StopReason = "refusal"          // 拒答 / 内容过滤
	StopContextOverflow StopReason = "context_overflow" // 上下文超限 → 紧急整理后重试一次
	StopError           StopReason = "error"            // 其他错误
)
