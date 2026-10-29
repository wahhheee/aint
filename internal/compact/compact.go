// Package compact 是上下文整理（压缩）的参数与规则：阈值、保留原文的起点、交接摘要的校验（《02-技术方案》第 13 节）。
// 整理的执行（请求、落盘循环、提交 compaction 节点）在 runtime 中，与回合共用会话、工具与 VFS。
//
// 流程（inline 模式，默认）：
//
//	当前完整上下文（命中缓存）+ 整理指令（prompts.CompactionTask）
//	  → 落盘循环（≤ FlushMaxCalls 轮 read/list/search/write/edit/remove）
//	  → 交接摘要（校验非空与长度，超长要求压缩一次）
//	  → 提交 compaction 节点（阶段块 + keep_from + 新记忆清单）
//	  → 新阶段：L0–L3 不变（命中缓存），L4 换成新阶段块 + 保留原文
//
// separate 模式用轻量模型单独整理：输入是上一份摘要、本阶段各回合的原文、当前状态与记忆目录，不复用主笔的缓存。
//
// 触发点：回合提交后上下文达到阈值（玩家阅读期间）、回合内达到硬上限、API 报上下文超限（紧急，重试一次）、手动。
// 整理期间 offer_choices / update_state / roll_dice 返回 prompts.CompactionToolUnavailable（工具定义不变）。
package compact

// Params 是整理参数（13.1）。
type Params struct {
	ReserveMin    int     // reserve = max_output + max(ReserveMin, 窗口 × ReserveRatio)
	ReserveRatio  float64 //
	Budget        int     // 工作上下文预算（阈值 T = min(窗口 − reserve, Budget)）
	KeepRecent    int     // 保留原文 token，至少最近 2 个回合，至多 T × 30%
	SummaryTarget int     // 交接摘要目标（token，约等于汉字数），不超过 T × 10%
	SummaryMax    int     // 交接摘要的硬上限（token）：超过时要求压缩一次
	FlushMaxCalls int     // 落盘阶段最多的模型调用次数
	Mode          string  // inline | separate
}

// 整理方式。
const (
	ModeInline   = "inline"
	ModeSeparate = "separate"
)

// DefaultParams 是默认整理参数。
var DefaultParams = Params{
	ReserveMin: 8_000, ReserveRatio: 0.05, Budget: 256_000, KeepRecent: 24_000,
	SummaryTarget: 3_000, SummaryMax: 6_000, FlushMaxCalls: 8, Mode: ModeInline,
}

// With 返回覆盖了预算、保留量与整理方式的参数；零值表示沿用原值。
func (p Params) With(budget, keepRecent int, mode string) Params {
	if budget > 0 {
		p.Budget = budget
	}
	if keepRecent > 0 {
		p.KeepRecent = keepRecent
	}
	if mode == ModeInline || mode == ModeSeparate {
		p.Mode = mode
	}
	return p
}

// reserve 是为输出与估算误差预留的 token。
func (p Params) reserve(window, maxOutput int) int {
	return maxOutput + max(p.ReserveMin, int(float64(window)*p.ReserveRatio))
}

// Threshold 计算整理阈值 T = min(窗口 − reserve, 预算)。回合提交后上下文达到 T 即开始整理。
func (p Params) Threshold(window, maxOutput int) int {
	return max(min(p.HardLimit(window, maxOutput), p.Budget), 1)
}

// HardLimit 是硬上限（窗口 − reserve）：回合进行中下一次调用的上下文估算达到它时，先在回合内整理。
func (p Params) HardLimit(window, maxOutput int) int {
	return max(window-p.reserve(window, maxOutput), 1)
}

// KeepTokens 是整理后保留原文的 token 目标：KeepRecent，且不超过 T × 30%。
func (p Params) KeepTokens(threshold int) int {
	return min(p.KeepRecent, threshold*30/100)
}

// SummaryChars 是交接摘要的目标长度（字）：SummaryTarget，且不超过 T × 10%。
func (p Params) SummaryChars(threshold int) int {
	return max(min(p.SummaryTarget, threshold/10), 300)
}

// KeepFrom 选出保留原文的起点。sizes 是当前上下文中各回合的 token 估算（按时间顺序），
// 从最后一个回合向前累计，达到 keep 为止（以回合为界，不切分回合），且至少保留最后 2 个回合（不足 2 个时全部保留）。
// 返回第一个保留回合的下标；返回 0 表示全部保留（没有可以摘要掉的回合）。
func KeepFrom(sizes []int, keep int) int {
	i, total := len(sizes), 0
	for i > 0 {
		if len(sizes)-i >= 2 && total >= keep {
			break
		}
		i--
		total += sizes[i]
	}
	return i
}

// SummaryProblem 检查交接摘要：返回空表示可用，否则是问题的说明（"摘要是空的"、"偏长"）。
// tokens 是摘要的 token 估算。
func (p Params) SummaryProblem(summary string, tokens int) string {
	switch {
	case len(summary) == 0:
		return "empty"
	case tokens > p.SummaryMax:
		return "long"
	}
	return ""
}

// Reason 是整理原因（compaction.started 事件、整理节点 meta）。
type Reason string

const (
	ReasonThreshold Reason = "threshold" // 回合提交后上下文达到阈值
	ReasonLimit     Reason = "limit"     // 回合进行中达到硬上限
	ReasonOverflow  Reason = "overflow"  // 模型报上下文超限
	ReasonManual    Reason = "manual"    // 玩家手动（按钮或 /compact）
	ReasonUpgrade   Reason = "upgrade"   // 存档升级到作品新版本（M6-6）
)
