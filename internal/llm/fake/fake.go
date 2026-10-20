// Package fake 是脚本化的假模型：按脚本流式输出正文与工具调用，结果完全确定。
//
// 用途（实施计划原则 2"假模型先行"）：
//   - 运行器、存档树、整理、分支等逻辑的自动化测试；
//   - 前端开发与端到端测试：yanyi --fake-llm demo（内置演示脚本）或 --fake-llm path/to/script.yaml；
//   - 复现问题：把出问题的模型输出写成脚本，稳定重放。
//
// 脚本格式见 script.go；内置演示脚本见 demo.yaml。
package fake

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// ModelName 是假模型在供应商侧的名字。
const ModelName = "fake-demo"

// Model 返回假模型的配置：能力全开，价格未知（界面显示"无法估算"）。
func Model() *llm.Model {
	return &llm.Model{
		ID:            "fake",
		ProviderID:    "fake",
		Name:          ModelName,
		DisplayName:   "假模型（演示）",
		ContextWindow: 200_000,
		MaxOutput:     32_000,
		Caps: llm.Caps{
			Vision: true, Tools: true, ParallelTools: true, ImageInToolResult: true,
			Reasoning: "none", Cache: "none", ImageMaxEdge: 1568,
		},
	}
}

// Provider 是假模型供应商。按存档（Request.CacheKey）分别计数，依次循环使用脚本中的响应。
type Provider struct {
	script *Script
	mu     sync.Mutex
	calls  map[string]int
}

// New 创建假模型。
func New(s *Script) *Provider {
	return &Provider{script: s, calls: make(map[string]int)}
}

func (p *Provider) Protocol() llm.Protocol { return llm.ProtocolFake }

func (p *Provider) ListModels(context.Context) ([]llm.RemoteModel, error) {
	return []llm.RemoteModel{{ID: ModelName, DisplayName: "假模型（演示）"}}, nil
}

func (p *Provider) Stream(ctx context.Context, _ *llm.Model, req *llm.Request) (llm.Stream, error) {
	if len(p.script.Responses) == 0 {
		return nil, errs.Invalid("假模型脚本没有任何响应")
	}
	p.mu.Lock()
	n := p.calls[req.CacheKey]
	p.calls[req.CacheKey] = n + 1
	p.mu.Unlock()

	resp := p.script.Responses[n%len(p.script.Responses)]
	if resp.Error != "" {
		return nil, &llm.APIError{Kind: llm.ErrorKind(resp.Error), Message: "假模型按脚本模拟的错误"}
	}
	text := strings.ReplaceAll(resp.Text, "{{input}}", lastPlayerText(req))
	return &stream{ctx: ctx, resp: resp, text: text, call: n, inputChars: requestChars(req)}, nil
}

// stream 在 Next 中惰性产生事件（不起 goroutine），按脚本的节奏 sleep。
type stream struct {
	ctx        context.Context
	resp       Response
	text       string
	call       int
	inputChars int

	pos      int // 已输出的字节位置
	toolIdx  int
	started  bool
	finished bool
	out      strings.Builder
}

func (s *stream) Next() (llm.Event, error) {
	if s.finished {
		return nil, io.EOF
	}
	delay := time.Duration(s.resp.delay()) * time.Millisecond
	if !s.started {
		s.started = true
		delay = time.Duration(s.resp.FirstDelayMS) * time.Millisecond
	}
	if err := sleep(s.ctx, delay); err != nil {
		return nil, err
	}

	// 1) 正文增量
	if s.pos < len(s.text) {
		end := s.pos
		for i := 0; i < s.resp.chunk() && end < len(s.text); i++ {
			_, size := utf8.DecodeRuneInString(s.text[end:])
			end += size
		}
		delta := s.text[s.pos:end]
		s.pos = end
		s.out.WriteString(delta)
		return llm.TextDelta{Text: delta}, nil
	}
	// 2) 工具调用（参数一次给全）
	if s.toolIdx < len(s.resp.ToolCalls) {
		tc := s.resp.ToolCalls[s.toolIdx]
		s.toolIdx++
		args, err := jsonx.Marshal(tc.Args)
		if err != nil {
			return nil, fmt.Errorf("假模型脚本中的工具参数无法序列化: %w", err)
		}
		return llm.ToolCallDone{ID: fmt.Sprintf("call_%d_%d", s.call, s.toolIdx), Name: tc.Name, Args: args}, nil
	}
	// 3) 结束
	s.finished = true
	return llm.Done{Stop: s.resp.stop(), Usage: s.usage(), Message: s.message(), Raw: "fake"}, nil
}

func (s *stream) Close() error { return nil }

func (s *stream) message() llm.Message {
	m := llm.Message{Role: llm.RoleAssistant}
	if s.out.Len() > 0 {
		m.Parts = append(m.Parts, llm.Text(s.out.String()))
	}
	for i, tc := range s.resp.ToolCalls {
		args, _ := jsonx.Marshal(tc.Args)
		m.Parts = append(m.Parts, llm.Part{Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{
			ID: fmt.Sprintf("call_%d_%d", s.call, i+1), Name: tc.Name, Args: args,
		}})
	}
	return m
}

func (s *stream) usage() llm.Usage {
	if s.resp.Usage != nil {
		return *s.resp.Usage
	}
	// 粗略估算：中文约 1 字 1 token。只为界面有数可看，不代表真实计费。
	return llm.Usage{InputUncached: s.inputChars, Output: utf8.RuneCountInString(s.text)}
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var inputTag = regexp.MustCompile(`(?s)<input[^>]*>(.*?)</input>`)

// lastPlayerText 取最后一条玩家消息中的玩家原文（<input> 内容或选项结果），供脚本中的 {{input}} 使用。
func lastPlayerText(req *llm.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		m := req.Messages[i]
		if m.Role != llm.RoleUser {
			continue
		}
		if sub := inputTag.FindStringSubmatch(m.TextOf()); sub != nil {
			return strings.TrimSpace(sub[1])
		}
		for _, p := range m.Parts {
			if p.Kind == llm.PartToolResult && p.ToolResult != nil && len(p.ToolResult.Parts) > 0 {
				return p.ToolResult.Parts[0].Text
			}
		}
		return strings.TrimSpace(m.TextOf())
	}
	return ""
}

func requestChars(req *llm.Request) int {
	n := 0
	for _, b := range req.System {
		n += utf8.RuneCountInString(b.Text)
	}
	for _, m := range req.Messages {
		n += utf8.RuneCountInString(m.TextOf())
	}
	return n
}
