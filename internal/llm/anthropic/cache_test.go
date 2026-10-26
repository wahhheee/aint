package anthropic

import (
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

func TestCacheLongResponse(t *testing.T) {
	for _, n := range []int{18, 19, 25} {
		req := &llm.Request{System: []llm.SystemBlock{{Text: "系统", CacheMark: true}}, Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{{Kind: llm.PartText, Text: "常驻", CacheMark: true}, {Kind: llm.PartText, Text: "阶段", CacheMark: true}, llm.Text("上次输入")}},
			{Role: llm.RoleAssistant}, {Role: llm.RoleUser, Parts: []llm.Part{llm.Text("最新输入")}},
		}}
		for range n {
			req.Messages[1].Parts = append(req.Messages[1].Parts, llm.Text("正文"))
		}
		b, err := New(llm.ProviderConfig{}).Body(t.Context(), &llm.Model{Caps: llm.Caps{Cache: "breakpoints"}}, req)
		if err != nil {
			t.Fatal(err)
		}
		var out request
		if err := jsonx.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		if got := hasCache(out.Messages[0].Content[2]); got != (n >= 19) {
			t.Fatalf("新增 %d 个位置，锚点=%v", n+1, got)
		}
		if strings.Count(string(b), `"cache_control"`) > 4 {
			t.Fatal("缓存断点超过四个")
		}
		if !hasCache(out.Messages[2].Content[0]) || !hasCache(out.System[0]) {
			t.Fatal("缺少系统或最新断点")
		}
	}
}

func TestCacheSkipsThinking(t *testing.T) {
	req := &llm.Request{Messages: []llm.Message{
		{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("输入")}},
		{Role: llm.RoleAssistant, Native: map[string]jsonx.Value{"anthropic/m": jsonx.Value(`[{"type":"redacted_thinking","data":"OPAQUE"}]`)}},
	}}
	b, err := New(llm.ProviderConfig{}).Body(t.Context(), &llm.Model{Name: "m", Caps: llm.Caps{Cache: "breakpoints"}}, req)
	if err != nil {
		t.Fatal(err)
	}
	var out request
	if err := jsonx.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !hasCache(out.Messages[0].Content[0]) || hasCache(out.Messages[1].Content[0]) {
		t.Fatal("断点应回退到可缓存块")
	}
}
