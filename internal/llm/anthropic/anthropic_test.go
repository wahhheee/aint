package anthropic

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

// 夹具来自真实调用（M0 探针）：同一个"先说一句话，再同时查询两地天气"的请求，
// 分别经中转站调用 claude-sonnet-5-5 与 DeepSeek 的 Anthropic 兼容接口。

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
	return New(llm.ProviderConfig{BaseURL: srv.URL + "/v1/", APIKey: "test-key", HTTP: srv.Client()})
}

func collect(t *testing.T, s llm.Stream) ([]llm.Event, llm.Done) {
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

func model(name string) *llm.Model {
	return &llm.Model{Name: name, Caps: llm.Caps{Tools: true, Reasoning: "effort", Cache: "breakpoints",
		ReasoningMap: map[string]string{"off": "between_tools", "low": "adaptive:low"}}}
}

func TestStreamParallelTools(t *testing.T) {
	for _, tc := range []struct {
		fixture, model string
		usage          llm.Usage
		sigPrefix      string
	}{
		{"sonnet55-parallel-tools.sse", "claude-sonnet-5-5", llm.Usage{InputUncached: 474, Output: 118 - 92, Reasoning: 92}, "CAQS"},
		{"deepseek-parallel-tools.sse", "deepseek-flash", llm.Usage{InputUncached: 311, Output: 115}, "ade1aa38"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var gotPath, gotKey, gotVersion string
			p := serve(t, tc.fixture, func(r *http.Request, _ []byte) {
				gotPath, gotKey, gotVersion = r.URL.Path, r.Header.Get("X-Api-Key"), r.Header.Get("Anthropic-Version")
			})
			s, err := p.Stream(context.Background(), model(tc.model), &llm.Request{
				Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("hi")}}}, MaxOutput: 600,
			})
			if err != nil {
				t.Fatal(err)
			}
			evs, done := collect(t, s)
			if gotPath != "/v1/messages" || gotKey != "test-key" || gotVersion != apiVersion {
				t.Fatalf("请求地址或头不对：%s %q %q", gotPath, gotKey, gotVersion)
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
			if done.Stop != llm.StopToolUse || done.Raw != "tool_use" {
				t.Fatalf("stop = %s (%s)", done.Stop, done.Raw)
			}
			if done.Usage != tc.usage {
				t.Fatalf("usage = %+v，期望 %+v", done.Usage, tc.usage)
			}
			calls := done.Message.ToolCalls()
			if len(calls) != 2 || calls[0].Name != "get_weather" {
				t.Fatalf("工具调用 = %+v", calls)
			}
			var a struct {
				City string `json:"city"`
			}
			if err := jsonx.Unmarshal(calls[0].Args, &a); err != nil || a.City != "杭州" {
				t.Fatalf("参数 = %s (%v)", calls[0].Args, err)
			}
			if done.Message.TextOf() != text.String() || text.Len() == 0 {
				t.Fatalf("正文不一致：%q vs %q", done.Message.TextOf(), text.String())
			}

			// Native：thinking（带签名）→ text → tool_use × 2
			var blocks []struct {
				Type      string `json:"type"`
				Signature string `json:"signature"`
			}
			if err := jsonx.Unmarshal(done.Message.Native[llm.NativeKey(llm.ProtocolAnthropic, tc.model)], &blocks); err != nil {
				t.Fatal(err)
			}
			types := []string{}
			for _, b := range blocks {
				types = append(types, b.Type)
			}
			if strings.Join(types, ",") != "thinking,text,tool_use,tool_use" {
				t.Fatalf("Native 块 = %v", types)
			}
			if !strings.HasPrefix(blocks[0].Signature, tc.sigPrefix) {
				t.Fatalf("签名 = %q", blocks[0].Signature)
			}
		})
	}
}

