package gemini

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/wire"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// stream 把 streamGenerateContent 的分块转为中立事件，并把分块中的部件合并为完整的模型消息（Native）。
//
// 合并规则：相邻的普通文本（或思考文本）分块合并为一个部件；带 thoughtSignature 的非空文本分块并入当前部件并结束它；
// 带签名的空文本分块单独成为一个部件（保留独立边界）；函数调用与其他部件原样保留。
type stream struct {
	ctx   context.Context
	body  io.ReadCloser
	sse   *wire.SSE
	key   string
	queue wire.Queue
	done  bool

	parts      []*partAcc
	finish     string
	block      string // promptFeedback.blockReason
	responseID string
	usage      *usageMeta
}

type partAcc struct {
	kind string // text | thought | call | other
	text strings.Builder
	sig  string
	raw  jsonx.Value // call / other：原样部件
	call *llm.ToolCall
	open bool // 文本部件是否还能继续合并
}

func newStream(ctx context.Context, body io.ReadCloser, key string) *stream {
	return &stream{ctx: ctx, body: body, sse: wire.NewSSE(body), key: key}
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
				if s.finish != "" || s.block != "" { // Gemini 没有单独的结束事件：有 finishReason 即正常结束
					s.done = true
					s.queue.Push(s.result())
					continue
				}
				return nil, wire.StreamError(s.ctx, wire.ErrNoDone)
			}
			return nil, wire.StreamError(s.ctx, err)
		}
		if len(ev.Data) == 0 {
			continue
		}
		if err := s.handle(ev.Data); err != nil {
			return nil, err
		}
	}
}

