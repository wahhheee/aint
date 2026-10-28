// Package gemini 是 Gemini generateContent 协议适配器（标准库实现，D-44）。
//
// 请求（《02-技术方案》7.4）：
//   - POST /v1beta/models/{model}:streamGenerateContent?alt=sse；systemInstruction = L0、L1 两个文本部件；
//     工具用 functionDeclarations + parametersJsonSchema（直接使用作品工具的 JSON Schema）；
//   - safetySettings 显式关闭全部可配置类别的安全过滤；
//   - contents 中 user / model 交替，相邻同角色合并；user 的普通部件在前，functionResponse 在后，各组内部顺序不变；
//   - 同协议同模型的模型消息原样回传 Native 中的部件（含 thoughtSignature；Gemini 3 起工具调用缺签名会报错）；
//     跨模型重建时，每条模型消息的第一个 functionCall 部件带官方文档给出的占位签名 skip_thought_signature_validator；
//   - 工具结果为 functionResponse（按调用 ID 找回工具名）；图片：Caps.ImageInToolResult 时放进 functionResponse.parts，
//     否则作为同一条 user 内容中位于 functionResponse 之前的 inlineData 部件；
//   - 缓存：只依赖隐式缓存（前缀稳定即可），CacheMark 忽略。
//
// 推理档位映射（Caps.ReasoningMap 的值）：omit = 不发 thinkingConfig；minimal / low / medium / high = thinkingLevel；
// budget:N = thinkingBudget（Gemini 2.5，0 表示关闭）。缺省：off→low（3.8 / 4 系列最低档为 low），其余同名。
//
// 函数调用 ID：响应带 ID 时原样使用；缺少时按响应 ID 与部件位置生成 gsyn_ 前缀的 ID，
// 响应 ID 也缺失时使用本次调用的独立命名空间。合成 ID 随消息存储，原生回传时不添加 ID。
package gemini

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

// DefaultBaseURL 是官方地址（可在供应商配置中改为兼容网关）。
const DefaultBaseURL = "https://generativelanguage.googleapis.com"

// skipSignature 是官方文档给出的占位签名：把其他模型的历史（或注入的函数调用）交给 Gemini 3 时使用。
const skipSignature = "skip_thought_signature_validator"

// synthPrefix 是中立层为缺少 ID 的函数调用生成的 ID 前缀。
const synthPrefix = "gsyn_"

// Provider 是 Gemini 适配器。
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
	auth.Set("X-Goog-Api-Key", cfg.APIKey)
	ep := wire.NewEndpoint(llm.ProtocolGemini, wire.TrimVersion(base, "v1beta", "v1"), auth, cfg.Headers, cfg.HTTP, cfg.TraceDir)
	ep.Classify = classify
	return &Provider{cfg: cfg, ep: ep}
}

func (p *Provider) Protocol() llm.Protocol { return llm.ProtocolGemini }

// Stream 发起一次流式调用。
func (p *Provider) Stream(ctx context.Context, m *llm.Model, req *llm.Request) (llm.Stream, error) {
	body, err := p.Body(ctx, m, req)
	if err != nil {
		return nil, err
	}
	path := "/v1beta/models/" + wire.PathEscape(m.Name) + ":streamGenerateContent?alt=sse"
	rc, err := p.ep.PostStream(ctx, path, body, wire.Call{Purpose: req.Purpose, Model: m.Name})
	if err != nil {
		return nil, err
	}
	return newStream(ctx, rc, llm.NativeKey(llm.ProtocolGemini, m.Name)), nil
}

// ListModels 拉取模型列表（GET /v1beta/models）。
func (p *Provider) ListModels(ctx context.Context) ([]llm.RemoteModel, error) {
	var out struct {
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
	}
	if err := p.ep.GetJSON(ctx, "/v1beta/models?pageSize=1000", &out); err != nil {
		return nil, err
	}
	models := make([]llm.RemoteModel, 0, len(out.Models))
	for _, m := range out.Models {
		models = append(models, llm.RemoteModel{ID: strings.TrimPrefix(m.Name, "models/"), DisplayName: m.DisplayName})
	}
	return models, nil
}

// ---------------------------------------------------------------------------
// 请求

type request struct {
	SystemInstruction *content        `json:"systemInstruction,omitzero"`
	Contents          []content       `json:"contents"`
	Tools             []toolGroup     `json:"tools,omitzero"`
	SafetySettings    []safetySetting `json:"safetySettings"`
	GenerationConfig  *generation     `json:"generationConfig,omitzero"`
}

type safetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type content struct {
	Role  string        `json:"role,omitzero"`
	Parts []jsonx.Value `json:"parts"`
}

type toolGroup struct {
	FunctionDeclarations []function `json:"functionDeclarations"`
}

