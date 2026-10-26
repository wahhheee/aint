// Package helper 是后台助手：主笔通过 delegate 工具把检索、整理等幕后工作交给它（《02-技术方案》第 14 节）。
//
//   - 默认关闭；玩家在全局或存档设置中开启；使用玩家配置的轻量模型（未配置时不可开启）；
//   - research：工具 read/list/search；curate：另加 /memory 的 write/edit/remove；
//   - 上下文隔离：只拿到助手提示词（prompts.HelperResearch / HelperCurate）与任务文本，看不到主笔对话；
//   - 预算：最多 10 次调用、单次输出 4k、总时长 3 分钟；返回 ≤ 1,500 字带出处的简报；
//   - curate 的写操作进入当前回合的记忆覆盖层，随回合一起提交或作废；
//   - 助手不写正文、不改状态、不与玩家交互（决策 D-12）。
package helper

import (
	"context"
	"strings"
	"time"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
)

// Kind 是助手类型。
type Kind string

const (
	Research Kind = "research"
	Curate   Kind = "curate"
)

// Run 在隔离上下文中执行有界工具循环。调用与工具执行由运行时提供，便于统一记录用量与调试信息。
func Run(ctx context.Context, req *llm.Request, call func(context.Context, *llm.Request) (llm.Message, llm.StopReason, error), execute func(context.Context, []llm.ToolCall) llm.Message) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for range 10 {
		msg, stop, err := call(ctx, req)
		if err != nil {
			return "", err
		}
		req.Messages = append(req.Messages, msg)
		if calls := msg.ToolCalls(); len(calls) > 0 {
			if len(calls) > 20 {
				return "", errs.Invalid("助手一次请求的工具过多")
			}
			req.Messages = append(req.Messages, execute(ctx, calls))
			continue
		}
		if stop == llm.StopMaxTokens {
			return "", errs.Invalid("助手简报超出输出上限，请缩小任务范围")
		}
		var text strings.Builder
		for _, part := range msg.Parts {
			if part.Kind == llm.PartText {
				text.WriteString(part.Text)
			}
		}
		result := []rune(strings.TrimSpace(text.String()))
		if len(result) == 0 {
			return "", errs.Invalid("助手未返回简报")
		}
		if len(result) > 1500 {
			result = append(result[:1500], []rune("（简报已截断）")...)
		}
		return string(result), nil
	}
	return "", errs.Invalid("助手已达调用上限，请缩小任务范围")
}
