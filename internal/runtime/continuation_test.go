package runtime

import (
	"strings"
	"testing"
)

func TestDedupOverlap(t *testing.T) {
	prev := "雾越来越浓，林舟把帽子挂上衣架，水珠沿着帽檐往下落。"
	cases := []struct {
		name, next, want string
	}{
		{"重复了末尾一句", "水珠沿着帽檐往下落。他转过身来。", "他转过身来。"},
		{"没有重复", "他转过身来。", "他转过身来。"},
		{"太短的重合不算重复", "落。好像有人来过。", "落。好像有人来过。"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.next[dedupOverlap(prev, c.next):]; got != c.want {
				t.Fatalf("得到 %q，应为 %q", got, c.want)
			}
		})
	}
}

func TestOverlapFilter(t *testing.T) {
	prev := strings.Repeat("前文。", 100) + "水珠沿着帽檐往下落。"
	f := newOverlapFilter(prev)

	// 逐字送入：缓冲满之前不放行，满了之后去掉重复再放行，之后原样通过。
	next := "水珠沿着帽檐往下落。" + strings.Repeat("续写", 120)
	var shown strings.Builder
	for _, r := range next {
		shown.WriteString(f.push(string(r)))
	}
	shown.WriteString(f.flush())
	if want := strings.Repeat("续写", 120); shown.String() != want {
		t.Fatalf("显示的续写不对：%q…", shown.String()[:min(30, shown.Len())])
	}

	// 流在缓冲满之前结束：flush 时同样去重。
	f = newOverlapFilter(prev)
	if got := f.push("水珠沿着帽檐往下落。完。"); got != "" {
		t.Fatalf("缓冲期间不应放行：%q", got)
	}
	if got := f.flush(); got != "完。" {
		t.Fatalf("流结束时应去重后放行：%q", got)
	}

	// 上一段没有正文：不去重。
	if f := newOverlapFilter(""); f.push("原样") != "原样" || f.flush() != "" {
		t.Fatal("nil 过滤器应原样放行")
	}
}
