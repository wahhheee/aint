package runtime

import (
	"errors"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
)

var errTurnTimeout = errors.New("本回合已达时长上限，可以继续或重试。")

// describeError 把错误归类为面向玩家的提示（《04-交互与界面设计》8.9、16）：
// 说明发生了什么、玩家能做什么，不暴露技术细节（细节在日志与节点 meta 中）。
func describeError(err error) (code, message string, recoverable bool) {
	if errors.Is(err, errTurnTimeout) {
		return "budget", errTurnTimeout.Error(), true
	}
	if e, ok := errs.As(err); ok {
		return string(e.Code), e.Message, e.Code != errs.CodeInvalidArgument
	}
	switch llm.KindOf(err) {
	case llm.ErrRateLimited, llm.ErrOverloaded:
		return "rate_limited", "模型服务繁忙（限流），已自动重试 3 次。", true
	case llm.ErrAuth:
		return "auth", "模型服务拒绝了请求：鉴权失败或余额不足。", true
	case llm.ErrNetwork:
		return "network", "连接模型服务失败，请检查网络后重试。", true
	case llm.ErrServer:
		return "server", "模型服务暂时出错，请稍后重试。", true
	case llm.ErrContextOverflow:
		return "context_overflow", "上下文超过了模型上限，已尝试整理。", true
	case llm.ErrUnsupported:
		return "unsupported", "当前模型不支持这项能力。", false
	case llm.ErrInvalidRequest:
		return "invalid_request", "请求被模型服务拒绝（可能是适配器问题，详见日志）。", true
	}
	return string(errs.CodeInternal), "回合运行出错（详见日志）。", true
}
