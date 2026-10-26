package openai

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/wire"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// stream 把 Responses 流式事件转为中立事件。
type stream struct {
	ctx   context.Context
	body  io.ReadCloser
	sse   *wire.SSE
	key   string
	queue wire.Queue
	done  bool

	items map[int]*itemState // output_index → 输出项
}

type itemState struct {
	typ    string
	callID string
	name   string
	raw    jsonx.Value // output_item.done 中的完整输出项
}

func newStream(ctx context.Context, body io.ReadCloser, key string) *stream {
	return &stream{ctx: ctx, body: body, sse: wire.NewSSE(body), key: key, items: map[int]*itemState{}}
}

func (s *stream) Close() error { return s.body.Close() }

func (s *stream) Next() (llm.Event, error) {
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
		if len(ev.Data) == 0 || wire.IsDone(ev) {
			continue
		}
		if err := s.handle(ev.Data); err != nil {
			return nil, err
		}
	}
}

type event struct {
	Type        string      `json:"type"`
	OutputIndex int         `json:"output_index"`
	Delta       string      `json:"delta"`
	Item        jsonx.Value `json:"item"`
	Response    *response   `json:"response"`
	// error 事件
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	Status            string        `json:"status"`
	Output            []jsonx.Value `json:"output"`
	Usage             *usage        `json:"usage"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type usage struct {
	InputTokens        int `json:"input_tokens"`
	InputTokensDetails struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
	} `json:"input_tokens_details"`
	OutputTokens        int `json:"output_tokens"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// item 是输出项中用到的字段。
type item struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Refusal string `json:"refusal"`
	} `json:"content"`
	Summary []struct {
		Text string `json:"text"`
	} `json:"summary"`
}

func (s *stream) handle(data []byte) error {
	var ev event
	if err := jsonx.Unmarshal(data, &ev); err != nil {
		return &llm.APIError{Kind: llm.ErrUnknown, Message: "无法解析流式事件：" + string(data), Err: err}
	}
	switch ev.Type {
	case "response.output_item.added":
		var it item
		if err := jsonx.Unmarshal(ev.Item, &it); err != nil {
			return nil //nolint:nilerr // 只用于提前发出 ToolCallStart；完整内容以 output_item.done 为准
		}
		st := &itemState{typ: it.Type, callID: it.CallID, name: it.Name}
		s.items[ev.OutputIndex] = st
		if it.Type == "function_call" {
			s.queue.Push(llm.ToolCallStart{ID: it.CallID, Name: it.Name})
		}
	case "response.output_text.delta":
		if ev.Delta != "" {
			s.queue.Push(llm.TextDelta{Text: ev.Delta})
		}
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		if ev.Delta != "" {
			s.queue.Push(llm.ReasoningSummaryDelta{Text: ev.Delta})
		}
	case "response.function_call_arguments.delta":
		if st := s.items[ev.OutputIndex]; st != nil && ev.Delta != "" {
			s.queue.Push(llm.ToolCallArgsDelta{ID: st.callID, Delta: ev.Delta})
		}
	case "response.output_item.done":
		var it item
		if err := jsonx.Unmarshal(ev.Item, &it); err != nil {
			return &llm.APIError{Kind: llm.ErrUnknown, Message: "无法解析输出项：" + string(ev.Item), Err: err}
		}
		st := s.items[ev.OutputIndex]
		if st == nil {
			st = &itemState{}
			s.items[ev.OutputIndex] = st
		}
		st.typ, st.callID, st.name, st.raw = it.Type, it.CallID, it.Name, ev.Item
		if it.Type == "function_call" {
			s.queue.Push(llm.ToolCallDone{ID: it.CallID, Name: it.Name, Args: argsJSON(it.Arguments)})
		}
	case "response.completed", "response.incomplete":
		s.done = true
		s.queue.Push(s.finish(ev.Response))
	case "response.failed":
		msg, code := "响应失败", ""
		if ev.Response != nil && ev.Response.Error != nil {
			msg, code = ev.Response.Error.Message, ev.Response.Error.Code
		}
		return failure(code, msg)
	case "error":
		return failure(ev.Code, ev.Message)
	}
	return nil
}

