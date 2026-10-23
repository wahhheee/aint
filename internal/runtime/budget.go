package runtime

import "time"

// Budget 是每回合的预算（《02-技术方案》9.5）。TODO(M5-10)：存档设置覆盖每回合上限。
// 超预算时优雅停止并提交，界面提示"本回合已达上限"并提供"继续"。
type Budget struct {
	MaxCalls         int           // 最大模型调用数
	MaxToolCalls     int           // 最大工具调用数
	MaxContinuations int           // 截断续写次数
	MaxOutputCap     int           // 单次调用 max_output 上限（实际取 min(模型最大输出, 此值)）
	WallClock        time.Duration // 每回合墙钟时长
}

// DefaultBudget 是默认预算。
var DefaultBudget = Budget{
	MaxCalls: 12, MaxToolCalls: 40, MaxContinuations: 3, MaxOutputCap: 32_000, WallClock: 10 * time.Minute,
}

// exceeded 判断是否超预算，返回原因（空表示未超）。
func (b Budget) exceeded(d *draft) string {
	switch {
	case d.calls >= b.MaxCalls:
		return "已达本回合模型调用上限"
	case d.toolCalls >= b.MaxToolCalls:
		return "已达本回合工具调用上限"
	case time.Since(d.started) >= b.WallClock:
		return "已达本回合时长上限"
	}
	return ""
}
