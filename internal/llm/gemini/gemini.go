// Package gemini 是 Gemini generateContent 协议适配器。
//
// TODO(M0/M4)：正式实现（官方 SDK google.golang.org/genai，SDK 类型只出现在本包）。
// 要点（《02-技术方案》7.4；核对清单第 13–16 项需在 M0 实测确认）：
//   - systemInstruction = L0+L1；tools.functionDeclarations；contents 中 user/model 交替；
//   - 模型输出的 parts（含 thoughtSignature）原样存入 Message.Native 并回传（Gemini 3 缺签名会报错）；
//   - 依赖隐式缓存：保证前缀稳定即可；显式缓存（cachedContents）为 P2；
//   - 函数调用缺少 ID 时生成确定性 ID（同一调用每次映射结果相同，保证前缀稳定）。
package gemini

import (
	"context"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
)

// DefaultBaseURL 是官方地址（可在供应商配置中改为兼容网关）。
const DefaultBaseURL = "https://generativelanguage.googleapis.com"

// Provider 是 Gemini 适配器。
type Provider struct {
	cfg llm.ProviderConfig
}

// New 创建适配器。
func New(cfg llm.ProviderConfig) *Provider { return &Provider{cfg: cfg} }

func (p *Provider) Protocol() llm.Protocol { return llm.ProtocolGemini }

func (p *Provider) Stream(context.Context, *llm.Model, *llm.Request) (llm.Stream, error) {
	return nil, errs.NotImplemented("Gemini 适配器（请先用 --fake-llm 运行）")
}

func (p *Provider) ListModels(context.Context) ([]llm.RemoteModel, error) {
	return nil, errs.NotImplemented("Gemini 模型列表")
}
