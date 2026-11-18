package runtime_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/provider"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
)

// summaryProvider 只将维护工具之后的交接调用发给 HTTP 夹具。
type summaryProvider struct {
	llm.Provider
	summary llm.Provider
}

func (p summaryProvider) Stream(ctx context.Context, model *llm.Model, req *llm.Request) (llm.Stream, error) {
	if req.Purpose == "compact" && !strings.Contains(req.Messages[len(req.Messages)-1].TextOf(), `<system_task kind="compaction">`) {
		return p.summary.Stream(ctx, model, req)
	}
	return p.Provider.Stream(ctx, model, req)
}

func TestCompactionHTTPOutcome(t *testing.T) {
	const proto = llm.ProtocolOpenAIResponses
	fragment := protocolText(proto, "钥匙在", true)
	filtered := strings.ReplaceAll(fragment, "max_output_tokens", "content_filter")
	for _, tc := range []struct {
		name             string
		replies          []string
		summary, problem string
	}{
		{"半句后过滤", []string{filtered}, "", "内容过滤"},
		{"续写时过滤且无正文", []string{fragment, strings.ReplaceAll(filtered, "钥匙在", "")}, "", "内容过滤"},
		{"无结束事件", []string{`data: {"type":"response.output_text.delta","delta":"钥匙在"}` + "\n\n"}, "", "连接"},
		{"未知终止", []string{strings.ReplaceAll(fragment, "max_output_tokens", "new_reason")}, "", "没有正常结束"},
		{"空续写", []string{fragment, protocolText(proto, "", false)}, "", "未返回内容"},
		{"正常续写", []string{fragment, protocolText(proto, "桌上。", false)}, "钥匙在桌上。", ""},
		{"正常结束不审查语义", []string{protocolText(proto, "钥匙在", false)}, "钥匙在", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				i := int(calls.Add(1)) - 1
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, tc.replies[min(i, len(tc.replies)-1)])
			}))
			defer srv.Close()
			p, err := provider.NewAdapter(llm.ProviderConfig{Protocol: proto, BaseURL: srv.URL, HTTP: srv.Client()})
			if err != nil {
				t.Fatal(err)
			}
			e := setupWith(t, summaryProvider{fake.New(maintenanceScript()), p})
			sv := maintenanceSave(t, e)
			play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
			for range 4 {
				play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "继续"})
			}
			before, line, err := e.saves.ProgressLine(t.Context(), sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			old := line[len(line)-1]
			sub, _, _ := e.mgr.SubscribeEvents(sv.ID, 0)
			defer sub.Close()
			if err = e.mgr.Compact(t.Context(), sv.ID); err != nil {
				t.Fatal(err)
			}
			terminal := waitAny(t, sub, event.CompactionDone, event.CompactionFailed)
			after, line, err := e.saves.ProgressLine(t.Context(), sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			n := line[len(line)-1]
			if int(calls.Load()) != len(tc.replies) {
				t.Fatalf("交接请求次数：%d", calls.Load())
			}
			if tc.problem != "" {
				f, ok := terminal.Data.(event.CompactionFailedData)
				if !ok || !strings.Contains(f.Message, tc.problem) || f.RetryHead != old.ID || before.HeadNodeID != after.HeadNodeID || n.MemoryHash != old.MemoryHash || n.StateHash != old.StateHash {
					t.Fatalf("未回滚或未通知：%+v", terminal)
				}
				fresh, replay, _ := e.mgr.SubscribeEvents(sv.ID, 0)
				defer fresh.Close()
				snap := replay[0].Data.(runtime.SessionSnapshot)
				if snap.CompactFailure == nil || *snap.CompactFailure != f {
					t.Fatal("刷新丢失整理失败")
				}
				return
			}
			epoch, err := e.saves.Blob(t.Context(), n.EpochHash)
			if err != nil {
				t.Fatal(err)
			}
			if terminal.Type != event.CompactionDone || n.ParentID != old.ID || n.MemoryHash == old.MemoryHash || n.StateHash == old.StateHash || !strings.Contains(string(epoch), "<summary>\n"+tc.summary+"\n</summary>") {
				t.Fatal("未一起采用摘要、状态和记忆")
			}
		})
	}
}