type function struct {
	Name                 string      `json:"name"`
	Description          string      `json:"description,omitzero"`
	ParametersJSONSchema jsonx.Value `json:"parametersJsonSchema,omitzero"`
}

type generation struct {
	MaxOutputTokens int             `json:"maxOutputTokens,omitzero"`
	Temperature     *float64        `json:"temperature,omitzero"`
	ThinkingConfig  *thinkingConfig `json:"thinkingConfig,omitzero"`
}

type thinkingConfig struct {
	ThinkingLevel  string `json:"thinkingLevel,omitzero"`
	ThinkingBudget *int   `json:"thinkingBudget,omitzero"`
}

type textPart struct {
	Text             string `json:"text"`
	Thought          bool   `json:"thought,omitzero"`
	ThoughtSignature string `json:"thoughtSignature,omitzero"`
}

type inlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type dataPart struct {
	InlineData inlineData `json:"inlineData"`
}

type callPart struct {
	FunctionCall     functionCall `json:"functionCall"`
	ThoughtSignature string       `json:"thoughtSignature,omitzero"`
}

type functionCall struct {
	ID   string      `json:"id,omitzero"`
	Name string      `json:"name"`
	Args jsonx.Value `json:"args"`
}

type responsePart struct {
	FunctionResponse functionResponse `json:"functionResponse"`
}

type functionResponse struct {
	ID       string        `json:"id,omitzero"`
	Name     string        `json:"name"`
	Response any           `json:"response"`
	Parts    []jsonx.Value `json:"parts,omitzero"`
}

// Body 构建请求体（确定性 JSON）。导出供测试与前缀比对使用。
func (p *Provider) Body(ctx context.Context, m *llm.Model, req *llm.Request) ([]byte, error) {
	key := llm.NativeKey(llm.ProtocolGemini, m.Name)
	out := request{SafetySettings: []safetySetting{
		{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "OFF"},
		{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "OFF"},
		{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "OFF"},
		{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "OFF"},
		{Category: "HARM_CATEGORY_CIVIC_INTEGRITY", Threshold: "OFF"},
		{Category: "HARM_CATEGORY_JAILBREAK", Threshold: "OFF"},
	}}
	var sys content
	for _, s := range req.System {
		if s.Text != "" {
			sys.Parts = append(sys.Parts, jsonx.MustMarshal(textPart{Text: s.Text}))
		}
	}
	if len(sys.Parts) > 0 {
		out.SystemInstruction = &sys
	}
	if len(req.Tools) > 0 {
		g := toolGroup{}
		for _, t := range req.Tools {
			g.FunctionDeclarations = append(g.FunctionDeclarations, function{Name: t.Name, Description: t.Description, ParametersJSONSchema: t.Schema})
		}
		out.Tools = []toolGroup{g}
	}

	names := map[string]string{} // 调用 ID → 工具名（functionResponse 需要）
	for _, msg := range llm.RepairToolHistory(req.Messages) {
		for _, c := range msg.ToolCalls() {
			names[c.ID] = c.Name
		}
		parts, err := p.parts(ctx, key, msg, names, m.Caps.ImageInToolResult)
		if err != nil {
			return nil, err
		}
		if len(parts) == 0 {
			continue
		}
		role := "user"
		if msg.Role == llm.RoleAssistant {
			role = "model"
		}
		if n := len(out.Contents); n > 0 && out.Contents[n-1].Role == role {
			out.Contents[n-1].Parts = append(out.Contents[n-1].Parts, parts...)
			continue
		}
		out.Contents = append(out.Contents, content{Role: role, Parts: parts})
	}
	for i := range out.Contents {
		c := &out.Contents[i]
		if c.Role == "user" {
			if err := c.orderToolResults(); err != nil {
				return nil, err
			}
		}
	}

	gen := &generation{MaxOutputTokens: req.MaxOutput, Temperature: req.Temperature, ThinkingConfig: thinking(m.Caps, req.Reasoning)}
	if gen.MaxOutputTokens > 0 || gen.Temperature != nil || gen.ThinkingConfig != nil {
		out.GenerationConfig = gen
	}
	return jsonx.Marshal(out)
}

// orderToolResults 在同角色消息合并后排列用户部件。部分 Gemini 接入拒绝工具结果后附带文字的请求，
// 因此普通文字与图片在前，工具结果在后；各组内部顺序和工具结果内嵌图片保持不变。
func (c *content) orderToolResults() error {
	var ordinary, responses []jsonx.Value
	for _, raw := range c.Parts {
		var p struct {
			FunctionResponse jsonx.Value `json:"functionResponse"`
		}
		if err := jsonx.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("用户消息的内容损坏: %w", err)
		}
		if len(p.FunctionResponse) > 0 {
			responses = append(responses, raw)
		} else {
			ordinary = append(ordinary, raw)
		}
	}
	parts := make([]jsonx.Value, 0, len(ordinary)+len(responses))
	parts = append(parts, ordinary...)
	parts = append(parts, responses...)
	c.Parts = parts
	return nil
}

