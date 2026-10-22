package vfs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// memSource 是测试用的内存来源。
type memSource struct {
	files []File
	data  map[string][]byte
}

func (s *memSource) Files(context.Context) ([]File, error) { return s.files, nil }
func (s *memSource) Open(_ context.Context, f File) ([]byte, error) {
	if d, ok := s.data[f.Path]; ok {
		return d, nil
	}
	return nil, &PathError{Path: f.Path, Err: ErrNotFound}
}

func testTable() *Table {
	long := strings.Repeat("长", MaxReadRunes+100)
	var many strings.Builder
	for i := 1; i <= 1200; i++ {
		fmt.Fprintf(&many, "%d\n", i)
	}
	work := &memSource{
		files: []File{
			{Path: "/work/lore", IsDir: true, Summary: "设定"},
			{Path: "/work/lore/characters/lin.md", Summary: "林舟：邮差", Names: []string{"lin", "林舟"}},
			{Path: "/work/lore/places/harbor.md", Summary: "雾港"},
			{Path: "/work/lore/long.md"},
			{Path: "/work/lore/many.md"},
			{Path: "/work/assets/bg/harbor.webp", Blob: "sha256:img", Size: 10, Summary: "夜里的港口"},
			{Path: "/work/assets/bgm/rain.ogg", Summary: "雨声"},
		},
		data: map[string][]byte{
			"/work/lore/characters/lin.md": []byte("\ufeff# 林舟\r\n\r\n雾港的邮差。\r\n"),
			"/work/lore/places/harbor.md":  []byte("雾港\n"),
			"/work/lore/long.md":           []byte(long + "\n第二行\n"),
			"/work/lore/many.md":           []byte(many.String()),
		},
	}
	memory := NewMemorySource(NewOverlay(map[string]FileRef{"/memory/index.md": {Blob: "b1", Size: 12}}),
		func(_ context.Context, blob string) ([]byte, error) {
			return []byte("---\nsummary: 总索引\n---\n- a\n"), nil
		})
	return NewTable(
		Mount{Root: "/work", Title: "作品资料", Source: work},
		Mount{Root: "/memory", Title: "记忆", Source: memory},
		Mount{Root: "/transcript", Title: "原文"},
	)
}

func pathErr(t *testing.T, err error, kind error) *PathError {
	t.Helper()
	var pe *PathError
	if !errors.As(err, &pe) || !errors.Is(err, kind) {
		t.Fatalf("期望 %v，实际 %v", kind, err)
	}
	return pe
}