type chunk struct {
	Candidates []struct {
		Content *struct {
			Parts []jsonx.Value `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata *usageMeta `json:"usageMetadata"`
	ResponseID    string     `json:"responseId"`
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

type usageMeta struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	ToolUsePromptTokenCount int `json:"toolUsePromptTokenCount"`
}

// part 是部件中用到的字段。
type part struct {
	Text             *string `json:"text"`
	Thought          bool    `json:"thought"`
	ThoughtSignature string  `json:"thoughtSignature"`
	FunctionCall     *struct {
		ID   string      `json:"id"`
		Name string      `json:"name"`
		Args jsonx.Value `json:"args"`
	} `json:"functionCall"`
}

func (s *stream) handle(data []byte) error {
	var c chunk
	if err := jsonx.Unmarshal(data, &c); err != nil {
		return &llm.APIError{Kind: llm.ErrUnknown, Message: "无法解析流式分块：" + string(data), Err: err}
	}
	if c.Error != nil {
		kind := llm.KindFromStatus(c.Error.Code)
		if k := classify(c.Error.Code, nil, c.Error.Message); k != "" {
			kind = k
		}
		return &llm.APIError{Kind: kind, Status: c.Error.Code, Message: strings.TrimSpace(c.Error.Status + ": " + c.Error.Message)}
	}
	if c.ResponseID != "" {
		s.responseID = c.ResponseID
	}
	if c.UsageMetadata != nil {
		s.usage = c.UsageMetadata
	}
	if c.PromptFeedback != nil && c.PromptFeedback.BlockReason != "" {
		s.block = c.PromptFeedback.BlockReason
	}
	if len(c.Candidates) == 0 {
		return nil
	}
	cand := c.Candidates[0]
	if cand.Content != nil {
		for _, raw := range cand.Content.Parts {
			s.addPart(raw)
		}
	}
	if cand.FinishReason != "" {
		s.finish = cand.FinishReason
	}
	return nil
}

func (s *stream) addPart(raw jsonx.Value) {
	raw = compact(raw)
	var p part
	if jsonx.Unmarshal(raw, &p) != nil {
		return
	}
	switch {
	case p.FunctionCall != nil:
		id := p.FunctionCall.ID
		if id == "" {
			id = s.synthID(len(s.parts))
		}
		tc := &llm.ToolCall{ID: id, Name: p.FunctionCall.Name, Args: argsOrEmpty(p.FunctionCall.Args)}
		s.closeText()
		s.parts = append(s.parts, &partAcc{kind: "call", raw: raw, call: tc})
		s.queue.Push(llm.ToolCallStart{ID: id, Name: tc.Name}, llm.ToolCallDone{ID: id, Name: tc.Name, Args: tc.Args})
	case p.Text != nil:
		kind := "text"
		if p.Thought {
			kind = "thought"
		}
		text := *p.Text
		if text == "" && p.ThoughtSignature == "" {
			return // 结束分块常带一个空文本，忽略
		}
		if text != "" {
			if kind == "text" {
				s.queue.Push(llm.TextDelta{Text: text})
			} else {
				s.queue.Push(llm.ReasoningSummaryDelta{Text: text})
			}
		}
		last := s.last()
		if text != "" && last != nil && last.open && last.kind == kind {
			last.text.WriteString(text)
			if p.ThoughtSignature != "" {
				last.sig, last.open = p.ThoughtSignature, false
			}
			return
		}
		s.closeText()
		acc := &partAcc{kind: kind, sig: p.ThoughtSignature, open: p.ThoughtSignature == "" && text != ""}
		acc.text.WriteString(text)
		s.parts = append(s.parts, acc)
	default:
		s.closeText()
		s.parts = append(s.parts, &partAcc{kind: "other", raw: raw})
	}
}

// compact 去掉部件 JSON 中的空白（网关常输出带空格的 JSON），Native 只存紧凑形式。
func compact(raw jsonx.Value) jsonx.Value {
	v := slices.Clone(raw)
	if v.Compact() != nil {
		return raw
	}
	return v
}

func (s *stream) last() *partAcc {
	if len(s.parts) == 0 {
		return nil
	}
	return s.parts[len(s.parts)-1]
}

func (s *stream) closeText() {
	if l := s.last(); l != nil {
		l.open = false
	}
}

// synthID 为缺少 ID 的函数调用生成确定性 ID。
func (s *stream) synthID(i int) string {
	sum := sha256.Sum256([]byte(s.responseID + "/" + strconv.Itoa(i)))
	return synthPrefix + hex.EncodeToString(sum[:8])
}

// result 组装 Done。
func (s *stream) result() llm.Done {
	msg := llm.Message{Role: llm.RoleAssistant}
	var native []jsonx.Value
	hasCall := false
	for _, p := range s.parts {
		switch p.kind {
		case "text":
			native = append(native, jsonx.MustMarshal(textPart{Text: p.text.String(), ThoughtSignature: p.sig}))
			if p.text.Len() > 0 {
				msg.Parts = append(msg.Parts, llm.Text(p.text.String()))
			}
		case "thought":
			native = append(native, jsonx.MustMarshal(textPart{Text: p.text.String(), Thought: true, ThoughtSignature: p.sig}))
			if p.text.Len() > 0 {
				msg.Parts = append(msg.Parts, llm.Part{Kind: llm.PartReasoning, Text: p.text.String()})
			}
		case "call":
			hasCall = true
			native = append(native, p.raw)
			msg.Parts = append(msg.Parts, llm.Part{Kind: llm.PartToolCall, ToolCall: p.call})
		default:
			native = append(native, p.raw)
		}
	}
	if len(native) > 0 {
		msg.Native = map[string]jsonx.Value{s.key: jsonx.MustMarshal(native)}
	}
	done := llm.Done{Message: msg, Raw: s.finish}
	if s.block != "" {
		done.Raw = "blocked:" + s.block
	}
	if u := s.usage; u != nil {
		done.Usage = llm.Usage{
			InputUncached: max(u.PromptTokenCount-u.CachedContentTokenCount, 0) + u.ToolUsePromptTokenCount,
			CacheRead:     u.CachedContentTokenCount,
			Output:        u.CandidatesTokenCount,
			Reasoning:     u.ThoughtsTokenCount,
		}
	}
	done.Stop = stopReason(s.finish, s.block, hasCall)
	return done
}

func stopReason(finish, block string, hasCall bool) llm.StopReason {
	if block != "" {
		return llm.StopRefusal
	}
	switch finish {
	case "STOP":
		if hasCall {
			return llm.StopToolUse
		}
		return llm.StopEnd
	case "MAX_TOKENS", "CONTINUATION": // CONTINUATION：Gemini 4 单次请求输出上限，按截断续写处理
		return llm.StopMaxTokens
	case "SAFETY", "RECITATION", "LANGUAGE", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT":
		return llm.StopRefusal
	}
	if hasCall && finish == "" {
		return llm.StopToolUse
	}
	return llm.StopError // MALFORMED_FUNCTION_CALL、MISSING_THOUGHT_SIGNATURE、OTHER 等
}
