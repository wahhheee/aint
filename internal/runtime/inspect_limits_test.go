package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/vfs"
)

func TestDebugRetentionAndClearInFlight(t *testing.T) {
	m := NewManager(Deps{})
	m.SetDebug("s", true)
	p := fake.New(&fake.Script{})
	start := func() *debugRecorder {
		return m.debugStart(t.Context(), "s", "turn", 1, p, fake.Model(), &llm.Request{}, nil, nil)
	}
	for range debugMaxCalls + 1 {
		start().Finish(nil)
	}
	if s := m.Debug("s"); len(s.Calls) != debugMaxCalls || s.Dropped != 1 {
		t.Fatalf("条数限制：%+v", s)
	}
	r := start()
	r.Event(llm.TextDelta{Text: strings.Repeat("x", debugCallBytes+1)})
	r.Finish(errors.New("连接中断"))
	call, err := m.DebugCall("s", r.data.ID)
	if err != nil || !call.Truncated || call.Status != "error" || r.size > debugCallBytes {
		t.Fatalf("单次限额：%+v %v", call.DebugSummary, err)
	}
	m.ClearDebug("s")
	r.Event(llm.TextDelta{Text: "晚到的事件"})
	if len(m.Debug("s").Calls) != 0 || m.debug.bytes != 0 {
		t.Fatal("清空后在途事件复活了记录")
	}
	for range 20 {
		rec := start()
		rec.Event(llm.TextDelta{Text: strings.Repeat("x", debugCallBytes/2)})
		rec.Finish(nil)
	}
	if m.debug.bytes > debugMaxBytes {
		t.Fatal("全局限额失效")
	}
	if _, err := m.DebugFile(context.Background(), "s", "none", "wrong", "/memory/x", 1); err == nil {
		t.Fatal("非法快照阶段应被拒绝")
	}
}

func TestDebugFilePagination(t *testing.T) {
	ctx := t.Context()
	m := NewManager(Deps{})
	m.SetDebug("s", true)
	overlay := vfs.NewOverlay(nil, 1)
	fs := vfs.NewTable(vfs.Mount{Root: "/memory", Source: vfs.NewMemorySource(overlay, nil)})
	content := strings.Repeat("a", 19999) + "🙂中文" + strings.Repeat("b", 300)
	if _, err := fs.Write(ctx, "/memory/long.md", []byte(content)); err != nil {
		t.Fatal(err)
	}
	r := m.debugStart(ctx, "s", "turn", 1, fake.New(nil), fake.Model(), &llm.Request{}, fs, nil)
	overlay.Remove("/memory/long.md")
	first, err := m.DebugFile(ctx, "s", r.data.ID, "before", "/memory/long.md", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.DebugFile(ctx, "s", r.data.ID, "before", "/memory/long.md", first.ToChar+1)
	if err != nil {
		t.Fatal(err)
	}
	if first.ToChar != 20000 || first.TotalChars != 20302 || first.Text+second.Text != content {
		t.Fatal("单行长文本分页必须保留每个字符，包括跨页的中文与 emoji")
	}
	if _, err := m.DebugFile(ctx, "s", r.data.ID, "before", "/memory/long.md", second.ToChar+1); err == nil {
		t.Fatal("超出文件范围应返回错误")
	}
}