func failure(code, msg string) error {
	kind := llm.ErrUnknown
	switch code {
	case "rate_limit_exceeded":
		kind = llm.ErrRateLimited
	case "server_error", "server_is_overloaded":
		kind = llm.ErrServer
	case "invalid_prompt", "invalid_request_error":
		kind = llm.ErrInvalidRequest
	}
	if k := classify(0, nil, code+" "+msg); k != "" {
		kind = k
	}
	if code != "" {
		msg = code + ": " + msg
	}
	return &llm.APIError{Kind: kind, Message: msg}
}

// finish 组装 Done。输出项以 output_item.done 为准，缺失时用终止事件中的 output 补齐。
func (s *stream) finish(r *response) llm.Done {
	if r != nil {
		for i, raw := range r.Output {
			if st := s.items[i]; st == nil || st.raw == nil {
				var it item
				if jsonx.Unmarshal(raw, &it) == nil {
					s.items[i] = &itemState{typ: it.Type, callID: it.CallID, name: it.Name, raw: raw}
				}
			}
		}
	}
	idx := make([]int, 0, len(s.items))
	for i := range s.items {
		idx = append(idx, i)
	}
	sort.Ints(idx)

	msg := llm.Message{Role: llm.RoleAssistant}
	var native []jsonx.Value
	hasTool, refused := false, false
	for _, i := range idx {
		st := s.items[i]
		if st.raw == nil {
			continue
		}
		native = append(native, st.raw)
		var it item
		_ = jsonx.Unmarshal(st.raw, &it)
		switch it.Type {
		case "message":
			for _, c := range it.Content {
				switch c.Type {
				case "output_text":
					if c.Text != "" {
						msg.Parts = append(msg.Parts, llm.Text(c.Text))
					}
				case "refusal":
					refused = true
				}
			}
		case "reasoning":
			var sum []string
			for _, x := range it.Summary {
				sum = append(sum, x.Text)
			}
			for _, c := range it.Content { // 兼容接口（如 DeepSeek）返回明文推理
				sum = append(sum, c.Text)
			}
			if t := strings.TrimSpace(strings.Join(sum, "\n")); t != "" {
				msg.Parts = append(msg.Parts, llm.Part{Kind: llm.PartReasoning, Text: t})
			}
		case "function_call":
			hasTool = true
			msg.Parts = append(msg.Parts, llm.Part{Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{ID: it.CallID, Name: it.Name, Args: argsJSON(it.Arguments)}})
		}
	}
	if len(native) > 0 {
		msg.Native = map[string]jsonx.Value{s.key: jsonx.MustMarshal(native)}
	}

	done := llm.Done{Message: msg}
	status, reason := "", ""
	if r != nil {
		status = r.Status
		if r.IncompleteDetails != nil {
			reason = r.IncompleteDetails.Reason
		}
		if u := r.Usage; u != nil {
			cached, write := u.InputTokensDetails.CachedTokens, u.InputTokensDetails.CacheWriteTokens
			done.Usage = llm.Usage{
				InputUncached: max(u.InputTokens-cached-write, 0),
				CacheRead:     cached,
				CacheWrite:    write,
				Output:        max(u.OutputTokens-u.OutputTokensDetails.ReasoningTokens, 0),
				Reasoning:     u.OutputTokensDetails.ReasoningTokens,
			}
		}
	}
	done.Raw = status
	if reason != "" {
		done.Raw += ":" + reason
	}
	switch {
	case status == "incomplete" && reason == "max_output_tokens":
		done.Stop = llm.StopMaxTokens
	case status == "incomplete" && reason == "content_filter", refused:
		done.Stop = llm.StopRefusal
	case hasTool:
		done.Stop = llm.StopToolUse
	case status == "completed":
		done.Stop = llm.StopEnd
	default:
		done.Stop = llm.StopError
	}
	return done
}

// argsJSON 把 arguments 字符串转为 JSON 值；不是合法 JSON（如被截断）时保留为 JSON 字符串，交给工具报错。
func argsJSON(s string) jsonx.Value {
	s = strings.TrimSpace(s)
	if s == "" {
		return jsonx.Value(`{}`)
	}
	if jsonx.Value(s).IsValid() {
		return jsonx.Value(s)
	}
	return jsonx.MustMarshal(s)
}
