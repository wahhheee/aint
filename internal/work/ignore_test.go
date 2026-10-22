package work

import "testing"

func TestIgnoreRules(t *testing.T) {
	ig := ParseIgnore([]byte("\xEF\xBB\xBF# 注释\r\n" +
		"\n" +
		"*.psd\n" +
		"!keep.psd\n" +
		"drafts/\n" +
		"!drafts/final.md\n" + // 目录已被忽略，不能再找回（与 git 一致）
		"/notes.md\n" +
		"docs/**\n" +
		"!docs/keep.md\n" +
		"**/tmp\n" +
		"a/**/z.txt\n" +
		"\\#literal\n" +
		"trailing   \n" +
		"img[0-9].png\n" +
		"x?.txt\n" +
		"[!a]b.txt\n" +
		"build/out/\n"))

	cases := []struct {
		path string
		want bool
	}{
		{"art.psd", true},
		{"assets/deep/art.psd", true},
		{"keep.psd", false},
		{"assets/keep.psd", false},
		{"drafts/a.md", true},
		{"lore/drafts/a.md", true},
		{"drafts/final.md", true},
		{"drafts.md", false}, // 只匹配目录
		{"notes.md", true},
		{"lore/notes.md", false}, // 锚定在作品根
		{"docs/a.md", true},
		{"docs/sub/b.md", true},
		{"docs/keep.md", false},
		{"tmp", true},
		{"x/y/tmp/f", true},
		{"a/z.txt", true},
		{"a/b/c/z.txt", true},
		{"b/a/z.txt", false},
		{"#literal", true},
		{"trailing", true},
		{"img3.png", true},
		{"imgx.png", false},
		{"x1.txt", true},
		{"x12.txt", false},
		{"bb.txt", true},
		{"ab.txt", false},
		{"build/out/a.js", true},
		{"src/build/out/a.js", false}, // 中间有 "/"：锚定
		{"work.yaml", false},
	}
	for _, c := range cases {
		if got := ig.Ignored(c.path); got != c.want {
			t.Errorf("Ignored(%q) = %v，应为 %v", c.path, got, c.want)
		}
	}
}

func TestIgnoreEmpty(t *testing.T) {
	var nilIgnore *Ignore
	if nilIgnore.Ignored("a.md") || ParseIgnore(nil).Ignored("a.md") || ParseIgnore([]byte("# 只有注释\n\n")).Ignored("a.md") {
		t.Fatal("没有规则时不应忽略任何文件")
	}
	// 不完整的字符类按字面量处理，不应导致整份规则失效
	if !ParseIgnore([]byte("[abc\n*.bak\n")).Ignored("x.bak") {
		t.Fatal("无效行之后的规则应仍然有效")
	}
}
