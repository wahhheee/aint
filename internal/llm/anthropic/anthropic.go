// Package anthropic 是 Anthropic Messages 协议适配器。
//
// TODO(M0/M4)：正式实现（官方 SDK github.com/anthropics/anthropic-sdk-go，SDK 类型只出现在本包）。
// 要点（《02-技术方案》7.4；核对清单第 7–12 项需在 M0 实测确认）：
//   - 顺序：tools → system → messages；L0+L1 为 system 文本块；
//   - 缓存断点最多 4 个：①L3 末尾 ②L4 末尾 ③最新消息末尾（滚动）④必要时上次请求末尾；TTL 5m/1h；
//   - thinking 块（含签名）存入 Message.Native，工具循环中原样回传；开启 thinking 时不使用 assistant 预填；
//   - tool_result 可含图片块；
//   - stop_reason → llm.StopReason；usage → llm.Usage（cache_creation 区分 5m/1h）；
//   - 错误：HTTP 状态 → llm.APIError（529 过载可重试）。
package anthropic

import (
	"context"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
)

// DefaultBaseURL 是官方地址（可在供应商配置中改为兼容网关）。
const DefaultBaseURL = "https://api.anthropic.com"

// Provider 是 Anthropic 适配器。
type Provider struct {
	cfg llm.ProviderConfig
}

// New 创建适配器。
func New(cfg llm.ProviderConfig) *Provider { return &Provider{cfg: cfg} }

func (p *Provider) Protocol() llm.Protocol { return llm.ProtocolAnthropic }

func (p *Provider) Stream(context.Context, *llm.Model, *llm.Request) (llm.Stream, error) {
	return nil, errs.NotImplemented("Anthropic Messages 适配器（请先用 --fake-llm 运行）")
}

func (p *Provider) ListModels(context.Context) ([]llm.RemoteModel, error) {
	return nil, errs.NotImplemented("Anthropic 模型列表")
}
