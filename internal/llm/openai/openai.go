// Package openai 是 OpenAI Responses 协议适配器（不支持 Chat Completions，D-05；标准库实现，D-44）。
//
// 请求（《02-技术方案》7.4）：
//   - 无状态：store=false；开启推理时 include=["reasoning.encrypted_content"]，返回的 reasoning 项随 Native 原样回传；
//   - 同协议同模型的助手消息原样回传 Native 中的输出项（reasoning / message / function_call，含 phase 等字段）；
//     否则按中立内容重建（output_text、function_call），推理丢弃；
//   - 工具结果为 function_call_output；带图片时 output 用内容列表（input_text / input_image）；
//   - 缓存（Caps.Cache）：
//     breakpoints —— GPT-5.6 起：prompt_cache_options {mode: implicit, ttl: 30m|12h}，L0+L1 放进首条 developer 消息
//     （顶层 instructions 不能带断点），带 CacheMark 的块（L1、L3、L4 末尾）加 prompt_cache_breakpoint；
//     隐式断点自动落在最新一条可缓存消息末尾，相当于滚动断点；
//     implicit —— L0+L1 放 instructions，只发 prompt_cache_key（早期模型、DeepSeek 等兼容接口会忽略不认识的字段）；
//     none —— 不发缓存参数。
//
// 推理档位映射（Caps.ReasoningMap 的值）：omit = 不发 reasoning；其他值原样作为 reasoning.effort
// （none / minimal / low / medium / high / xhigh，取决于模型）。缺省：off→minimal，其余同名。
package openai

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/wire"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// DefaultBaseURL 是官方地址（含 /v1，与官方 SDK 的 base_url 写法一致；兼容接口按其文档填写，如 https://api.deepseek.com）。
const DefaultBaseURL = "https://api.openai.com/v1"

// Provider 是 OpenAI Responses 适配器。
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
	auth.Set("Authorization", "Bearer "+cfg.APIKey)
	ep := wire.NewEndpoint(llm.ProtocolOpenAIResponses, base, auth, cfg.Headers, cfg.HTTP, cfg.TraceDir)
	ep.Classify = classify
	return &Provider{cfg: cfg, ep: ep}
}

func (p *Provider) Protocol() llm.Protocol { return llm.ProtocolOpenAIResponses }

// Stream 发起一次流式调用（POST /responses）。
func (p *Provider) Stream(ctx context.Context, m *llm.Model, req *llm.Request) (llm.Stream, error) {
	body, err := p.Body(ctx, m, req)
	if err != nil {
		return nil, err
	}
	rc, err := p.ep.PostStream(ctx, "/responses", body, wire.Call{Purpose: req.Purpose, Model: m.Name})
	if err != nil {
		return nil, err
	}
	return newStream(ctx, rc, llm.NativeKey(llm.ProtocolOpenAIResponses, m.Name)), nil
}

// ListModels 拉取模型列表（GET /models）。
func (p *Provider) ListModels(ctx context.Context) ([]llm.RemoteModel, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := p.ep.GetJSON(ctx, "/models", &out); err != nil {
		return nil, err
	}
	models := make([]llm.RemoteModel, 0, len(out.Data))
	for _, d := range out.Data {
		models = append(models, llm.RemoteModel{ID: d.ID})
	}
	return models, nil
}

// ---------------------------------------------------------------------------
// 请求

type request struct {
	Model              string        `json:"model"`
	Instructions       string        `json:"instructions,omitzero"`
	Input              []jsonx.Value `json:"input"`
	Tools              []tool        `json:"tools,omitzero"`
	MaxOutputTokens    int           `json:"max_output_tokens,omitzero"`
	Reasoning          *reasoning    `json:"reasoning,omitzero"`
	Include            []string      `json:"include,omitzero"`
	Temperature        *float64      `json:"temperature,omitzero"`
	PromptCacheKey     string        `json:"prompt_cache_key,omitzero"`
	PromptCacheOptions *cacheOptions `json:"prompt_cache_options,omitzero"`
	Store              bool          `json:"store"` // 始终显式发送 false（无状态）
	Stream             bool          `json:"stream"`
}

