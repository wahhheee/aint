package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// 夹具来自真实调用（M0 探针）："先说一句话，再同时查询两地天气"，
// 分别经中转站调用 gpt-6.1-sol 与 DeepSeek 的 Responses 兼容接口。

func serve(t *testing.T, fixture string, check func(r *http.Request, body []byte)) *Provider {
	t.Helper()
	data, err := os.ReadFile("testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if check != nil {
			check(r, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return New(llm.ProviderConfig{BaseURL: srv.URL + "/v1", APIKey: "test-key", HTTP: srv.Client()})
}

func drain(t *testing.T, s llm.Stream) ([]llm.Event, llm.Done) {
	t.Helper()
	defer func() { _ = s.Close() }()
	var evs []llm.Event
	for {
		ev, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		evs = append(evs, ev)
	}
	done, ok := evs[len(evs)-1].(llm.Done)
	if !ok {
		t.Fatalf("最后一个事件不是 Done：%T", evs[len(evs)-1])
	}
	return evs, done
}

func TestStreamParallelTools(t *testing.T) {
	for _, tc := range []struct {
		fixture, model string
		usage          llm.Usage
		native         string // Native 中输出项的类型序列
	}{
		{"gpt61sol-parallel-tools.sse", "gpt-6.1-sol", llm.Usage{InputUncached: 75, Output: 65}, "message,function_call,function_call"},
		{"deepseek-parallel-tools.sse", "deepseek-flash", llm.Usage{InputUncached: 316, Output: 111 - 28, Reasoning: 28}, "reasoning,message,function_call,function_call"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var path, auth string
			p := serve(t, tc.fixture, func(r *http.Request, _ []byte) { path, auth = r.URL.Path, r.Header.Get("Authorization") })
			s, err := p.Stream(context.Background(), &llm.Model{Name: tc.model},
				&llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("hi")}}}})
			if err != nil {
				t.Fatal(err)
			}
			evs, done := drain(t, s)
			if path != "/v1/responses" || auth != "Bearer test-key" {
				t.Fatalf("请求地址或鉴权不对：%s %q", path, auth)
			}
			var text strings.Builder
			starts, dones := 0, 0
			for _, ev := range evs {
				switch ev := ev.(type) {
				case llm.TextDelta:
					text.WriteString(ev.Text)
				case llm.ToolCallStart:
					starts++
				case llm.ToolCallDone:
					dones++
				}
			}
			if starts != 2 || dones != 2 {
				t.Fatalf("工具调用事件：start=%d done=%d", starts, dones)
			}
			if done.Stop != llm.StopToolUse || done.Usage != tc.usage {
				t.Fatalf("stop=%s usage=%+v", done.Stop, done.Usage)
			}
			if done.Message.TextOf() != text.String() || text.Len() == 0 {
				t.Fatalf("正文不一致：%q vs %q", done.Message.TextOf(), text.String())
			}
			calls := done.Message.ToolCalls()
			var a struct {
				City string `json:"city"`
			}
			if len(calls) != 2 || jsonx.Unmarshal(calls[1].Args, &a) != nil || a.City != "北京" {
				t.Fatalf("工具调用 = %+v", calls)
			}
			var items []struct {
				Type string `json:"type"`
			}
			if err := jsonx.Unmarshal(done.Message.Native[llm.NativeKey(llm.ProtocolOpenAIResponses, tc.model)], &items); err != nil {
				t.Fatal(err)
			}
			var types []string
			for _, it := range items {
				types = append(types, it.Type)
			}
			if strings.Join(types, ",") != tc.native {
				t.Fatalf("Native 输出项 = %v", types)
			}
		})
	}
}

