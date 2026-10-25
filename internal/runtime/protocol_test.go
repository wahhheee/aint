package runtime_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/provider"
	"github.com/wahhheee/yanyi/internal/save"
)

func protocolText(protocol llm.Protocol, text string, truncated bool) string {
	quote := string(jsonx.MustMarshal(text))
	switch protocol {
	case llm.ProtocolAnthropic:
		stop := "end_turn"
		if truncated {
			stop = "max_tokens"
		}
		return `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":` + quote + `}}` + "\n\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n" + `data: {"type":"message_delta","delta":{"stop_reason":"` + stop + `"},"usage":{"input_tokens":10,"output_tokens":5}}` + "\n\n" + `data: {"type":"message_stop"}` + "\n\n"
	case llm.ProtocolOpenAIResponses:
		status, typ, details := "completed", "response.completed", ""
		if truncated {
			status, typ, details = "incomplete", "response.incomplete", `,"incomplete_details":{"reason":"max_output_tokens"}`
		}
		return `data: {"type":"response.output_text.delta","delta":` + quote + `}` + "\n\n" + `data: {"type":"` + typ + `","response":{"status":"` + status + `"` + details + `,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":` + quote + `}]}],"usage":{"input_tokens":10,"output_tokens":5}}}` + "\n\n"
	default:
		stop := "STOP"
		if truncated {
			stop = "MAX_TOKENS"
		}
		return `data: {"candidates":[{"content":{"parts":[{"text":` + quote + `}]},"finishReason":"` + stop + `"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}` + "\n\n"
	}
}

func TestProtocolContinuation(t *testing.T) {
	const first = "水珠沿着帽檐缓缓落下。"
	for _, protocol := range []llm.Protocol{llm.ProtocolAnthropic, llm.ProtocolOpenAIResponses, llm.ProtocolGemini} {
		t.Run(string(protocol), func(t *testing.T) {
			var requests []string
			var requestsMu sync.Mutex
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestsMu.Lock()
				defer requestsMu.Unlock()
				b, _ := io.ReadAll(r.Body)
				requests = append(requests, string(b))
				w.Header().Set("Content-Type", "text/event-stream")
				text := first
				if len(requests) > 1 {
					text += "他走进邮局。"
				}
				_, _ = io.WriteString(w, protocolText(protocol, text, len(requests) == 1))
			}))
			defer srv.Close()
			p, err := provider.NewAdapter(llm.ProviderConfig{Protocol: protocol, BaseURL: srv.URL, HTTP: srv.Client()})
			if err != nil {
				t.Fatal(err)
			}
			e := setupWith(t, p)
			sv := choicesSave(t, e, "")
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			if _, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputFree, Text: "开始"}); err != nil {
				t.Fatal(err)
			}
			waitFor(t, sub, event.TurnCommitted)
			tl, err := e.saves.Timeline(t.Context(), sv.ID, "", 1)
			if err != nil {
				t.Fatal(err)
			}
			n := tl.Nodes[len(tl.Nodes)-1]
			var shown strings.Builder
			for _, item := range n.Display {
				if item.T == "text" {
					shown.WriteString(item.Markup)
				}
			}
			requestsMu.Lock()
			defer requestsMu.Unlock()
			if len(requests) != 2 || shown.String() != first+"他走进邮局。" {
				t.Fatalf("调用=%d，正文=%q", len(requests), shown.String())
			}
			if !strings.Contains(requests[1], first) || !strings.Contains(requests[1], prompts.ContinueNotice) {
				t.Fatal("续写丢失原文或指令")
			}
		})
	}
}

func TestAnthropicPauseNative(t *testing.T) {
	var requests []string
	var requestsMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		defer requestsMu.Unlock()
		b, _ := io.ReadAll(r.Body)
		requests = append(requests, string(b))
		w.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			_, _ = io.WriteString(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"OPAQUE"}}`+"\n\n"+`data: {"type":"content_block_stop","index":0}`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"pause_turn"}}`+"\n\n"+`data: {"type":"message_stop"}`+"\n\n")
			return
		}
		_, _ = io.WriteString(w, protocolText(llm.ProtocolAnthropic, "正文完成。", false))
	}))
	defer srv.Close()
	p, err := provider.NewAdapter(llm.ProviderConfig{Protocol: llm.ProtocolAnthropic, BaseURL: srv.URL, HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	e := setupWith(t, p)
	sv := choicesSave(t, e, "")
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if _, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputFree, Text: "开始"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	requestsMu.Lock()
	defer requestsMu.Unlock()
	if len(requests) != 2 || !strings.Contains(requests[1], `"data":"OPAQUE"`) || strings.Contains(requests[1], prompts.ContinueNotice) {
		t.Fatal("暂停续发没有原样携带原生内容")
	}
}

func TestDisconnectedUsage(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			_, _ = io.WriteString(w, `data: {"type":"message_start","message":{"usage":{"input_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":1}}}`+"\n\n"+`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"雾渐渐散开。"}}`+"\n\n")
			return
		}
		_, _ = io.WriteString(w, protocolText(llm.ProtocolAnthropic, "他走进邮局。", false))
	}))
	defer srv.Close()
	p, err := provider.NewAdapter(llm.ProviderConfig{Protocol: llm.ProtocolAnthropic, BaseURL: srv.URL, HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	e := setupWith(t, p)
	sv := choicesSave(t, e, "")
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if _, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputFree, Text: "开始"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	var u llm.Usage
	if err := e.db.Read().QueryRowContext(t.Context(), `SELECT SUM(input_uncached),SUM(cache_read),SUM(cache_write),SUM(output) FROM usage_log WHERE save_id=?`, sv.ID).Scan(&u.InputUncached, &u.CacheRead, &u.CacheWrite, &u.Output); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || u != (llm.Usage{InputUncached: 12, CacheRead: 100, CacheWrite: 50, Output: 6}) {
		t.Fatalf("断流用量丢失：%+v calls=%d", u, calls.Load())
	}
}