func TestTurnFilteredOutcome(t *testing.T) {
	for _, text := range []string{"", "钥匙在"} {
		t.Run("正文："+text, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, strings.ReplaceAll(protocolText(llm.ProtocolOpenAIResponses, text, true), "max_output_tokens", "content_filter"))
			}))
			defer srv.Close()
			p, err := provider.NewAdapter(llm.ProviderConfig{Protocol: llm.ProtocolOpenAIResponses, BaseURL: srv.URL, HTTP: srv.Client()})
			if err != nil {
				t.Fatal(err)
			}
			ctx := t.Context()
			e := setupWith(t, p)
			sv := maintenanceSave(t, e)
			sub, _, _ := e.mgr.SubscribeEvents(sv.ID, 0)
			defer sub.Close()
			in := save.TurnInput{Kind: save.InputOpening}
			id, err := e.mgr.Submit(ctx, sv.ID, in)
			if err != nil {
				t.Fatal(err)
			}
			f := waitFor(t, sub, event.TurnFailed).Data.(event.TurnFailedData)
			if f.Code != "refusal" || f.TurnID != id || !strings.Contains(f.Message, "内容过滤") || !f.Recoverable {
				t.Fatalf("过滤原因丢失：%+v", f)
			}
			after, line, err := e.saves.ProgressLine(ctx, sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			if text == "" {
				if f.NodeID != "" || after.HeadNodeID != sv.HeadNodeID || f.Input.(save.TurnInput).Kind != in.Kind {
					t.Fatalf("无正文时进度或重试输入错误：%+v", f)
				}
			} else {
				n := line[len(line)-1]
				if after.HeadNodeID != id || n.Status != save.StatusInterrupted || n.Meta.StopRaw != "incomplete:content_filter" || f.NodeID != id || f.ParentID != sv.HeadNodeID {
					t.Fatalf("未按中断保存：%+v", n)
				}
			}
			another, replay, _ := e.mgr.SubscribeEvents(sv.ID, 0)
			defer another.Close()
			snap := replay[0].Data.(runtime.SessionSnapshot)
			if snap.Turn != nil || snap.Failure == nil || snap.Failure.Code != f.Code || snap.Failure.TurnID != id {
				t.Fatalf("快照丢失回合终态：%+v", snap)
			}
		})
	}
}

func TestRedoCompaction(t *testing.T) {
	script := maintenanceScript()
	e := setup(t, script)
	sv := maintenanceSave(t, e)
	ctx := t.Context()
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	var parent string
	for range 4 {
		parent = play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "继续"})
	}
	sub, _, _ := e.mgr.SubscribeEvents(sv.ID, 0)
	defer sub.Close()
	if err := e.mgr.Compact(ctx, sv.ID); err != nil {
		t.Fatal(err)
	}
	old := waitFor(t, sub, event.CompactionDone).Data.(event.CompactionDoneData).NodeID
	script.Compact[1].Stop = "refusal"
	if err := e.mgr.CompactAt(ctx, sv.ID, old, true); err != nil {
		t.Fatal(err)
	}
	f := waitFor(t, sub, event.CompactionFailed).Data.(event.CompactionFailedData)
	if !f.Redo || f.RetryHead != old {
		t.Fatalf("重试目标错误：%+v", f)
	}
	sv, err := e.saves.Get(ctx, sv.ID)
	if err != nil || sv.HeadNodeID != old {
		t.Fatal("重新整理失败覆盖了原节点")
	}
	script.Compact[1].Stop = "end"
	if err = e.mgr.CompactAt(ctx, sv.ID, old, true); err != nil {
		t.Fatal(err)
	}
	next := waitFor(t, sub, event.CompactionDone).Data.(event.CompactionDoneData).NodeID
	n, err := e.saves.Node(ctx, sv.ID, next)
	if err != nil || n.ParentID != parent || next == old {
		t.Fatal("未从原历史创建新的整理版本")
	}
	if _, err = e.saves.Node(ctx, sv.ID, old); err != nil {
		t.Fatal(err)
	}
	if err = e.mgr.CompactAt(ctx, sv.ID, old, true); !errs.Is(err, errs.CodeConflict) {
		t.Fatal("接受了过时的重新整理请求")
	}
}