func TestBody(t *testing.T) {
	key := llm.NativeKey(llm.ProtocolOpenAIResponses, "gpt-6.1-sol")
	native := jsonx.Value(`[{"id":"rs_1","type":"reasoning","encrypted_content":"ENC","summary":[]},{"id":"fc_1","type":"function_call","status":"completed","arguments":"{\"path\":\"/a\"}","call_id":"call_1","name":"read"}]`)
	req := &llm.Request{
		System: []llm.SystemBlock{{Text: "L0"}, {Text: "L1", CacheMark: true}},
		Tools:  []llm.ToolSpec{{Name: "read", Description: "读取", Schema: jsonx.Value(`{"type":"object"}`)}},
		Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{{Kind: llm.PartText, Text: "L3", CacheMark: true}, llm.Text("开场")}},
			{Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text("旧正文"), {Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{ID: "call_1", Name: "read", Args: jsonx.Value(`{"path":"/a"}`)}}},
				Native: map[string]jsonx.Value{key: native}},
			{Role: llm.RoleUser, Parts: []llm.Part{{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: "call_1", Parts: []llm.Part{llm.Text("内容")}}}, llm.Text("<input>继续</input>")}},
		},
		MaxOutput: 1000, Reasoning: llm.ReasoningLow, CacheKey: "aint:s1", CacheTTL: llm.CacheLong,
	}

	// GPT-5.6 起：断点模式
	m := &llm.Model{Name: "gpt-6.1-sol", Caps: llm.Caps{Reasoning: "effort", Cache: "breakpoints"}}
	b, err := New(llm.ProviderConfig{}).Body(context.Background(), m, req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"input":[{"role":"developer","content":[{"type":"input_text","text":"L0"},{"type":"input_text","text":"L1","prompt_cache_breakpoint":{"mode":"explicit"}}]}`,
		`{"role":"user","content":[{"type":"input_text","text":"L3","prompt_cache_breakpoint":{"mode":"explicit"}},{"type":"input_text","text":"开场"}]}`,
		string(native)[1 : len(native)-1], // Native 原样展开
		`{"type":"function_call_output","call_id":"call_1","output":"内容"},{"role":"user","content":[{"type":"input_text","text":"<input>继续</input>"}]}`,
		`"reasoning":{"effort":"low"},"include":["reasoning.encrypted_content"]`,
		`"prompt_cache_key":"aint:s1","prompt_cache_options":{"mode":"implicit","ttl":"12h"},"store":false,"stream":true`,
		`"tools":[{"type":"function","name":"read","description":"读取","parameters":{"type":"object"},"strict":false}]`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("请求体缺少 %s\n实际：%s", want, s)
		}
	}
	if strings.Contains(s, "instructions") || strings.Contains(s, "旧正文") {
		t.Fatalf("断点模式不应使用 instructions，同模型应走 Native：%s", s)
	}

	// 自动缓存（DeepSeek 等）：instructions + prompt_cache_key；换模型时按中立内容重建
	m2 := &llm.Model{Name: "deepseek-flash", Caps: llm.Caps{Reasoning: "effort", Cache: "implicit", ReasoningMap: map[string]string{"low": "low", "off": "omit"}}}
	b2, err := New(llm.ProviderConfig{}).Body(context.Background(), m2, req)
	if err != nil {
		t.Fatal(err)
	}
	s2 := string(b2)
	for _, want := range []string{
		`"instructions":"L0\n\nL1"`,
		`{"role":"assistant","content":[{"type":"output_text","text":"旧正文"}]},{"type":"function_call","call_id":"call_1","name":"read","arguments":"{\"path\":\"/a\"}"}`,
		`"prompt_cache_key":"aint:s1","store":false`,
	} {
		if !strings.Contains(s2, want) {
			t.Fatalf("请求体缺少 %s\n实际：%s", want, s2)
		}
	}
	if strings.Contains(s2, "ENC") || strings.Contains(s2, "prompt_cache_breakpoint") || strings.Contains(s2, "prompt_cache_options") {
		t.Fatalf("跨模型或自动缓存模式下不应出现这些字段：%s", s2)
	}
	req.Reasoning = llm.ReasoningOff
	b3, _ := New(llm.ProviderConfig{}).Body(context.Background(), m2, req)
	if strings.Contains(string(b3), `"reasoning"`) {
		t.Fatalf("omit 不应发送 reasoning：%s", b3)
	}
}

func TestIncompleteAndFailed(t *testing.T) {
	for _, tc := range []struct {
		name, sse string
		stop      llm.StopReason
		kind      llm.ErrorKind
	}{
		{"截断", `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","role":"assistant","content":[]}}

data: {"type":"response.output_text.delta","output_index":0,"delta":"雾"}

data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","role":"assistant","status":"incomplete","content":[{"type":"output_text","text":"雾"}]}}

data: {"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":10,"output_tokens":1}}}

`, llm.StopMaxTokens, ""},
		{"上下文超限", `data: {"type":"response.failed","response":{"status":"failed","error":{"code":"context_length_exceeded","message":"Your input exceeds the context window"}}}

`, "", llm.ErrContextOverflow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, tc.sse)
			}))
			defer srv.Close()
			s, err := New(llm.ProviderConfig{BaseURL: srv.URL, HTTP: srv.Client()}).Stream(context.Background(), &llm.Model{Name: "m"},
				&llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("x")}}}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			var last llm.Event
			for {
				ev, err := s.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					if llm.KindOf(err) != tc.kind {
						t.Fatalf("错误种类 = %v", err)
					}
					return
				}
				last = ev
			}
			done := last.(llm.Done)
			if done.Stop != tc.stop || done.Message.TextOf() != "雾" {
				t.Fatalf("stop=%s text=%q", done.Stop, done.Message.TextOf())
			}
		})
	}
}
