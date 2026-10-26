package anthropic

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/wire"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// stream 把 Messages 流式事件转为中立事件。
type stream struct {
	ctx   context.Context
	body  io.ReadCloser
	sse   *wire.SSE
	key   string
	queue wire.Queue
	done  bool

	blocks []*blockState // 按 index
	usage  usageState
	stop   string
	detail string // refusal 的类别等
}

type blockState struct {
	typ       string // text | thinking | redacted_thinking | tool_use | 其他原生块
	text      strings.Builder
	signature strings.Builder
	data      string // redacted_thinking
	id, name  string
	args      strings.Builder
	closed    bool
	raw       jsonx.Value // 原始块的完整字段；服务端工具等非中立内容随 Native 回传
}

type usageState struct {
	input, cacheRead, cacheWrite, output, thinking int
}

func newStream(ctx context.Context, body io.ReadCloser, key string) *stream {
	return &stream{ctx: ctx, body: body, sse: wire.NewSSE(body), key: key}
}

func (s *stream) Close() error { return s.body.Close() }

func (s *stream) Next() (event llm.Event, err error) {
	defer func() {
		if err != nil && !errors.Is(err, io.EOF) {
			err = llm.WithPartialUsage(err, s.usage.normalize())
		}
	}()
	for {
		if ev, ok := s.queue.Pop(); ok {
			return ev, nil
		}
		if s.done {
			return nil, io.EOF
		}
		ev, err := s.sse.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, wire.StreamError(s.ctx, wire.ErrNoDone)
			}
			return nil, wire.StreamError(s.ctx, err)
		}
		if err := s.handle(ev); err != nil {
			return nil, err
		}
	}
}