// parts 转换一条消息的内容。
func (p *Provider) parts(ctx context.Context, key string, msg llm.Message, names map[string]string, imageInResult bool) ([]jsonx.Value, error) {
	if raw, ok := msg.Native[key]; ok && msg.Role == llm.RoleAssistant {
		var parts []jsonx.Value
		if err := jsonx.Unmarshal(raw, &parts); err != nil {
			return nil, fmt.Errorf("模型消息的原生内容损坏: %w", err)
		}
		return parts, nil
	}
	var out []jsonx.Value
	firstCall := true
	for _, part := range msg.Parts {
		switch part.Kind {
		case llm.PartText:
			if part.Text != "" {
				out = append(out, jsonx.MustMarshal(textPart{Text: part.Text}))
			}
		case llm.PartImage:
			b, err := p.image(ctx, part.Image)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
		case llm.PartToolCall:
			c := callPart{FunctionCall: functionCall{Name: part.ToolCall.Name, Args: argsOrEmpty(part.ToolCall.Args)}}
			if !strings.HasPrefix(part.ToolCall.ID, synthPrefix) {
				c.FunctionCall.ID = part.ToolCall.ID
			}
			if firstCall { // 跨模型历史：第一个函数调用带占位签名
				c.ThoughtSignature = skipSignature
				firstCall = false
			}
			out = append(out, jsonx.MustMarshal(c))
		case llm.PartToolResult:
			parts, err := p.toolResult(ctx, part.ToolResult, names, imageInResult)
			if err != nil {
				return nil, err
			}
			out = append(out, parts...)
		case llm.PartReasoning:
			// 跨模型时推理丢弃
		default:
			return nil, fmt.Errorf("未知的内容块类型 %q", part.Kind)
		}
	}
	return out, nil
}

func (p *Provider) image(ctx context.Context, ref *llm.ImageRef) (jsonx.Value, error) {
	data, mime, err := wire.ImageData(ctx, p.cfg.Blobs, ref)
	if err != nil {
		return nil, err
	}
	return jsonx.Marshal(dataPart{InlineData: inlineData{MIMEType: mime, Data: data}})
}

// toolResult 生成 functionResponse；图片按能力内嵌或独立发送，独立部件由 Body 在合并用户消息后排序。
func (p *Provider) toolResult(ctx context.Context, tr *llm.ToolResult, names map[string]string, imageInResult bool) ([]jsonx.Value, error) {
	var texts []string
	var images []jsonx.Value
	for _, sub := range tr.Parts {
		switch sub.Kind {
		case llm.PartText:
			texts = append(texts, sub.Text)
		case llm.PartImage:
			b, err := p.image(ctx, sub.Image)
			if err != nil {
				return nil, err
			}
			images = append(images, b)
		}
	}
	text := strings.Join(texts, "\n")
	fr := functionResponse{Name: names[tr.CallID], Response: map[string]string{"output": text}}
	if tr.IsError {
		fr.Response = map[string]string{"error": text}
	}
	if fr.Name == "" {
		fr.Name = "unknown"
	}
	if !strings.HasPrefix(tr.CallID, synthPrefix) {
		fr.ID = tr.CallID
	}
	if imageInResult {
		fr.Parts = images
		return []jsonx.Value{jsonx.MustMarshal(responsePart{FunctionResponse: fr})}, nil
	}
	return append([]jsonx.Value{jsonx.MustMarshal(responsePart{FunctionResponse: fr})}, images...), nil
}

func argsOrEmpty(v jsonx.Value) jsonx.Value {
	if len(strings.TrimSpace(string(v))) == 0 {
		return jsonx.Value(`{}`)
	}
	return v
}

// thinking 把推理档位映射为 thinkingConfig。
func thinking(caps llm.Caps, level llm.ReasoningLevel) *thinkingConfig {
	if caps.Reasoning == "none" || caps.Reasoning == "" || level == "" {
		return nil
	}
	v, ok := caps.ReasoningFor(level)
	if !ok {
		v = string(level)
		if level == llm.ReasoningOff {
			v = "low"
		}
	}
	if v == "omit" {
		return nil
	}
	if n, ok := strings.CutPrefix(v, "budget:"); ok {
		b, err := strconv.Atoi(n)
		if err != nil {
			return nil
		}
		return &thinkingConfig{ThinkingBudget: &b}
	}
	return &thinkingConfig{ThinkingLevel: v}
}

// classify 细化错误种类：上下文超限。
func classify(status int, body []byte, msg string) llm.ErrorKind {
	low := strings.ToLower(msg + " " + string(body))
	if strings.Contains(low, "exceeds the maximum number of tokens") || strings.Contains(low, "context window") {
		return llm.ErrContextOverflow
	}
	return ""
}
