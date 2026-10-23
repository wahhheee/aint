package vfs

import (
	"context"
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	ctx := context.Background()
	tb := testTable()

	res, err := tb.Write(ctx, "/memory/characters/lin.md", []byte("# 林舟\n第 3 回合：收下了信\n"))
	if err != nil || !res.Created || res.Lines != 2 {
		t.Fatalf("新建 = %+v %v", res, err)
	}
	r, err := tb.Read(ctx, "/memory/characters/lin.md", 0, 0)
	if err != nil || r.Text != "# 林舟\n第 3 回合：收下了信" || r.Summary != "林舟" {
		t.Fatalf("写入后立即可读 = %+v %v", r, err)
	}
	if res, _ := tb.Write(ctx, "/memory/characters/lin.md", []byte("改写")); res.Created {
		t.Fatal("覆盖不是新建")
	}
	entries, _ := tb.List(ctx, "/memory", 2)
	if len(entries) != 3 { // characters/、characters/lin.md、index.md
		t.Fatalf("列目录 = %+v", entries)
	}

	if err := tb.Remove(ctx, "/memory/characters/lin.md"); err != nil {
		t.Fatal(err)
	}
	_, err = tb.Read(ctx, "/memory/characters/lin.md", 0, 0)
	_ = pathErr(t, err, ErrNotFound)
	if err := tb.Remove(ctx, "/memory/index.md"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := tb.List(ctx, "/memory", 1); len(entries) != 0 {
		t.Fatalf("删光后为空目录：%+v", entries)
	}
}

func TestWriteConstraints(t *testing.T) {
	ctx := context.Background()
	tb := testTable()

	for p, kind := range map[string]error{
		"/work/lore/x.md":                            ErrReadOnly,
		"/transcript/0001":                           ErrUnavailable,
		"/memory/notes.json":                         ErrInvalid,
		"/memory/a/b/c/d/e/f/g.md":                   ErrInvalid,
		"/memory/" + strings.Repeat("长", 81) + ".md": ErrInvalid,
		"/memory/index.md/x.md":                      ErrInvalid,
		"/memory/../work/x.md":                       ErrInvalid,
	} {
		_, err := tb.Write(ctx, p, []byte("x"))
		_ = pathErr(t, err, kind)
	}
	if _, err := tb.Write(ctx, "/memory/big.md", []byte(strings.Repeat("a", MaxMemoryFile+1))); err == nil {
		t.Fatal("单个文件超过上限应拒绝")
	} else {
		_ = pathErr(t, err, ErrTooLarge)
	}
	if _, err := tb.Write(ctx, "/memory/bin.md", []byte{0, 1, 2}); err == nil {
		t.Fatal("非文本内容应拒绝")
	}
	// 总量：逐个写入接近上限的文件，超出总量时拒绝
	chunk := []byte(strings.Repeat("a", MaxMemoryFile))
	var err error
	for i := 0; err == nil && i < 100; i++ {
		_, err = tb.Write(ctx, "/memory/bulk/"+strings.Repeat("x", i+1)+".md", chunk)
	}
	if pe := pathErr(t, err, ErrTooLarge); !strings.Contains(pe.Reason, "记忆总量") {
		t.Fatalf("总量上限：%v", pe.Reason)
	}
	// 写目录、删目录
	_, err = tb.Write(ctx, "/memory/bulk", []byte("x"))
	_ = pathErr(t, err, ErrInvalid) // 没有扩展名
	err = tb.Remove(ctx, "/memory/bulk")
	_ = pathErr(t, err, ErrIsDir)
}

func TestReadFile(t *testing.T) {
	ctx := context.Background()
	tb := testTable()
	data, err := tb.ReadFile(ctx, "/work/lore/characters/lin.md")
	if err != nil || string(data) != "# 林舟\r\n\r\n雾港的邮差。\r\n" {
		t.Fatalf("完整内容（去掉 BOM）= %q %v", data, err)
	}
	_, err = tb.ReadFile(ctx, "/work/assets/bgm/rain.ogg")
	_ = pathErr(t, err, ErrBinary)
}

func TestOverlay(t *testing.T) {
	o := NewOverlay(map[string]FileRef{"/memory/a.md": {Blob: "sha256:a", Size: 3, Turn: 1}}, 5)
	if o.Dirty() {
		t.Fatal("未写入时不脏")
	}
	o.Put("/memory/b.md", []byte("hello"))
	o.Remove("/memory/a.md")
	files, data := o.Result()
	if len(files) != 1 || files["/memory/b.md"].Turn != 5 || files["/memory/b.md"].Blob != ContentHash([]byte("hello")) {
		t.Fatalf("清单 = %+v", files)
	}
	if string(data["/memory/b.md"]) != "hello" || o.TotalSize() != 5 {
		t.Fatalf("内容 = %q，总量 %d", data["/memory/b.md"], o.TotalSize())
	}
	written, removed := o.Changes()
	if len(written) != 1 || len(removed) != 1 {
		t.Fatalf("改动 = %v %v", written, removed)
	}
	// 写入后再删除本回合新建的文件：不留痕迹
	o.Remove("/memory/b.md")
	if files, _ := o.Result(); len(files) != 0 {
		t.Fatalf("清单 = %+v", files)
	}
}
