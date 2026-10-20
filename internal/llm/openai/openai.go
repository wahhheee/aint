// Package openai 是 OpenAI Responses 协议适配器（不支持 Chat Completions，决策 D-05）。
//
// TODO(M0/M4)：正式实现（官方 SDK github.com/openai/openai-go，SDK 类型只出现在本包）。
// 要点（《02-技术方案》7.4；核对清单第 1–6 项需在 M0 实测确认）：
//   - 无状态模式：store=false，include=["reasoning.encrypted_content"]；reasoning 项存入 Message.Native 原样回传；
//   - L0+L1 放 instructions（或首条 developer 消息，M0 比较缓存表现后定）；
//   - prompt_cache_key = "yanyi:" + save_id（Request.CacheKey）；
//   - function_call_output 不支持图片时，把图片作为紧随其后的 user 输入；
//   - incomplete(max_output_tokens) → llm.StopMaxTokens；usage 中 cached_tokens / reasoning_tokens 归一。
package openai

import (
	"context"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
)

// DefaultBaseURL 是官方地址（可在供应商配置中改为兼容网关）。
const DefaultBaseURL = "https://api.openai.com/v1"

// Provider 是 OpenAI Responses 适配器。
type Provider struct {
	cfg llm.ProviderConfig
}

// New 创建适配器。
func New(cfg llm.ProviderConfig) *Provider { return &Provider{cfg: cfg} }

func (p *Provider) Protocol() llm.Protocol { return llm.ProtocolOpenAIResponses }

func (p *Provider) Stream(context.Context, *llm.Model, *llm.Request) (llm.Stream, error) {
	return nil, errs.NotImplemented("OpenAI Responses 适配器（请先用 --fake-llm 运行）")
}

func (p *Provider) ListModels(context.Context) ([]llm.RemoteModel, error) {
	return nil, errs.NotImplemented("OpenAI 模型列表")
}