type tool struct {
	Type        string      `json:"type"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitzero"`
	Parameters  jsonx.Value `json:"parameters"`
	Strict      bool        `json:"strict"` // 显式 false：作品工具的 schema 有可选字段，不满足严格模式
}

type reasoning struct {
	Effort string `json:"effort"`
}

type cacheOptions struct {
	Mode string `json:"mode"`
	TTL  string `json:"ttl,omitzero"`
}

type breakpoint struct {
	Mode string `json:"mode"`
}

type message struct {
	Role    string        `json:"role"`
	Content []jsonx.Value `json:"content"`
}

type inputText struct {
	Type       string      `json:"type"`
	Text       string      `json:"text"`
	Breakpoint *breakpoint `json:"prompt_cache_breakpoint,omitzero"`
}

type inputImage struct {
	Type     string `json:"type"`
	ImageURL string `json:"image_url"`
}

type outputText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type functionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type functionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output any    `json:"output"` // 字符串，或内容列表（带图片时）
}

// Body 构建请求体（确定性 JSON）。导出供测试与前缀比对使用。
func (p *Provider) Body(ctx context.Context, m *llm.Model, req *llm.Request) ([]byte, error) {
	key := llm.NativeKey(llm.ProtocolOpenAIResponses, m.Name)
	breakpoints := m.Caps.Cache == "breakpoints"

	out := request{Model: m.Name, MaxOutputTokens: req.MaxOutput, Temperature: req.Temperature, Stream: true}
	for _, t := range req.Tools {
		params := t.Schema
		if len(params) == 0 {
			params = jsonx.Value(`{"type":"object","properties":{}}`)
		}
		out.Tools = append(out.Tools, tool{Type: "function", Name: t.Name, Description: t.Description, Parameters: params})
	}
	switch m.Caps.Cache {
	case "breakpoints":
		out.PromptCacheKey = req.CacheKey
		out.PromptCacheOptions = &cacheOptions{Mode: "implicit", TTL: "30m"}
		if req.CacheTTL == llm.CacheLong {
			out.PromptCacheOptions.TTL = "12h"
		}
	case "implicit":
		out.PromptCacheKey = req.CacheKey
	}

	// 系统提示：断点模式放进首条 developer 消息（可以带断点），否则放 instructions。
	if breakpoints {
		var dev message
		for _, s := range req.System {
			if s.Text == "" {
				continue
			}
			b := inputText{Type: "input_text", Text: s.Text}
			if s.CacheMark {
				b.Breakpoint = &breakpoint{Mode: "explicit"}
			}
			dev.Content = append(dev.Content, jsonx.MustMarshal(b))
		}
		if len(dev.Content) > 0 {
			dev.Role = "developer"
			out.Input = append(out.Input, jsonx.MustMarshal(dev))
		}
	} else {
		var parts []string
		for _, s := range req.System {
			if s.Text != "" {
				parts = append(parts, s.Text)
			}
		}
		out.Instructions = strings.Join(parts, "\n\n")
	}

	for _, msg := range req.Messages {
		items, err := p.items(ctx, key, msg, breakpoints)
		if err != nil {
			return nil, err
		}
		out.Input = append(out.Input, items...)
	}

	if r := effort(m.Caps, req.Reasoning); r != "" {
		out.Reasoning = &reasoning{Effort: r}
		out.Include = []string{"reasoning.encrypted_content"}
	}
	return jsonx.Marshal(out)
}