// 协议事件的公共形态（只解码用到的字段）。
type event struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message *struct {
		Usage *usage `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		Name      string `json:"name"`
		Data      string `json:"data"`
		Text      string `json:"text"`
		Thinking  string `json:"thinking"`
		Signature string `json:"signature"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
		StopDetails *struct {
			Category    string `json:"category"`
			Explanation string `json:"explanation"`
		} `json:"stop_details"`
	} `json:"delta"`
	Usage *usage `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type usage struct {
	InputTokens              *int `json:"input_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	OutputTokensDetails      *struct {
		ThinkingTokens int `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
}

func (s *stream) handle(raw wire.Event) error {
	if len(raw.Data) == 0 {
		return nil
	}
	var ev event
	if err := jsonx.Unmarshal(raw.Data, &ev); err != nil {
		return &llm.APIError{Kind: llm.ErrUnknown, Message: "无法解析流式事件：" + string(raw.Data), Err: err}
	}
	switch ev.Type {
	case "message_start":
		if ev.Message != nil {
			s.usage.merge(ev.Message.Usage)
		}
	case "content_block_start":
		if ev.ContentBlock == nil {
			return nil
		}
		b := s.block(ev.Index)
		b.typ = ev.ContentBlock.Type
		var fields map[string]jsonx.Value
		if err := jsonx.Unmarshal(raw.Data, &fields); err != nil {
			return err
		}
		b.raw = fields["content_block"]
		switch b.typ {
		case "tool_use":
			b.id, b.name = ev.ContentBlock.ID, ev.ContentBlock.Name
			s.queue.Push(llm.ToolCallStart{ID: b.id, Name: b.name})
		case "thinking":
			b.text.WriteString(ev.ContentBlock.Thinking)
			b.signature.WriteString(ev.ContentBlock.Signature)
			if b.text.Len() > 0 {
				s.queue.Push(llm.ReasoningSummaryDelta{Text: ev.ContentBlock.Thinking})
			}
		case "redacted_thinking":
			b.data = ev.ContentBlock.Data
		case "text":
			if ev.ContentBlock.Text != "" { // 一般为空；个别网关会把首段放在这里
				b.text.WriteString(ev.ContentBlock.Text)
				s.queue.Push(llm.TextDelta{Text: ev.ContentBlock.Text})
			}
		}
	case "content_block_delta":
		if ev.Delta == nil {
			return nil
		}
		b := s.block(ev.Index)
		switch ev.Delta.Type {
		case "text_delta":
			if ev.Delta.Text != "" {
				b.text.WriteString(ev.Delta.Text)
				s.queue.Push(llm.TextDelta{Text: ev.Delta.Text})
			}
		case "thinking_delta":
			if ev.Delta.Thinking != "" {
				b.text.WriteString(ev.Delta.Thinking)
				s.queue.Push(llm.ReasoningSummaryDelta{Text: ev.Delta.Thinking})
			}
		case "signature_delta":
			b.signature.WriteString(ev.Delta.Signature)
		case "input_json_delta":
			if ev.Delta.PartialJSON != "" {
				b.args.WriteString(ev.Delta.PartialJSON)
				if b.typ == "tool_use" {
					s.queue.Push(llm.ToolCallArgsDelta{ID: b.id, Delta: ev.Delta.PartialJSON})
				}
			}
		}
	case "content_block_stop":
		b := s.block(ev.Index)
		b.closed = true
		if b.typ == "tool_use" {
			s.queue.Push(llm.ToolCallDone{ID: b.id, Name: b.name, Args: b.argsJSON()})
		}
	case "message_delta":
		if ev.Delta != nil && ev.Delta.StopReason != "" {
			s.stop = ev.Delta.StopReason
			if d := ev.Delta.StopDetails; d != nil && d.Category != "" {
				s.detail = d.Category
			}
		}
		s.usage.merge(ev.Usage)
	case "message_stop":
		s.done = true
		s.queue.Push(s.finish())
	case "error":
		kind, msg := llm.ErrUnknown, string(raw.Data)
		if ev.Error != nil {
			msg = ev.Error.Message
			switch ev.Error.Type {
			case "overloaded_error":
				kind = llm.ErrOverloaded
			case "rate_limit_error":
				kind = llm.ErrRateLimited
			case "api_error":
				kind = llm.ErrServer
			case "invalid_request_error":
				kind = llm.ErrInvalidRequest
			}
			if k := classify(0, nil, msg); k != "" {
				kind = k
			}
		}
		return &llm.APIError{Kind: kind, Message: msg}
	}
	return nil
}

func (s *stream) block(i int) *blockState {
	for len(s.blocks) <= i {
		s.blocks = append(s.blocks, &blockState{})
	}
	return s.blocks[i]
}

func (b *blockState) argsJSON() jsonx.Value {
	a := strings.TrimSpace(b.args.String())
	if a == "" {
		var initial struct {
			Input jsonx.Value `json:"input"`
		}
		if jsonx.Unmarshal(b.raw, &initial) == nil && len(initial.Input) > 0 {
			return initial.Input
		}
		return jsonx.Value(`{}`)
	}
	v := jsonx.Value(a)
	if !v.IsValid() {
		return jsonx.MustMarshal(a)
	}
	return v
}

func (u *usageState) merge(x *usage) {
	if x == nil {
		return
	}
	set := func(dst *int, v *int) {
		if v != nil {
			*dst = *v
		}
	}
	set(&u.input, x.InputTokens)
	set(&u.cacheWrite, x.CacheCreationInputTokens)
	set(&u.cacheRead, x.CacheReadInputTokens)
	set(&u.output, x.OutputTokens)
	if x.OutputTokensDetails != nil {
		u.thinking = x.OutputTokensDetails.ThinkingTokens
	}
}

func (u usageState) normalize() llm.Usage {
	return llm.Usage{
		InputUncached: u.input,
		CacheRead:     u.cacheRead,
		CacheWrite:    u.cacheWrite,
		Output:        max(u.output-u.thinking, 0),
		Reasoning:     u.thinking,
	}
}

// finish 组装 Done：中立消息 + Native（规范化后的请求形态内容块）。
func (s *stream) finish() llm.Done {
	msg := llm.Message{Role: llm.RoleAssistant}
	var native []jsonx.Value
	hasTool := false
	for _, b := range s.blocks {
		switch b.typ {
		case "text":
			if b.text.Len() == 0 {
				continue
			}
			msg.Parts = append(msg.Parts, llm.Text(b.text.String()))
			native = append(native, jsonx.MustMarshal(textBlock{Type: "text", Text: b.text.String()}))
		case "thinking":
			if b.text.Len() > 0 {
				msg.Parts = append(msg.Parts, llm.Part{Kind: llm.PartReasoning, Text: b.text.String()})
			}
			native = append(native, jsonx.MustMarshal(struct {
				Type      string `json:"type"`
				Thinking  string `json:"thinking"`
				Signature string `json:"signature"`
			}{"thinking", b.text.String(), b.signature.String()}))
		case "redacted_thinking":
			native = append(native, jsonx.MustMarshal(struct {
				Type string `json:"type"`
				Data string `json:"data"`
			}{"redacted_thinking", b.data}))
		case "tool_use":
			if !b.closed || s.stop == "max_tokens" {
				continue // 截断时的工具调用尚未执行，不进入下一次请求
			}
			hasTool = true
			args := b.argsJSON()
			msg.Parts = append(msg.Parts, llm.Part{Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{ID: b.id, Name: b.name, Args: args}})
			native = append(native, jsonx.MustMarshal(toolUseBlock{Type: "tool_use", ID: b.id, Name: b.name, Input: args}))
		default:
			if len(b.raw) > 0 {
				if b.args.Len() > 0 {
					var fields map[string]jsonx.Value
					if jsonx.Unmarshal(b.raw, &fields) == nil {
						fields["input"] = b.argsJSON()
						b.raw = jsonx.MustMarshal(fields)
					}
				}
				native = append(native, b.raw)
			}
		}
	}
	if len(native) > 0 {
		msg.Native = map[string]jsonx.Value{s.key: jsonx.MustMarshal(native)}
	}
	raw := s.stop
	if s.detail != "" {
		raw += ":" + s.detail
	}
	return llm.Done{Stop: stopReason(s.stop, hasTool), Usage: s.usage.normalize(), Message: msg, Raw: raw}
}

func stopReason(r string, hasTool bool) llm.StopReason {
	switch r {
	case "tool_use":
		return llm.StopToolUse
	case "end_turn", "stop_sequence":
		if hasTool {
			return llm.StopToolUse
		}
		return llm.StopEnd
	case "max_tokens":
		return llm.StopMaxTokens
	case "pause_turn":
		return llm.StopPause
	case "refusal":
		return llm.StopRefusal
	case "model_context_window_exceeded":
		return llm.StopContextOverflow
	case "":
		return llm.StopError
	}
	if hasTool {
		return llm.StopToolUse
	}
	return llm.StopEnd
}
