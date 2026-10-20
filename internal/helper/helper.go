// Package helper 是后台助手：主笔通过 delegate 工具把检索、整理等幕后工作交给它（《02-技术方案》第 14 节）。
//
//   - 默认关闭；玩家在全局或存档设置中开启；使用玩家配置的轻量模型（未配置时不可开启）；
//   - research：工具 read/list/search；curate：另加 /memory 的 write/edit/remove；
//   - 上下文隔离：只拿到助手提示词（prompts.HelperResearch / HelperCurate）与任务文本，看不到主笔对话；
//   - 预算：最多 10 次调用、单次输出 4k、总时长 3 分钟；返回 ≤ 1,500 字带出处的简报；
//   - curate 的写操作进入当前回合的记忆覆盖层，随回合一起提交或作废；
//   - 助手不写正文、不改状态、不与玩家交互（决策 D-12）。
//
// TODO(M5-8)：实现（复用 runtime 的调用循环与 tools 注册表的子集）。
package helper

import (
	"context"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
)

// Kind 是助手类型。
type Kind string

const (
	Research Kind = "research"
	Curate   Kind = "curate"
)

// Report 是助手返回给主笔的简报。
type Report struct {
	Text string
}

// Run 执行一次委托任务。
func Run(ctx context.Context, kind Kind, task string) (*Report, error) {
	return nil, errs.NotImplemented("后台助手")
}
