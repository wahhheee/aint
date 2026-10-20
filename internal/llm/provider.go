package llm

import (
	"context"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// Protocol 是支持的协议（决策 D-05：不支持 Chat Completions）。
type Protocol string

const (
	ProtocolOpenAIResponses Protocol = "openai-responses"
	ProtocolAnthropic       Protocol = "anthropic"
	ProtocolGemini          Protocol = "gemini"
	ProtocolFake            Protocol = "fake" // 脚本化假模型：测试、演示、--fake-llm
)

// Provider 是一个已配置好的供应商连接。实现必须并发安全。
type Provider interface {
	Protocol() Protocol
	// Stream 发起一次流式调用。返回错误时尚未产生任何输出（可安全重试）。
	Stream(ctx context.Context, m *Model, req *Request) (Stream, error)
	// ListModels 拉取供应商侧的模型列表（协议不支持时返回 errs.NotImplemented）。
	ListModels(ctx context.Context) ([]RemoteModel, error)
}

// ProviderConfig 是构造适配器所需的配置（密钥已从 secrets 解析）。
type ProviderConfig struct {
	ID       string
	Name     string
	Protocol Protocol
	BaseURL  string
	APIKey   string
	Headers  map[string]string
	Options  jsonx.Value
}

// RemoteModel 是供应商返回的模型条目。
type RemoteModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitzero"`
}

// Model 是一个本地配置好的模型（models 表 + 内置预设 + 用户修正）。
type Model struct {
	ID            string `json:"id"`          // 本地配置 ID
	ProviderID    string `json:"provider_id"` // 所属供应商
	Name          string `json:"name"`        // 供应商侧模型 ID
	DisplayName   string `json:"display_name,omitzero"`
	ContextWindow int    `json:"context_window"`
	MaxOutput     int    `json:"max_output"`
	Caps          Caps   `json:"caps"`
	Price         *Price `json:"price,omitzero"`
}

// NativeKey 返回 Message.Native 中属于该模型的键："<protocol>/<model>"。
func NativeKey(p Protocol, model string) string { return string(p) + "/" + model }

// Caps 是能力矩阵（《02-技术方案》7.5）。
// 来源：内置预设 + 连接测试结果 + 用户手动修正。
type Caps struct {
	Vision            bool   `json:"vision"`
	Tools             bool   `json:"tools"`
	ParallelTools     bool   `json:"parallel_tools"`
	ImageInToolResult bool   `json:"image_in_tool_result"`
	Reasoning         string `json:"reasoning"`      // effort | budget | level | none
	Cache             string `json:"cache"`          // breakpoints | key | implicit | none
	ImageMaxEdge      int    `json:"image_max_edge"` // 发送前缩放的长边上限
}