// items 把一条中立消息转为若干输入项。
func (p *Provider) items(ctx context.Context, key string, msg llm.Message, breakpoints bool) ([]jsonx.Value, error) {
	if raw, ok := msg.Native[key]; ok && msg.Role == llm.RoleAssistant {
		var items []jsonx.Value
		if err := jsonx.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("助手消息的原生内容损坏: %w", err)
		}
		return items, nil
	}

	var out []jsonx.Value
	var content []jsonx.Value // 当前累积的消息内容
	role := "user"
	if msg.Role == llm.RoleAssistant {
		role = "assistant"
	}
	flush := func() {
		if len(content) > 0 {
			out = append(out, jsonx.MustMarshal(message{Role: role, Content: content}))
			content = nil
		}
	}
	for _, part := range msg.Parts {
		switch part.Kind {
		case llm.PartText:
			if part.Text == "" {
				continue
			}
			if role == "assistant" {
				content = append(content, jsonx.MustMarshal(outputText{Type: "output_text", Text: part.Text}))
				continue
			}
			b := inputText{Type: "input_text", Text: part.Text}
			if breakpoints && part.CacheMark {
				b.Breakpoint = &breakpoint{Mode: "explicit"}
			}
			content = append(content, jsonx.MustMarshal(b))
		case llm.PartImage:
			img, err := p.image(ctx, part.Image)
			if err != nil {
				return nil, err
			}
			content = append(content, img)
		case llm.PartToolCall:
			flush()
			args := strings.TrimSpace(string(part.ToolCall.Args))
			if args == "" {
				args = "{}"
			}
			out = append(out, jsonx.MustMarshal(functionCall{Type: "function_call", CallID: wire.ToolID(part.ToolCall.ID), Name: part.ToolCall.Name, Arguments: args}))
		case llm.PartToolResult:
			flush()
			item, err := p.toolOutput(ctx, part.ToolResult)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		case llm.PartReasoning:
			// 跨模型时推理丢弃（同模型走 Native）
		default:
			return nil, fmt.Errorf("未知的内容块类型 %q", part.Kind)
		}
	}
	flush()
	return out, nil
}

func (p *Provider) image(ctx context.Context, ref *llm.ImageRef) (jsonx.Value, error) {
	data, mime, err := wire.ImageData(ctx, p.cfg.Blobs, ref)
	if err != nil {
		return nil, err
	}
	return jsonx.Marshal(inputImage{Type: "input_image", ImageURL: "data:" + mime + ";base64," + data})
}

// toolOutput 生成 function_call_output：纯文本用字符串，带图片时用内容列表。
func (p *Provider) toolOutput(ctx context.Context, tr *llm.ToolResult) (jsonx.Value, error) {
	item := functionCallOutput{Type: "function_call_output", CallID: wire.ToolID(tr.CallID)}
	hasImage := false
	for _, sub := range tr.Parts {
		if sub.Kind == llm.PartImage {
			hasImage = true
		}
	}
	if !hasImage {
		var texts []string
		for _, sub := range tr.Parts {
			if sub.Kind == llm.PartText {
				texts = append(texts, sub.Text)
			}
		}
		item.Output = strings.Join(texts, "\n")
		return jsonx.Marshal(item)
	}
	var list []jsonx.Value
	for _, sub := range tr.Parts {
		switch sub.Kind {
		case llm.PartText:
			list = append(list, jsonx.MustMarshal(inputText{Type: "input_text", Text: sub.Text}))
		case llm.PartImage:
			img, err := p.image(ctx, sub.Image)
			if err != nil {
				return nil, err
			}
			list = append(list, img)
		}
	}
	item.Output = list
	return jsonx.Marshal(item)
}

// effort 把推理档位映射为 reasoning.effort；返回 "" 表示不发 reasoning。
func effort(caps llm.Caps, level llm.ReasoningLevel) string {
	if caps.Reasoning == "none" || caps.Reasoning == "" || level == "" {
		return ""
	}
	v, ok := caps.ReasoningFor(level)
	if !ok {
		v = string(level)
		if level == llm.ReasoningOff {
			v = "minimal"
		}
	}
	if v == "omit" {
		return ""
	}
	return v
}

// classify 细化错误种类：上下文超限、余额不足。
func classify(status int, body []byte, msg string) llm.ErrorKind {
	low := strings.ToLower(msg + " " + string(body))
	switch {
	case strings.Contains(low, "context_length_exceeded"), strings.Contains(low, "maximum context length"),
		strings.Contains(low, "context window"):
		return llm.ErrContextOverflow
	case strings.Contains(low, "insufficient_quota"):
		return llm.ErrAuth
	}
	return ""
}