func TestList(t *testing.T) {
	ctx := context.Background()
	tb := testTable()

	root, err := tb.List(ctx, "/", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(root) != 3 || root[0].Path != "/work" || root[0].Files != 6 || !root[2].Unavailable {
		t.Fatalf("根目录 = %+v", root)
	}
	lore, err := tb.List(ctx, "/work/lore", 2)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range lore {
		got = append(got, fmt.Sprintf("%d:%s", e.Depth, e.Path))
	}
	want := "1:/work/lore/characters,2:/work/lore/characters/lin.md,1:/work/lore/places,2:/work/lore/places/harbor.md,1:/work/lore/long.md,1:/work/lore/many.md"
	if strings.Join(got, ",") != want {
		t.Fatalf("目录在前、文件在后：\n%s\n%s", strings.Join(got, ","), want)
	}
	if mem, _ := tb.List(ctx, "/memory", 1); len(mem) != 1 || mem[0].Summary != "总索引" {
		t.Fatalf("记忆目录 = %+v", mem)
	}
	_, err = tb.List(ctx, "/work/lore/places/harbor.md", 1)
	_ = pathErr(t, err, ErrNotDir)
}

func TestRead(t *testing.T) {
	ctx := context.Background()
	tb := testTable()

	r, err := tb.Read(ctx, "/work/lore/characters/lin.md", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "# 林舟\n\n雾港的邮差。" || r.FromLine != 1 || r.ToLine != 3 || r.TotalLines != 3 {
		t.Fatalf("去 BOM、统一换行：%+v", r)
	}
	r, _ = tb.Read(ctx, "/work/lore/many.md", 1195, 0)
	if r.FromLine != 1195 || r.ToLine != 1200 || !strings.HasPrefix(r.Text, "1195\n1196") {
		t.Fatalf("行范围：%+v", r)
	}
	r, _ = tb.Read(ctx, "/work/lore/many.md", 0, 5000)
	if r.ToLine != MaxReadLines {
		t.Fatalf("limit 上限：%d", r.ToLine)
	}
	// 字数到达上限时在整行处结束（第 2 行之后放不下）
	var wide strings.Builder
	for range 3 {
		wide.WriteString(strings.Repeat("宽", MaxReadRunes/2-10) + "\n")
	}
	tb2 := NewTable(Mount{Root: "/m", Source: &memSource{files: []File{{Path: "/m/w.md"}}, data: map[string][]byte{"/m/w.md": []byte(wide.String())}}})
	if r, _ := tb2.Read(ctx, "/m/w.md", 0, 0); r.ToLine != 2 || r.LineCut || r.TotalLines != 3 {
		t.Fatalf("按字数截断：%+v", r)
	}
	r, _ = tb.Read(ctx, "/work/lore/long.md", 0, 0)
	if !r.LineCut || r.ToLine != 1 || len([]rune(r.Text)) != MaxReadRunes {
		t.Fatalf("超长行截断：to=%d cut=%v len=%d", r.ToLine, r.LineCut, len([]rune(r.Text)))
	}
	_, err = tb.Read(ctx, "/work/lore/many.md", 5000, 0)
	if pe := pathErr(t, err, ErrOutOfRange); pe.Total != 1200 {
		t.Fatalf("总行数 = %d", pe.Total)
	}

	img, err := tb.Read(ctx, "/work/assets/bg/harbor.webp", 0, 0)
	if err != nil || img.Image == nil || img.Image.MIME != "image/webp" || img.Summary != "夜里的港口" {
		t.Fatalf("图片 = %+v %v", img, err)
	}
	_, err = tb.Read(ctx, "/work/assets/bgm/rain.ogg", 0, 0)
	_ = pathErr(t, err, ErrBinary)

	_, err = tb.Read(ctx, "/work/lore", 0, 0)
	if pe := pathErr(t, err, ErrIsDir); len(pe.Children) == 0 {
		t.Fatal("读目录应列出子项")
	}
	mem, err := tb.Read(ctx, "/memory/index.md", 0, 0)
	if err != nil || !strings.Contains(mem.Text, "summary: 总索引") {
		t.Fatalf("记忆 = %+v %v", mem, err)
	}
}

func TestPathErrors(t *testing.T) {
	ctx := context.Background()
	tb := testTable()

	for _, p := range []string{"/work/../secrets.json", "/memory/../../x", ""} {
		_, err := tb.Read(ctx, p, 0, 0)
		_ = pathErr(t, err, ErrInvalid)
	}
	// 漏写开头的 /：给出存在的完整路径
	_, err := tb.Read(ctx, "work/lore/places/harbor.md", 0, 0)
	if pe := pathErr(t, err, ErrInvalid); len(pe.Similar) != 1 || pe.Similar[0] != "/work/lore/places/harbor.md" {
		t.Fatalf("建议 = %v", pe.Similar)
	}
	// 不在任何挂载点下
	_, err = tb.Read(ctx, "/etc/passwd", 0, 0)
	if pe := pathErr(t, err, ErrNotFound); len(pe.Roots) != 3 {
		t.Fatalf("应列出可访问的目录：%+v", pe)
	}
	// 大小写、漏扩展名、用了别名
	for _, tc := range []struct{ in, want string }{
		{"/work/lore/characters/LIN.md", "/work/lore/characters/lin.md"},
		{"/work/lore/places/harbor", "/work/lore/places/harbor.md"},
		{"/work/lore/林舟", "/work/lore/characters/lin.md"},
	} {
		_, err := tb.Read(ctx, tc.in, 0, 0)
		pe := pathErr(t, err, ErrNotFound)
		if len(pe.Similar) == 0 || pe.Similar[0] != tc.want {
			t.Errorf("%s：建议 %v，期望 %s", tc.in, pe.Similar, tc.want)
		}
	}
	// 已规划但暂不可用的挂载点
	_, err = tb.Read(ctx, "/transcript/0001", 0, 0)
	_ = pathErr(t, err, ErrUnavailable)
}