func TestBodyReplayAndCache(t *testing.T) {
	p := New(llm.ProviderConfig{APIKey: "k"})
	m := model("claude-sonnet-5-5")
	key := llm.NativeKey(llm.ProtocolAnthropic, m.Name)
	native := jsonx.Value(`[{"type":"thinking","thinking":"","signature":"SIG"},{"type":"tool_use","id":"toolu_1","name":"read","input":{"path":"/work/a.md"}}]`)
	req := &llm.Request{
		System: []llm.SystemBlock{{Text: "L0"}, {Text: "L1", CacheMark: true}},
		Tools:  []llm.ToolSpec{{Name: "read", Description: "读取", Schema: jsonx.Value(`{"type":"object"}`)}},
		Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{{Kind: llm.PartText, Text: "L3", CacheMark: true}}},
			{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("开场"), llm.Text("")}},
			{Role: llm.RoleAssistant, Parts: []llm.Part{{Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{ID: "toolu_1", Name: "read"}}},
				Native: map[string]jsonx.Value{key: native, "gemini/x": jsonx.Value(`[1]`)}},
			{Role: llm.RoleUser, Parts: []llm.Part{{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: "toolu_1", Parts: []llm.Part{llm.Text("内容")}}}}},
		},
		MaxOutput: 1000, Reasoning: llm.ReasoningOff, CacheTTL: llm.CacheLong,
	}
	b, err := p.Body(context.Background(), m, req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"system":[{"type":"text","text":"L0"},{"type":"text","text":"L1","cache_control":{"type":"ephemeral","ttl":"1h"}}]`,
		// L3 与开场合并为一条 user 消息，空文本省略
		`{"role":"user","content":[{"type":"text","text":"L3","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","text":"开场"}]}`,
		// 同模型助手消息原样回传 Native（签名不变）
		`{"role":"assistant","content":` + string(native)[:len(native)-1],
		// 滚动断点在最后一块
		`{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"内容"}],"cache_control":{"type":"ephemeral","ttl":"1h"}}`,
		`"thinking":{"type":"between_tools"}`,
		`"tools":[{"name":"read","description":"读取","input_schema":{"type":"object"}}]`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("请求体缺少 %s\n实际：%s", want, s)
		}
	}
	if strings.Count(s, "cache_control") > maxBreakpoints {
		t.Fatalf("断点超过上限：%s", s)
	}

	// 换模型：不用 Native，按中立内容重建（推理丢弃）
	b2, err := p.Body(context.Background(), model("claude-opus-5-5"), req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b2), "SIG") || !strings.Contains(string(b2), `{"type":"tool_use","id":"toolu_1","name":"read","input":{}}`) {
		t.Fatalf("跨模型重建不对：%s", b2)
	}
}

func TestReasoningMapping(t *testing.T) {
	for _, tc := range []struct {
		caps  llm.Caps
		level llm.ReasoningLevel
		want  string
	}{
		{llm.Caps{Reasoning: "effort"}, llm.ReasoningLow, `"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}`},
		{llm.Caps{Reasoning: "effort"}, llm.ReasoningOff, `"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}`},
		{llm.Caps{Reasoning: "budget"}, llm.ReasoningMedium, `"thinking":{"type":"enabled","budget_tokens":8192}`},
		{llm.Caps{Reasoning: "budget"}, llm.ReasoningOff, `"thinking":{"type":"disabled"}`},
		{llm.Caps{Reasoning: "effort", ReasoningMap: map[string]string{"medium": "effort:high"}}, llm.ReasoningMedium, `"output_config":{"effort":"high"}`},
		{llm.Caps{Reasoning: "effort", ReasoningMap: map[string]string{"off": "disabled"}}, llm.ReasoningOff, `"thinking":{"type":"disabled"}`},
	} {
		b, err := New(llm.ProviderConfig{}).Body(context.Background(), &llm.Model{Name: "m", Caps: tc.caps},
			&llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("x")}}}, MaxOutput: 32000, Reasoning: tc.level})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), tc.want) {
			t.Errorf("%v/%s：期望包含 %s，实际 %s", tc.caps.Reasoning, tc.level, tc.want, b)
		}
	}
	// none：不发推理参数
	b, _ := New(llm.ProviderConfig{}).Body(context.Background(), &llm.Model{Name: "m", Caps: llm.Caps{Reasoning: "none"}},
		&llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("x")}}}, Reasoning: llm.ReasoningHigh})
	if strings.Contains(string(b), "thinking") || strings.Contains(string(b), "effort") {
		t.Fatalf("none 不应发送推理参数：%s", b)
	}
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		kind   llm.ErrorKind
	}{
		{529, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`, llm.ErrOverloaded},
		{400, `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 1200000 tokens > 1000000 maximum"}}`, llm.ErrContextOverflow},
		{401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`, llm.ErrAuth},
		{429, `{"error":{"message":"Upstream rate limit exceeded"}}`, llm.ErrRateLimited},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, err := New(llm.ProviderConfig{BaseURL: srv.URL, HTTP: srv.Client()}).Stream(context.Background(), model("m"),
			&llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("x")}}}})
		srv.Close()
		var apiErr *llm.APIError
		if !errors.As(err, &apiErr) || apiErr.Kind != tc.kind || apiErr.Status != tc.status || apiErr.RetryAfter.Seconds() != 7 {
			t.Errorf("%d：err = %v", tc.status, err)
		}
	}
}

func TestStreamErrorEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5}}}\n\n"+
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"+
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"雾\"}}\n\n"+
			"event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n")
	}))
	defer srv.Close()
	s, err := New(llm.ProviderConfig{BaseURL: srv.URL, HTTP: srv.Client()}).Stream(context.Background(), model("m"),
		&llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("x")}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ev, err := s.Next()
	if err != nil || ev.(llm.TextDelta).Text != "雾" {
		t.Fatalf("第一个事件 = %v %v", ev, err)
	}
	_, err = s.Next()
	if llm.KindOf(err) != llm.ErrOverloaded {
		t.Fatalf("流中错误 = %v", err)
	}
}
