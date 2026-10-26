package provider

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
)

// 手工 SSE 覆盖无法稳定通过真实调用触发的供应商停止原因与流中错误。
func TestProtocolStreamEdges(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol llm.Protocol
		data     string
		stop     llm.StopReason
		kind     llm.ErrorKind
	}{
		{"Responses拒答", llm.ProtocolOpenAIResponses, `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"不能回答"}]}]}}`, llm.StopRefusal, ""},
		{"Responses过滤", llm.ProtocolOpenAIResponses, `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"content_filter"}}}`, llm.StopRefusal, ""},
		{"Responses限流", llm.ProtocolOpenAIResponses, `{"type":"error","code":"rate_limit_exceeded","message":"限流"}`, "", llm.ErrRateLimited},
		{"Responses余额不足", llm.ProtocolOpenAIResponses, `{"type":"response.failed","response":{"error":{"code":"insufficient_quota","message":"余额不足"}}}`, "", llm.ErrAuth},
		{"Gemini提示拒答", llm.ProtocolGemini, `{"promptFeedback":{"blockReason":"SAFETY"}}`, llm.StopRefusal, ""},
		{"Gemini输出拒答", llm.ProtocolGemini, `{"candidates":[{"finishReason":"SAFETY"}]}`, llm.StopRefusal, ""},
		{"Gemini无效工具", llm.ProtocolGemini, `{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}]}`, llm.StopError, ""},
		{"Gemini续写上限", llm.ProtocolGemini, `{"candidates":[{"finishReason":"CONTINUATION"}]}`, llm.StopMaxTokens, ""},
		{"Gemini流中限流", llm.ProtocolGemini, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"限流"}}`, "", llm.ErrRateLimited},
		{"Gemini超限", llm.ProtocolGemini, `{"error":{"code":400,"message":"input exceeds the maximum number of tokens"}}`, "", llm.ErrContextOverflow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: "+tc.data+"\n\n")
			}))
			defer srv.Close()
			p, err := NewAdapter(llm.ProviderConfig{Protocol: tc.protocol, BaseURL: srv.URL, HTTP: srv.Client()})
			if err != nil {
				t.Fatal(err)
			}
			s, err := p.Stream(t.Context(), &llm.Model{Name: "m"}, &llm.Request{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			var stop llm.StopReason
			for {
				ev, err := s.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					if tc.kind == "" || llm.KindOf(err) != tc.kind {
						t.Fatalf("错误=%v", err)
					}
					return
				}
				if d, ok := ev.(llm.Done); ok {
					stop = d.Stop
				}
			}
			if tc.kind != "" || stop != tc.stop {
				t.Fatalf("stop=%s，期望 %s / error=%s", stop, tc.stop, tc.kind)
			}
		})
	}
}
