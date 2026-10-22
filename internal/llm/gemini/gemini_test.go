package gemini

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

func serveData(t *testing.T, data string, check func(r *http.Request, body []byte)) *Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if check != nil {
			check(r, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, data)
	}))
	t.Cleanup(srv.Close)
	return New(llm.ProviderConfig{BaseURL: srv.URL + "/v1beta", APIKey: "test-key", HTTP: srv.Client()})
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
	return evs, evs[len(evs)-1].(llm.Done)
}

var userHi = &llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("hi")}}}}

// 夹具来自真实调用（M0 探针）：经中转站调用 gemini-3.8-flash，"先说一句话，再同时查询两地天气"。
func TestStreamParallelTools(t *testing.T) {
	data, err := os.ReadFile("testdata/flash38-parallel-tools.sse")
	if err != nil {
		t.Fatal(err)
	}
	var path, key string
	p := serveData(t, string(data), func(r *http.Request, _ []byte) { path, key = r.URL.String(), r.Header.Get("X-Goog-Api-Key") })
	s, err := p.Stream(context.Background(), &llm.Model{Name: "gemini-3.8-flash"}, userHi)
	if err != nil {
		t.Fatal(err)
	}
	evs, done := drain(t, s)
	if path != "/v1beta/models/gemini-3.8-flash:streamGenerateContent?alt=sse" || key != "test-key" {
		t.Fatalf("请求地址或鉴权不对：%s %q", path, key)
	}
	dones := 0
	for _, ev := range evs {
		if _, ok := ev.(llm.ToolCallDone); ok {
			dones++
		}
	}
	if dones != 2 || done.Stop != llm.StopToolUse || done.Raw != "STOP" {
		t.Fatalf("工具调用 %d 个，stop=%s(%s)", dones, done.Stop, done.Raw)
	}
	if done.Usage != (llm.Usage{InputUncached: 69, Output: 44}) {
		t.Fatalf("usage = %+v", done.Usage)
	}
	calls := done.Message.ToolCalls()
	if len(calls) != 2 || calls[0].ID != "call_444974" || calls[1].ID != "call_444980" {
		t.Fatalf("工具调用 = %+v", calls)
	}
	native := string(done.Message.Native[llm.NativeKey(llm.ProtocolGemini, "gemini-3.8-flash")])
	// 文本 + 第一个调用（带签名）+ 第二个调用；结束分块的空文本不进入 Native；网关输出的空格被压缩
	if !strings.HasPrefix(native, `[{"text":"我将为您查询杭州和北京的实时天气情况。"},{"functionCall":{"name":"get_weather","args":{"city":"杭州"},"id":"call_444974"},"thoughtSignature":"EmkK`) ||
		strings.Count(native, "functionCall") != 2 || strings.Contains(native, `"text":""`) {
		t.Fatalf("Native = %s", native)
	}
}

func TestMergeTextAndSignatures(t *testing.T) {
	sse := `data: {"candidates":[{"content":{"parts":[{"text":"想","thought":true}],"role":"model"}}]}

data: {"candidates":[{"content":{"parts":[{"text":"一想","thought":true,"thoughtSignature":"T1"}],"role":"model"}}]}

data: {"candidates":[{"content":{"parts":[{"text":"雾从"}],"role":"model"}}]}

data: {"candidates":[{"content":{"parts":[{"text":"傍晚开始"}],"role":"model"}}]}

data: {"candidates":[{"content":{"parts":[{"text":"","thoughtSignature":"S2"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":60,"candidatesTokenCount":5,"thoughtsTokenCount":7}}

`
	p := serveData(t, sse, nil)
	s, err := p.Stream(context.Background(), &llm.Model{Name: "g"}, userHi)
	if err != nil {
		t.Fatal(err)
	}
	_, done := drain(t, s)
	native := string(done.Message.Native["gemini/g"])
	want := `[{"text":"想一想","thought":true,"thoughtSignature":"T1"},{"text":"雾从傍晚开始"},{"text":"","thoughtSignature":"S2"}]`
	if native != want {
		t.Fatalf("Native =\n%s\n期望\n%s", native, want)
	}
	if done.Message.TextOf() != "雾从傍晚开始" || done.Stop != llm.StopEnd {
		t.Fatalf("text=%q stop=%s", done.Message.TextOf(), done.Stop)
	}
	if done.Usage != (llm.Usage{InputUncached: 40, CacheRead: 60, Output: 5, Reasoning: 7}) {
		t.Fatalf("usage = %+v", done.Usage)
	}
}

func TestBody(t *testing.T) {
	key := llm.NativeKey(llm.ProtocolGemini, "gemini-3.8-flash")
	native := jsonx.Value(`[{"functionCall":{"name":"read","args":{"path":"/a"},"id":"c1"},"thoughtSignature":"SIG"}]`)
	req := &llm.Request{
		System: []llm.SystemBlock{{Text: "L0"}, {Text: "L1", CacheMark: true}},
		Tools:  []llm.ToolSpec{{Name: "read", Description: "读取", Schema: jsonx.Value(`{"type":"object"}`)}},
		Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{{Kind: llm.PartText, Text: "L3", CacheMark: true}}},
			{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("开场")}},
			{Role: llm.RoleAssistant, Parts: []llm.Part{{Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{ID: "c1", Name: "read", Args: jsonx.Value(`{"path":"/a"}`)}}},
				Native: map[string]jsonx.Value{key: native}},
			{Role: llm.RoleUser, Parts: []llm.Part{{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: "c1", Parts: []llm.Part{llm.Text("内容")}}}, llm.Text("<input>继续</input>")}},
		},
		MaxOutput: 1000, Reasoning: llm.ReasoningOff,
	}
	m := &llm.Model{Name: "gemini-3.8-flash", Caps: llm.Caps{Reasoning: "level"}}
	b, err := New(llm.ProviderConfig{}).Body(context.Background(), m, req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"systemInstruction":{"parts":[{"text":"L0"},{"text":"L1"}]}`,
		`{"role":"user","parts":[{"text":"L3"},{"text":"开场"}]}`,
		`{"role":"model","parts":` + string(native) + `}`,
		`{"role":"user","parts":[{"functionResponse":{"id":"c1","name":"read","response":{"output":"内容"}}},{"text":"<input>继续</input>"}]}`,
		`"tools":[{"functionDeclarations":[{"name":"read","description":"读取","parametersJsonSchema":{"type":"object"}}]}]`,
		`"generationConfig":{"maxOutputTokens":1000,"thinkingConfig":{"thinkingLevel":"low"}}`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("请求体缺少 %s\n实际：%s", want, s)
		}
	}

	// 跨模型：按中立内容重建，第一个函数调用带占位签名
	b2, _ := New(llm.ProviderConfig{}).Body(context.Background(), &llm.Model{Name: "gemini-4-argon"}, req)
	if !strings.Contains(string(b2), `{"functionCall":{"id":"c1","name":"read","args":{"path":"/a"}},"thoughtSignature":"skip_thought_signature_validator"}`) {
		t.Fatalf("跨模型重建不对：%s", b2)
	}
}
