package provider

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

// TestRecordedUsage 使用历史真实录制核对停止原因与互斥的用量分项，不访问外网。
func TestRecordedUsage(t *testing.T) {
	var cases []struct {
		Protocol llm.Protocol   `json:"protocol"`
		Model    string         `json:"model"`
		File     string         `json:"file"`
		Stop     llm.StopReason `json:"stop"`
		Usage    llm.Usage      `json:"usage"`
	}
	b, err := os.ReadFile("testdata/replay.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonx.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(string(tc.Protocol)+"/"+tc.File, func(t *testing.T) {
			done := replayResponse(t, tc.Protocol, tc.Model, tc.File)
			if done.Stop != tc.Stop || done.Usage != tc.Usage {
				t.Fatalf("stop=%s usage=%+v，期望 %s %+v", done.Stop, done.Usage, tc.Stop, tc.Usage)
			}
		})
	}
}

func replayResponse(t *testing.T, protocol llm.Protocol, model, file string) llm.Done {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	p, err := NewAdapter(llm.ProviderConfig{Protocol: protocol, BaseURL: srv.URL, HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	s, err := p.Stream(t.Context(), &llm.Model{Name: model}, &llm.Request{Messages: []llm.Message{user(llm.Text("测试"))}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var text strings.Builder
	var done llm.Done
	for {
		ev, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch ev := ev.(type) {
		case llm.TextDelta:
			text.WriteString(ev.Text)
		case llm.Done:
			done = ev
		}
	}
	if done.Message.TextOf() != text.String() {
		t.Fatal("正文与增量不一致")
	}
	return done
}

func TestCrossProtocolHistory(t *testing.T) {
	cases := []struct {
		protocol    llm.Protocol
		model, file string
	}{
		{llm.ProtocolAnthropic, "claude-sonnet-5-5", "../llm/anthropic/testdata/sonnet55-parallel-tools.sse"},
		{llm.ProtocolOpenAIResponses, "gpt-6.1-sol", "../llm/openai/testdata/gpt61sol-parallel-tools.sse"},
		{llm.ProtocolGemini, "gemini-3.8-flash", "../llm/gemini/testdata/flash38-parallel-tools.sse"},
	}
	for _, source := range cases {
		done := replayResponse(t, source.protocol, source.model, source.file)
		for _, target := range cases {
			t.Run(string(source.protocol)+"/"+string(target.protocol), func(t *testing.T) {
				p, err := NewAdapter(llm.ProviderConfig{Protocol: target.protocol})
				if err != nil {
					t.Fatal(err)
				}
				results := user()
				for _, c := range done.Message.ToolCalls() {
					results.Parts = append(results.Parts, llm.Part{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: c.ID, Parts: []llm.Part{llm.Text("晴天")}}})
				}
				req := &llm.Request{Messages: []llm.Message{user(llm.Text("查询天气")), done.Message, results}}
				before := string(jsonx.MustMarshal(req))
				body, err := p.(interface {
					Body(context.Context, *llm.Model, *llm.Request) ([]byte, error)
				}).Body(t.Context(), &llm.Model{Name: target.model}, req)
				if err != nil {
					t.Fatal(err)
				}
				if before != string(jsonx.MustMarshal(req)) {
					t.Fatal("构建请求修改了历史")
				}
				if strings.Count(string(body), "get_weather") < 2 || !strings.Contains(string(body), "晴天") {
					t.Fatal("工具调用或结果丢失")
				}
				if source.protocol == target.protocol {
					var blocks []jsonx.Value
					if err := jsonx.Unmarshal(done.Message.Native[llm.NativeKey(source.protocol, source.model)], &blocks); err != nil {
						t.Fatal(err)
					}
					for _, block := range blocks {
						if !strings.Contains(string(body), string(block)) {
							t.Fatalf("原生块没有原样回传：%s", block)
						}
					}
				} else if strings.Contains(string(body), "encrypted_content") || strings.Contains(string(body), `"thinking":`) {
					t.Fatal("跨协议历史残留推理数据")
				}
			})
		}
	}
}
