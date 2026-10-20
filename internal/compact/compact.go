// Package compact 是上下文整理（压缩）：记忆落盘 + 交接摘要 + 新阶段块（《02-技术方案》第 13 节）。
//
// 流程（inline 模式，默认）：
//
//	当前完整上下文（命中缓存）+ 整理指令（prompts.CompactionTask）
//	  → 落盘循环（≤ FlushMaxCalls 轮 read/list/search/write/edit/remove）
//	  → 交接摘要（校验非空与长度，超长要求压缩一次）
//	  → 提交 compaction 节点（阶段块 + keep_from + 新记忆清单）
//	  → 新阶段：L0–L3 不变（命中缓存），L4 换成新阶段块 + 保留原文
//
// 触发点：回合提交后超过阈值（玩家阅读期间）、回合内达到硬上限、API 报上下文超限（紧急，重试一次）、手动。
// 整理期间 offer_choices / update_state / roll_dice 返回 prompts.CompactionToolUnavailable（工具定义不变）。
package compact

import (
	"context"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
)

// Params 是整理参数（13.1）；默认值为初始值，需用示例作品实测后调整。
type Params struct {
	ReserveMin    int     // reserve = max_output + max(ReserveMin, 窗口 × ReserveRatio)
	ReserveRatio  float64 //
	BudgetCap     int     // 工作上下文预算上限（阈值 T = min(窗口 − reserve, BudgetCap)）
	KeepRecent    int     // 保留原文 token，至少最近 2 个回合，至多 T × 30%
	SummaryTarget int     // 交接摘要目标 token（硬上限 SummaryMax）
	SummaryMax    int
	FlushMaxCalls int    // 落盘阶段最多工具调用轮数
	Mode          string // inline | separate
}

// DefaultParams 是默认整理参数。
var DefaultParams = Params{
	ReserveMin: 8_000, ReserveRatio: 0.05, BudgetCap: 128_000, KeepRecent: 12_000,
	SummaryTarget: 3_000, SummaryMax: 6_000, FlushMaxCalls: 8, Mode: "inline",
}

// Threshold 计算整理阈值 T。
func (p Params) Threshold(window, maxOutput int) int {
	reserve := maxOutput + max(p.ReserveMin, int(float64(window)*p.ReserveRatio))
	return min(window-reserve, p.BudgetCap)
}

// Reason 是整理原因（compaction.started 事件）。
type Reason string

const (
	ReasonThreshold Reason = "threshold"
	ReasonManual    Reason = "manual"
	ReasonOverflow  Reason = "overflow"
	ReasonUpgrade   Reason = "upgrade"
)

// Compactor 执行上下文整理。
//
// TODO(M3-6/M3-7)：实现 inline 与 separate 两种模式、keep_from 计算、待回答选项处理、失败降级。
type Compactor struct{}

// Run 对存档执行一次整理。
func (c *Compactor) Run(ctx context.Context, saveID string, reason Reason) error {
	return errs.NotImplemented("上下文整理")
}
