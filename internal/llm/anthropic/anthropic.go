// Package anthropic 是 Anthropic Messages 协议适配器（标准库实现，D-44）。
//
// 请求（《02-技术方案》7.4）：
//   - 顺序 tools → system → messages；L0、L1 为两个 system 文本块；
//   - 缓存断点（Caps.Cache = "breakpoints" 时）：中立请求中带 CacheMark 的块（L1 末尾、L3 末尾、L4 末尾）
//     加最后一个可缓存块（滚动断点）；新增内容达到 20 个位置时保留前一次 user 末尾的历史锚点，
//     必要时让较晚的静态断点让位，合计最多 4 个；TTL 取 Request.CacheTTL（short 5m / long 1h）；
//   - 同协议同模型的助手消息原样回传 Native 中的内容块（thinking 块的签名不能改动）；否则用中立内容重建，推理丢弃；
//   - 相邻同角色消息合并为一条；空文本块省略（API 不接受空 text）。
//
// 推理档位映射（Caps.ReasoningMap 的值，缺省见 defaultThinking）：
//
//	omit               不发 thinking 与 effort（模型默认）
//	disabled           thinking {type: disabled}
//	between_tools[:E]  thinking {type: between_tools}（Claude Sonnet 5.5 关闭思考的写法），可带 effort
//	adaptive[:E]       thinking {type: adaptive} + output_config.effort = E
//	effort:E           只发 output_config.effort
//	budget:N           thinking {type: enabled, budget_tokens: N}（旧模型）
//
// 响应：thinking / redacted_thinking / text / tool_use 块按顺序存入 Native（规范化后的请求形态），
// stop_reason 与 usage 归一（usage.output_tokens_details.thinking_tokens 记入 Reasoning）。
package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/wire"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// DefaultBaseURL 是官方地址（可在供应商配置中改为兼容网关，如 https://api.deepseek.com/anthropic）。
const DefaultBaseURL = "https://api.anthropic.com"

const apiVersion = "2023-06-01"

// maxBreakpoints 是单个请求的缓存断点上限。
const maxBreakpoints = 4

// Provider 是 Anthropic 适配器。
type Provider struct {
	cfg llm.ProviderConfig
	ep  *wire.Endpoint
}

// New 创建适配器。
func New(cfg llm.ProviderConfig) *Provider {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	auth := http.Header{}
	auth.Set("X-Api-Key", cfg.APIKey)
	auth.Set("Anthropic-Version", apiVersion)
	ep := wire.NewEndpoint(llm.ProtocolAnthropic, wire.TrimVersion(base, "v1"), auth, cfg.Headers, cfg.HTTP, cfg.TraceDir)
	ep.Classify = classify
	return &Provider{cfg: cfg, ep: ep}
}

func (p *Provider) Protocol() llm.Protocol { return llm.ProtocolAnthropic }

// Stream 发起一次流式调用。
func (p *Provider) Stream(ctx context.Context, m *llm.Model, req *llm.Request) (llm.Stream, error) {
	body, err := p.Body(ctx, m, req)
	if err != nil {
		return nil, err
	}
	rc, err := p.ep.PostStream(ctx, "/v1/messages", body, wire.Call{Purpose: req.Purpose, Model: m.Name})
	if err != nil {
		return nil, err
	}
	return newStream(ctx, rc, llm.NativeKey(llm.ProtocolAnthropic, m.Name)), nil
}

// ListModels 拉取模型列表（GET /v1/models）。
func (p *Provider) ListModels(ctx context.Context) ([]llm.RemoteModel, error) {
	var out struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := p.ep.GetJSON(ctx, "/v1/models?limit=1000", &out); err != nil {
		return nil, err
	}
	models := make([]llm.RemoteModel, 0, len(out.Data))
	for _, d := range out.Data {
		models = append(models, llm.RemoteModel{ID: d.ID, DisplayName: d.DisplayName})
	}
	return models, nil
}

// ---------------------------------------------------------------------------
// 请求

type request struct {
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	Tools        []tool        `json:"tools,omitzero"`
	System       []jsonx.Value `json:"system,omitzero"`
	Messages     []message     `json:"messages"`
	Thinking     *thinking     `json:"thinking,omitzero"`
	OutputConfig *outputConfig `json:"output_config,omitzero"`
	Temperature  *float64      `json:"temperature,omitzero"`
	Stream       bool          `json:"stream"`
}

type tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitzero"`
	InputSchema jsonx.Value `json:"input_schema"`
}

type message struct {
	Role    string        `json:"role"`
	Content []jsonx.Value `json:"content"`
}

type thinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitzero"`
}

type outputConfig struct {
	Effort string `json:"effort,omitzero"`
}

type cacheControl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitzero"`
}

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type imageBlock struct {
	Type   string      `json:"type"`
	Source imageSource `json:"source"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type toolUseBlock struct {
	Type  string      `json:"type"`
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Input jsonx.Value `json:"input"`
}

type toolResultBlock struct {
	Type      string        `json:"type"`
	ToolUseID string        `json:"tool_use_id"`
	Content   []jsonx.Value `json:"content,omitzero"`
	IsError   bool          `json:"is_error,omitzero"`
}

// Body 构建请求体（确定性 JSON）。导出供测试与前缀比对使用。
func (p *Provider) Body(ctx context.Context, m *llm.Model, req *llm.Request) ([]byte, error) {
	key := llm.NativeKey(llm.ProtocolAnthropic, m.Name)
	caching := m.Caps.Cache == "breakpoints"
	cc := cacheControl{Type: "ephemeral"}
	if req.CacheTTL == llm.CacheLong {
		cc.TTL = "1h"
	}

	out := request{Model: m.Name, MaxTokens: req.MaxOutput, Temperature: req.Temperature, Stream: true}
	if out.MaxTokens <= 0 {
		out.MaxTokens = 8192
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, tool{Name: t.Name, Description: t.Description, InputSchema: schemaOrEmpty(t.Schema)})
	}

	marks := 0
	for _, s := range req.System {
		if s.Text == "" {
			continue
		}
		b := jsonx.MustMarshal(textBlock{Type: "text", Text: s.Text})
		if caching && s.CacheMark && marks < maxBreakpoints-1 { // 留一个给滚动断点
			b = withCache(b, cc)
			marks++
		}
		out.System = append(out.System, b)
	}

	msgs, err := p.messages(ctx, key, llm.RepairToolHistory(req.Messages), caching, cc, &marks)
	if err != nil {
		return nil, err
	}
	out.Messages = msgs
	if caching && len(msgs) > 0 {
		markRolling(msgs, cc)
		anchorCache(&out, cc)
	}
	out.Thinking, out.OutputConfig = reasoning(m.Caps, req.Reasoning, out.MaxTokens)
	return jsonx.Marshal(out)
}

// messages 转换消息：同协议同模型的助手消息用 Native，其余按中立内容重建；相邻同角色合并。
func (p *Provider) messages(ctx context.Context, key string, in []llm.Message, caching bool, cc cacheControl, marks *int) ([]message, error) {
	var out []message
	for _, m := range in {
		var blocks []jsonx.Value
		if raw, ok := m.Native[key]; ok && m.Role == llm.RoleAssistant {
			if err := jsonx.Unmarshal(raw, &blocks); err != nil {
				return nil, fmt.Errorf("助手消息的原生内容损坏: %w", err)
			}
		} else {
			for _, part := range m.Parts {
				b, err := p.block(ctx, part)
				if err != nil {
					return nil, err
				}
				if b == nil {
					continue
				}
				// 滚动断点固定占一个名额，中立标记最多用剩下的。
				if caching && part.CacheMark && *marks < maxBreakpoints-1 {
					b = withCache(b, cc)
					*marks++
				}
				blocks = append(blocks, b)
			}
		}
		if len(blocks) == 0 {
			continue
		}
		role := string(m.Role)
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content = append(out[n-1].Content, blocks...)
			continue
		}
		out = append(out, message{Role: role, Content: blocks})
	}
	return out, nil
}

// block 把一个中立内容块转为协议内容块；返回 nil 表示省略（空文本、推理摘要）。
func (p *Provider) block(ctx context.Context, part llm.Part) (jsonx.Value, error) {
	switch part.Kind {
	case llm.PartText:
		if part.Text == "" {
			return nil, nil
		}
		return jsonx.Marshal(textBlock{Type: "text", Text: part.Text})
	case llm.PartImage:
		data, mime, err := wire.ImageData(ctx, p.cfg.Blobs, part.Image)
		if err != nil {
			return nil, err
		}
		return jsonx.Marshal(imageBlock{Type: "image", Source: imageSource{Type: "base64", MediaType: mime, Data: data}})
	case llm.PartToolCall:
		tc := part.ToolCall
		return jsonx.Marshal(toolUseBlock{Type: "tool_use", ID: wire.ToolID(tc.ID), Name: tc.Name, Input: argsOrEmpty(tc.Args)})
	case llm.PartToolResult:
		tr := part.ToolResult
		res := toolResultBlock{Type: "tool_result", ToolUseID: wire.ToolID(tr.CallID), IsError: tr.IsError}
		for _, sub := range tr.Parts {
			b, err := p.block(ctx, sub)
			if err != nil {
				return nil, err
			}
			if b != nil {
				res.Content = append(res.Content, b)
			}
		}
		return jsonx.Marshal(res)
	case llm.PartReasoning:
		return nil, nil // 跨供应商时推理内容丢弃（同模型走 Native）
	}
	return nil, fmt.Errorf("未知的内容块类型 %q", part.Kind)
}

// withCache 在 JSON 对象末尾追加 cache_control 字段（不重排已有字段，保持字节稳定）。
func withCache(obj jsonx.Value, cc cacheControl) jsonx.Value {
	s := strings.TrimSpace(string(obj))
	if !strings.HasSuffix(s, "}") {
		return obj
	}
	member := `"cache_control":` + string(jsonx.MustMarshal(cc))
	if s == "{}" {
		return jsonx.Value("{" + member + "}")
	}
	return jsonx.Value(s[:len(s)-1] + "," + member + "}")
}

func hasCache(obj jsonx.Value) bool { return strings.Contains(string(obj), `"cache_control":`) }

func schemaOrEmpty(v jsonx.Value) jsonx.Value {
	if len(v) == 0 {
		return jsonx.Value(`{"type":"object","properties":{}}`)
	}
	return v
}

func argsOrEmpty(v jsonx.Value) jsonx.Value {
	if len(strings.TrimSpace(string(v))) == 0 {
		return jsonx.Value(`{}`)
	}
	return v
}

// reasoning 把推理档位映射为 thinking 与 output_config。
func reasoning(caps llm.Caps, level llm.ReasoningLevel, maxTokens int) (*thinking, *outputConfig) {
	if caps.Reasoning == "none" || level == "" {
		return nil, nil
	}
	v, ok := caps.ReasoningFor(level)
	if !ok {
		v = defaultThinking(caps.Reasoning, level)
	}
	kind, arg, _ := strings.Cut(v, ":")
	effort := func() *outputConfig {
		if arg == "" {
			return nil
		}
		return &outputConfig{Effort: arg}
	}
	switch kind {
	case "disabled":
		return &thinking{Type: "disabled"}, nil
	case "between_tools":
		return &thinking{Type: "between_tools"}, effort()
	case "adaptive":
		return &thinking{Type: "adaptive"}, effort()
	case "effort":
		return nil, effort()
	case "budget":
		n, err := strconv.Atoi(arg)
		if err != nil || n <= 0 {
			return nil, nil
		}
		n = min(n, maxTokens-1) // budget 必须小于 max_tokens
		if n < 1024 {
			return &thinking{Type: "disabled"}, nil
		}
		return &thinking{Type: "enabled", BudgetTokens: n}, nil
	}
	return nil, nil // omit 或无法识别：不发推理参数
}

// defaultThinking 是 ReasoningMap 未配置时的默认映射。
func defaultThinking(style string, level llm.ReasoningLevel) string {
	switch style {
	case "effort": // Claude 4.6 起：adaptive + effort；关闭时退到 low
		if level == llm.ReasoningOff {
			return "adaptive:low"
		}
		return "adaptive:" + string(level)
	case "budget":
		switch level {
		case llm.ReasoningLow:
			return "budget:2048"
		case llm.ReasoningMedium:
			return "budget:8192"
		case llm.ReasoningHigh:
			return "budget:16384"
		}
		return "disabled"
	}
	return "omit"
}

// classify 细化错误种类：上下文超限、过载。
func classify(status int, body []byte, msg string) llm.ErrorKind {
	low := strings.ToLower(msg + " " + string(body))
	switch {
	case strings.Contains(low, "overloaded"):
		return llm.ErrOverloaded
	case strings.Contains(low, "prompt is too long"), strings.Contains(low, "context window"),
		strings.Contains(low, "context_length"), strings.Contains(low, "too many tokens"):
		return llm.ErrContextOverflow
	}
	return ""
}
