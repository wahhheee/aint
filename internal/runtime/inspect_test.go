package runtime_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/llm/openai"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/save"
)

func TestDebugSnapshotsAndToggle(t *testing.T) {
	e := setup(t, &fake.Script{Responses: []fake.Response{
		{DelayMS: 1, ToolCalls: []fake.ToolCall{{Name: "write", Args: map[string]any{"path": "/memory/debug.md", "content": "调用时的记忆"}}}},
		{DelayMS: 1, Text: "故事继续。"},
		{DelayMS: 1, ToolCalls: []fake.ToolCall{{Name: "remove", Args: map[string]any{"path": "/memory/debug.md"}}}},
		{DelayMS: 1, Text: "新的篇章。"},
	}})
	ctx := t.Context()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.first-story"})
	if err != nil {
		t.Fatal(err)
	}
	if e.mgr.Debug(sv.ID).Enabled {
		t.Fatal("调试默认应关闭")
	}
	e.mgr.SetDebug(sv.ID, true)
	id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	status := e.mgr.Debug(sv.ID)
	if len(status.Calls) != 2 || status.Calls[0].TurnID != id {
		t.Fatalf("调用列表：%+v", status)
	}
	call, err := e.mgr.DebugCall(sv.ID, status.Calls[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(call.Tools) != 1 || call.Tools[0].Name != "write" || call.ToolCount != 1 || call.Status != "complete" {
		t.Fatalf("工具详情：%+v", call)
	}
	if len(call.HTTP) != 0 {
		t.Fatal("假模型没有 HTTP 请求")
	}
	if _, err := e.mgr.DebugFile(ctx, sv.ID, call.ID, "before", "/memory/debug.md", 1); err == nil {
		t.Fatal("调用前不应存在新记忆")
	}
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "往前走"})
	file, err := e.mgr.DebugFile(ctx, sv.ID, call.ID, "after", "/memory/debug.md", 1)
	if err != nil || file.Text != "调用时的记忆" {
		t.Fatalf("之后的删除不应改变旧快照：%+v %v", file, err)
	}
	if _, err := e.mgr.DebugCall("another-save", call.ID); err == nil {
		t.Fatal("不应跨存档读取记录")
	}
	if _, err := e.mgr.DebugFile(ctx, sv.ID, call.ID, "after", "/memory/../../secrets.json", 1); err == nil {
		t.Fatal("不应读取宿主路径")
	}
	e.mgr.SetDebug(sv.ID, false)
	n := len(e.mgr.Debug(sv.ID).Calls)
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "继续"})
	if len(e.mgr.Debug(sv.ID).Calls) != n {
		t.Fatal("关闭后不应记录新调用")
	}
	e.mgr.ClearDebug(sv.ID)
	if len(e.mgr.Debug(sv.ID).Calls) != 0 {
		t.Fatal("清空后仍有记录")
	}
}

func TestDebugCancelAndCompaction(t *testing.T) {
	t.Run("取消保留失败记录", func(t *testing.T) {
		gate := make(chan struct{})
		e := setupWith(t, gatedProvider{Provider: fake.New(quickScript()), open: gate})
		sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "yanyi.first-story"})
		if err != nil {
			t.Fatal(err)
		}
		e.mgr.SetDebug(sv.ID, true)
		sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
		defer sub.Close()
		id, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputOpening})
		if err != nil {
			t.Fatal(err)
		}
		waitFor(t, sub, event.TurnStarted)
		deadline := time.Now().Add(2 * time.Second)
		for len(e.mgr.Debug(sv.ID).Calls) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if err := e.mgr.Cancel(sv.ID, id); err != nil {
			t.Fatal(err)
		}
		waitFor(t, sub, event.TurnFailed)
		s := e.mgr.Debug(sv.ID)
		if len(s.Calls) != 1 || s.Calls[0].Status != "error" || s.Calls[0].Error == "" {
			t.Fatalf("取消记录：%+v", s)
		}
	})
	t.Run("整理工具与快照", func(t *testing.T) {
		e := setup(t, compactScript())
		sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "yanyi.first-story", Settings: save.Settings{ContextBudget: 100000, KeepRecent: 100}})
		if err != nil {
			t.Fatal(err)
		}
		for i := range 5 {
			in := save.TurnInput{Kind: save.InputFree, Text: "继续"}
			if i == 0 {
				in.Kind = save.InputOpening
			}
			play(t, e, sv.ID, in)
		}
		e.mgr.SetDebug(sv.ID, true)
		sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
		defer sub.Close()
		if err := e.mgr.Compact(t.Context(), sv.ID); err != nil {
			t.Fatal(err)
		}
		waitFor(t, sub, event.CompactionDone)
		s := e.mgr.Debug(sv.ID)
		if len(s.Calls) != 2 || s.Calls[0].Purpose != "compact" {
			t.Fatalf("整理记录：%+v", s)
		}
		c, err := e.mgr.DebugCall(sv.ID, s.Calls[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(c.Tools) != 2 || !c.Tools[1].IsError {
			t.Fatalf("整理禁用工具应留记录：%+v", c.Tools)
		}
		f, err := e.mgr.DebugFile(t.Context(), sv.ID, c.ID, "after", "/memory/chronicle/01.md", 1)
		if err != nil || !strings.Contains(f.Text, "雾夜") {
			t.Fatalf("整理后的 VFS：%+v %v", f, err)
		}
	})
}

