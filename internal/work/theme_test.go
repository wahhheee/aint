package work

import (
	"strings"
	"testing"
)

func assetLookup(ref string) (string, bool) {
	if ref == "fog" {
		return "/api/v1/assets/abc", true
	}
	return "", false
}

func TestScopeCSS(t *testing.T) {
	src := `/* 作品样式 */
.mk-gold { text-shadow: 0 0 6px gold; animation: shimmer 2s infinite; }
@media (max-width: 600px) { .prose-book { font-size: 16px; } }
@keyframes shimmer { from { opacity: .6 } to { opacity: 1 } }
.paper { background: url(asset:fog) center / cover; }
@charset "utf-8";
`
	out, issues := ScopeCSS(src, assetLookup)
	if len(issues) > 0 {
		t.Fatalf("不应有问题：%+v", issues)
	}
	for _, want := range []string{
		"@keyframes wk-shimmer {",
		".work-root {\n.mk-gold",
		"animation: wk-shimmer 2s infinite",
		`url("/api/v1/assets/abc")`,
		"@media (max-width: 600px)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q：\n%s", want, out)
		}
	}
	if strings.Contains(out, "@charset") || strings.Index(out, "@keyframes") > strings.Index(out, ".work-root {") {
		t.Fatalf("@charset 应去掉，@keyframes 应提到作用域之外：\n%s", out)
	}
}

func TestScopeCSSRejects(t *testing.T) {
	cases := []struct{ src, want string }{
		{`@import url("https://x/a.css");`, "不支持 @import"},
		{`.a { background: url(https://x/a.png) }`, "只能引用作品素材"},
		{`.a { background: url("asset:nope") }`, "引用的素材不存在"},
		{`:root { --paper: red }`, "不允许使用 :root"},
		{`.a { color: red } }`, "多了一个 }"},
		{`.a { color: red`, "不配对"},
		{`.a { background: u\72 l(x) }`, "反斜杠"},
		{`.a { content: "abc }`, "字符串没有结束"},
		{`@namespace svg url(http://www.w3.org/2000/svg);`, "不支持 @namespace"},
		{`.a { background: image-set("a.png" 1x) }`, "image-set"},
	}
	for _, c := range cases {
		_, issues := ScopeCSS(c.src, assetLookup)
		var msgs []string
		for _, i := range issues {
			msgs = append(msgs, i.Msg)
		}
		if !strings.Contains(strings.Join(msgs, "；"), c.want) {
			t.Errorf("%s：期望问题含 %q，得到 %v", c.src, c.want, msgs)
		}
	}
}

func TestCompileTheme(t *testing.T) {
	set := NewFileSet()
	_ = set.Add("work.yaml", []byte("format: 1\nid: test.theme\nversion: 0.1.0\ntitle: 主题\n"))
	_ = set.Add("ui/theme.yaml", []byte(`fonts: {body: serif, heading: nofont}
colors:
  light: {background: "#f6f2ea", text: "#2a2723", muted: "#7a7268", accent: "#8a5a2b", panel: "#efe8dc"}
  dark: {background: "121417"}
reading: {max_width: 38em, line_height: 2.0, indent: 2em, paragraph_spacing: big}
background: {image: assets/bg/fog.png, opacity: 0.12}
custom_css: ui/custom.css
`))
	_ = set.Add("ui/custom.css", []byte(".prose-book p { letter-spacing: .02em }\n"))
	m, rep := Compile(set)
	var got []string
	for _, i := range rep.Issues {
		got = append(got, i.String())
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"fonts.heading 引用的字体不存在", "colors.dark.background 不合法", "reading.paragraph_spacing 不合法", "background.image 引用的图片不存在"} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少问题 %q，得到：\n%s", want, joined)
		}
	}
	th := m.Theme
	if th.Reading.MaxWidth != "38em" || th.Reading.LineHeight != 2.0 || th.Reading.Font != "serif" || th.Reading.ParagraphSpacing != "" {
		t.Fatalf("排版数值：%+v", th.Reading)
	}
	if !strings.Contains(th.CSS, ".work-root { --paper: #f6f2ea") || strings.Contains(th.CSS, ".dark .work-root") {
		t.Fatalf("浅色配色应生成变量，不合法的深色配色不生成：\n%s", th.CSS)
	}
	if !strings.Contains(th.CSS, ".work-root {\n.prose-book p") {
		t.Fatalf("custom.css 应包在 .work-root 中：\n%s", th.CSS)
	}
}
