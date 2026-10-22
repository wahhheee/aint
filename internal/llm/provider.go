package llm

import (
	"context"
	"net/http"

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

	// Blobs 读取图片内容（ImageRef.Blob）；为空时请求中出现图片会报错。
	Blobs BlobReader
	// HTTP 为空时使用默认客户端（测试中可注入 httptest 的客户端）。
	HTTP *http.Client
	// TraceDir 非空时，适配器把协议原始请求与响应（脱敏）写到该目录（--trace-llm）。
	TraceDir string
}

// BlobReader 按内容哈希读取 blob（图片）。由 store 实现，适配器构建请求时调用。
type BlobReader interface {
	ReadBlob(ctx context.Context, hash string) ([]byte, error)
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
	// Price 是模型单价（运行时拿到的是已乘以供应商价格倍率的实际单价，D-48）。
	Price *Price `json:"price,omitzero"`
}

// NativeKey 返回 Message.Native 中属于该模型的键："<protocol>/<model>"。
func NativeKey(p Protocol, model string) string { return string(p) + "/" + model }

// Caps 是能力矩阵（《02-技术方案》7.5）。
// 来源：内置预设 + 连接测试结果 + 用户手动修正。
type Caps struct {
	Vision            bool `json:"vision"`
	Tools             bool `json:"tools"`
	ParallelTools     bool `json:"parallel_tools"`
	ImageInToolResult bool `json:"image_in_tool_result"`
	// Reasoning 是推理的控制方式：effort | budget | level | none（none 表示从不发送推理参数）。
	Reasoning string `json:"reasoning"`
	// ReasoningMap 把存档的推理档位（off | low | medium | high）映射到协议参数，写法由各适配器定义
	// （见 llm/anthropic、llm/openai、llm/gemini 包注释）。缺省的档位使用适配器的默认映射。
	// 同一协议不同模型的差异（如 Claude Sonnet 5.5 只能用 between_tools 关闭思考、DeepSeek 没有 medium 档）都写在这里。
	ReasoningMap map[string]string `json:"reasoning_map,omitzero"`
	// Cache 是缓存方式：
	//   breakpoints（显式断点：Anthropic cache_control；OpenAI GPT-5.6 起的 prompt_cache_breakpoint 与 prompt_cache_options）、
	//   implicit（供应商自动缓存：OpenAI 早期模型 / Gemini 隐式缓存 / DeepSeek 硬盘缓存；OpenAI 会附带 prompt_cache_key）、
	//   none（不发任何缓存参数）。
	Cache        string `json:"cache"`
	ImageMaxEdge int    `json:"image_max_edge"` // 发送前缩放的长边上限
}

// ReasoningFor 返回推理档位在该模型上的协议参数（ReasoningMap 中的值；未配置时为 ""，由适配器决定默认）。
func (c Caps) ReasoningFor(level ReasoningLevel) (string, bool) {
	v, ok := c.ReasoningMap[string(level)]
	return v, ok
}