// 真实录制夹具验证从 HTTP 报文到可读推理、工具执行的完整调试链路。
func TestDebugHTTPRetriesAndReasoning(t *testing.T) {
	fixture, err := os.ReadFile("../llm/openai/testdata/deepseek-parallel-tools.sse")
	if err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	var observedRequest []byte
	errorBody := `{"error":{"message":"稍后再试"},"padding":"` + strings.Repeat("x", 70<<10) + `"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if attempts.Add(1) == 1 {
			w.Header().Set("Set-Cookie", "private-session")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, errorBody)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-ID", "recording-123")
		if attempts.Load() == 2 {
			observedRequest = body
			// 把中文的 UTF-8 字节分开传输，调试原文必须可无损拼接。
			for _, b := range fixture {
				_, _ = w.Write([]byte{b})
				w.(http.Flusher).Flush()
			}
			return
		}
		_, _ = io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"结束\"}]}]}}\n\n")
	}))
	defer srv.Close()
	p := openai.New(llm.ProviderConfig{BaseURL: srv.URL + "/v1", APIKey: "private-key", Headers: map[string]string{"X-Trace-Token": "private-token"}, HTTP: srv.Client()})
	e := setupWith(t, llm.WithRetry(p, llm.RetryPolicy{MaxRetries: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}))
	sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "yanyi.first-story"})
	if err != nil {
		t.Fatal(err)
	}
	e.mgr.SetDebug(sv.ID, true)
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	status := e.mgr.Debug(sv.ID)
	if len(status.Calls) < 1 {
		t.Fatal("没有调用记录")
	}
	call, err := e.mgr.DebugCall(sv.ID, status.Calls[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(call.HTTP) != 2 || call.HTTP[0].Status != 429 || call.HTTP[1].Status != 200 {
		t.Fatalf("重试记录：%+v", call.HTTP)
	}
	if !call.HTTP[0].Truncated || !strings.HasPrefix(errorBody, call.HTTP[0].ResponseBody) {
		t.Fatal("超长错误响应应保留原文前缀并标记截断")
	}
	if call.HTTP[1].RequestBody != string(observedRequest) || call.HTTP[1].ResponseBody != string(fixture) {
		t.Fatal("原始报文与实际收发不一致")
	}
	if call.HTTP[1].RequestHeaders.Get("User-Agent") == "" {
		t.Fatal("缺少传输层补充的请求头")
	}
	b := string(jsonx.MustMarshal(call))
	for _, secret := range []string{"private-key", "private-token", "private-session"} {
		if strings.Contains(b, secret) {
			t.Fatalf("泄漏鉴权信息 %s", secret)
		}
	}
	reasoning := ""
	for _, s := range call.Steps {
		if s.Kind == "reasoning" {
			reasoning += s.Text
		}
	}
	if reasoning == "" {
		t.Fatal("DeepSeek 明文推理未记录")
	}
	if len(call.Tools) != 2 {
		t.Fatalf("工具执行记录：%+v", call.Tools)
	}
}
