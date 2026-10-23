package vfs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeTranscript 是实现了 Searcher 的原文来源：每个回合一个文件，检索时按回合号新→旧给出 Order。
type fakeTranscript struct{ turns map[int]string }

func (s fakeTranscript) Files(context.Context) ([]File, error) {
	var out []File
	for no := range s.turns {
		out = append(out, File{Path: fmt.Sprintf("/transcript/%04d", no)})
	}
	return out, nil
}

func (s fakeTranscript) Open(_ context.Context, f File) ([]byte, error) {
	var no int
	_, _ = fmt.Sscanf(f.Path, "/transcript/%04d", &no)
	return []byte(s.turns[no]), nil
}

func (s fakeTranscript) Search(_ context.Context, m *Matcher, prefix string) ([]Hit, error) {
	var hits []Hit
	for no, text := range s.turns {
		p := fmt.Sprintf("/transcript/%04d", no)
		if prefix != "/" && prefix != "/transcript" && prefix != p {
			continue
		}
		for _, h := range ScanLines(p, Lines([]byte(text)), m) {
			h.Order = -no
			hits = append(hits, h)
		}
	}
	return hits, nil
}

func searchTable() *Table {
	work := &memSource{
		files: []File{
			{Path: "/work/lore/characters/lin.md", Names: []string{"lin", "林舟", "阿舟", "邮差"}},
			{Path: "/work/lore/key.md"},
			{Path: "/work/assets/bg/harbor.webp"},
		},
		data: map[string][]byte{
			"/work/lore/characters/lin.md": []byte("# 林舟\n雾港的邮差，大家叫他阿舟。\n"),
			"/work/lore/key.md":            []byte("银色钥匙能打开灯塔的门。\n"),
		},
	}
	o := NewOverlay(map[string]FileRef{}, 1)
	o.Put("/memory/threads/key.md", []byte("第 12 回合林舟拾到银色钥匙\n钥匙的事没告诉任何人\n"))
	var many strings.Builder
	for range 8 {
		many.WriteString("银色钥匙\n")
	}
	o.Put("/memory/many.md", []byte(many.String()))
	memory := NewMemorySource(o, func(context.Context, string) ([]byte, error) { return nil, nil })
	transcript := fakeTranscript{turns: map[int]string{
		12: "# 第 12 回合\n他弯腰，从缆绳下抽出一把银色钥匙，锈迹里还有海盐。",
		57: "# 第 57 回合\n「那把银色钥匙，」老板娘压低声音。",
		60: "# 第 60 回合\n阿舟没有说话。",
	}}
	return NewTable(
		Mount{Root: "/work", Title: "作品资料", Source: work},
		Mount{Root: "/memory", Title: "记忆", Source: memory},
		Mount{Root: "/transcript", Title: "原文", Source: transcript},
	)
}

func hitPaths(r *SearchResult) string {
	var out []string
	for _, h := range r.Hits {
		out = append(out, fmt.Sprintf("%s:%d", h.Path, h.Line))
	}
	return strings.Join(out, ",")
}

func TestSearchOrder(t *testing.T) {
	ctx := context.Background()
	tb := searchTable()
	r, err := tb.Search(ctx, Query{Text: "银色钥匙"})
	if err != nil {
		t.Fatal(err)
	}
	// 记忆 > 作品资料 > 原文（新→旧）；每个文件最多 5 条
	want := "/memory/many.md:1,/memory/many.md:2,/memory/many.md:3,/memory/many.md:4,/memory/many.md:5," +
		"/memory/threads/key.md:1,/work/lore/key.md:1,/transcript/0057:2,/transcript/0012:2"
	if got := hitPaths(r); got != want {
		t.Fatalf("排序：\n%s\n%s", got, want)
	}
	if r.Total != 12 || !r.Truncated || r.Corpora != 3 {
		t.Fatalf("总数 %d、截断 %v、语料 %d", r.Total, r.Truncated, r.Corpora)
	}
	if s := r.Hits[len(r.Hits)-1].Snippet; s != "他弯腰，从缆绳下抽出一把【银色钥匙】，锈迹里还有海盐。" {
		t.Fatalf("片段：%q", s)
	}

	// 条数上限
	if r, _ := tb.Search(ctx, Query{Text: "银色钥匙", Limit: 2}); len(r.Hits) != 2 || !r.Truncated {
		t.Fatalf("limit：%s", hitPaths(r))
	}
	// 路径前缀
	if r, _ := tb.Search(ctx, Query{Text: "银色钥匙", Prefix: "/transcript"}); hitPaths(r) != "/transcript/0057:2,/transcript/0012:2" {
		t.Fatalf("前缀 /transcript：%s", hitPaths(r))
	}
	if r, _ := tb.Search(ctx, Query{Text: "钥匙", Prefix: "/memory/threads"}); hitPaths(r) != "/memory/threads/key.md:1,/memory/threads/key.md:2" {
		t.Fatalf("前缀 /memory/threads：%s", hitPaths(r))
	}
	if r, _ := tb.Search(ctx, Query{Text: "钥匙", Prefix: "/transcript/0012"}); hitPaths(r) != "/transcript/0012:2" {
		t.Fatalf("前缀 /transcript/0012：%s", hitPaths(r))
	}
	if _, err := tb.Search(ctx, Query{Text: "钥匙", Prefix: "/etc"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的前缀：%v", err)
	}
}

func TestSearchAliasAndRegex(t *testing.T) {
	ctx := context.Background()
	tb := searchTable()

	r, err := tb.Search(ctx, Query{Text: "林舟", Prefix: "/transcript"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.Aliases, ",") != "阿舟,邮差" || hitPaths(r) != "/transcript/0060:2" {
		t.Fatalf("别名扩展：%v %s", r.Aliases, hitPaths(r))
	}
	if r, _ := tb.Search(ctx, Query{Text: "LIN"}); len(r.Aliases) != 3 {
		t.Fatalf("查询词是文档 ID 时也扩展（不区分大小写）：%v", r.Aliases)
	}

	r, err = tb.Search(ctx, Query{Text: `第\s*\d+\s*回合`, Regex: true, Prefix: "/memory"})
	if err != nil || hitPaths(r) != "/memory/threads/key.md:1" {
		t.Fatalf("正则：%s %v", hitPaths(r), err)
	}
	if r.Hits[0].Snippet != "【第 12 回合】林舟拾到银色钥匙" {
		t.Fatalf("正则片段：%q", r.Hits[0].Snippet)
	}
	if _, err := tb.Search(ctx, Query{Text: "a(b", Regex: true}); !errors.Is(err, ErrBadQuery) {
		t.Fatalf("正则写错：%v", err)
	}
	if _, err := tb.Search(ctx, Query{Text: "  "}); !errors.Is(err, ErrBadQuery) {
		t.Fatalf("空查询：%v", err)
	}
}

func TestSnippet(t *testing.T) {
	long := strings.Repeat("前", 100) + "钥匙" + strings.Repeat("后", 100)
	loc := [][]int{{len(strings.Repeat("前", 100)), len(strings.Repeat("前", 100)) + len("钥匙")}}
	s := snippet(long, loc)
	if s != "…"+strings.Repeat("前", snippetRadius)+"【钥匙】"+strings.Repeat("后", snippetRadius)+"…" {
		t.Fatalf("片段：%q", s)
	}
}
