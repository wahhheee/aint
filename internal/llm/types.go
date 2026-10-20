package llm

import "github.com/wahhheee/yanyi/internal/pkg/jsonx"

// Role 是消息角色。系统提示放在 Request.System，不作为消息。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// PartKind 是内容块类型。
type PartKind string

const (
	PartText       PartKind = "text"
	PartImage      PartKind = "image"
	PartToolCall   PartKind = "tool_call"
	PartToolResult PartKind = "tool_result"
	PartReasoning  PartKind = "reasoning" // 推理摘要（可读部分）；原始推理数据在 Message.Native
)

// Message 是一条中立消息。以"发送形态"存储：构建请求时只读取不改写（前缀稳定的前提）。
type Message struct {
	Role  Role   `json:"role"`
	Parts []Part `json:"parts"`
	// Native 键为 "<protocol>/<model>"，值为该协议原样输出（含签名、加密推理）。
	Native map[string]jsonx.Value `json:"native,omitzero"`
}

// Part 是消息中的一个内容块（按 Kind 使用对应字段）。
type Part struct {
	Kind       PartKind    `json:"kind"`
	Text       string      `json:"text,omitzero"`
	Image      *ImageRef   `json:"image,omitzero"`
	ToolCall   *ToolCall   `json:"tool_call,omitzero"`
	ToolResult *ToolResult `json:"tool_result,omitzero"`
	// CacheMark 表示在此块之后放缓存断点。不参与存储（每次构建请求时重新计算）。
	CacheMark bool `json:"-"`
}

// ImageRef 引用一张图片（内容在 blob 存储中）。
type ImageRef struct {
	Blob   string `json:"blob"` // 'sha256:<hex>'
	MIME   string `json:"mime"`
	Width  int    `json:"width,omitzero"`
	Height int    `json:"height,omitzero"`
}

// ToolCall 是模型发起的工具调用。
type ToolCall struct {
	ID   string      `json:"id"`
	Name string      `json:"name"`
	Args jsonx.Value `json:"args"`
}

// ToolResult 是工具调用的结果。Parts 只允许 text 与 image。
type ToolResult struct {
	CallID  string `json:"call_id"`
	Parts   []Part `json:"parts"`
	IsError bool   `json:"is_error,omitzero"`
}

// SystemBlock 是一段系统提示文本。
type SystemBlock struct {
	Text      string `json:"text"`
	CacheMark bool   `json:"-"`
}

// ToolSpec 是发给模型的工具定义（中立形态；工具注册表见 tools 包）。
type ToolSpec struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Schema      jsonx.Value `json:"schema"` // 参数的 JSON Schema
}

// ReasoningLevel 是推理强度，由适配器映射到各协议参数（《02-技术方案》7.6）。
type ReasoningLevel string

const (
	ReasoningOff    ReasoningLevel = "off"
	ReasoningLow    ReasoningLevel = "low"
	ReasoningMedium ReasoningLevel = "medium"
	ReasoningHigh   ReasoningLevel = "high"
)

// Request 是一次模型调用的完整输入。
type Request struct {
	System      []SystemBlock  `json:"system"`
	Tools       []ToolSpec     `json:"tools"` // 顺序固定（前缀稳定）
	Messages    []Message      `json:"messages"`
	MaxOutput   int            `json:"max_output"`
	Reasoning   ReasoningLevel `json:"reasoning"`
	Temperature *float64       `json:"temperature,omitzero"`
	CacheKey    string         `json:"cache_key,omitzero"` // OpenAI prompt_cache_key
	CacheTTL    string         `json:"cache_ttl,omitzero"` // Anthropic："5m" | "1h"

	// Purpose 是调用用途（narrate | continue | compact | helper | title | probe），
	// 不发给模型，只用于用量记录、日志与调试追踪。
	Purpose string `json:"purpose"`
}

// Text 是构造文本块的便捷函数。
func Text(s string) Part { return Part{Kind: PartText, Text: s} }

// TextOf 拼接消息中全部文本块（调试、去重、纯文本化时使用）。
func (m Message) TextOf() string {
	var n int
	for _, p := range m.Parts {
		if p.Kind == PartText {
			n += len(p.Text)
		}
	}
	b := make([]byte, 0, n)
	for _, p := range m.Parts {
		if p.Kind == PartText {
			b = append(b, p.Text...)
		}
	}
	return string(b)
}

// ToolCalls 返回消息中的全部工具调用。
func (m Message) ToolCalls() []ToolCall {
	var out []ToolCall
	for _, p := range m.Parts {
		if p.Kind == PartToolCall && p.ToolCall != nil {
			out = append(out, *p.ToolCall)
		}
	}
	return out
}
