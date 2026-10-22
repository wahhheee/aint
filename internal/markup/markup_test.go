package markup

import "testing"

// PlainText 是服务端专用的入口（所有名称合法的标签都按已启用处理）；Parse 与 Plain 的行为由共享向量覆盖（vectors_test.go）。
func TestPlainText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"场景标题单独一行", `<scene bg="x" title="序章"/>雾很浓。`, "序章\n雾很浓。"},
		{"台词带说话人", `<say who="林舟">「你来了。」</say>`, "林舟：「你来了。」"},
		{"场外块不属于故事", "<ooc>好的，接下来节奏放慢。</ooc>\n雨停了。", "雨停了。"},
		{"未闭合的场外块延续到结尾", "雨停了。\n<ooc>场外\n跨行回应", "雨停了。"},
		{"行内场外回应", "他点头。<ooc>（好）</ooc>继续。", "他点头。继续。"},
		{"强调只取文字", "这是**很重要**的*线索*", "这是很重要的线索"},
		{"任何标签都去掉，包括作品未声明的", `<gold>封印</gold><unknown a="1">什么</unknown><pause ms="300"/>`, "封印什么"},
		{"空行与场景分隔", "第一段\n\n\n---\n\n第二段\n\n", "第一段\n◇\n第二段"},
		{"转义得到字符本身", `\*星号\* 与 \<b>`, "*星号* 与 <b>"},
		{"不构成标签的 < 保留", "1 < 2，a<b", "1 < 2，a<b"},
		{"多个正文显示项（runtime 以空行连接）", "<gold>甲\n\n乙</gold>\n\n", "甲\n乙"},
		{"空输入", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PlainText(c.in); got != c.want {
				t.Errorf("PlainText(%q) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}

// 任意输入都不能让解析出错，且结果稳定（同一输入两次解析得到相同的纯文本）。
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"", "雾很浓。", "<scene bg=\"a\"/>雾", "**a*b*c**", "<ooc>甲\n乙</ooc>丙", "\\<ooc>", "<say who=\"甲>话</say>",
		"<a><b><c><d><e>深</e></d></c></b></a>", "* * *", "> 引文\n---\n", "\ufeff　<gold>金",
	} {
		f.Add(s, false)
		f.Add(s, true)
	}
	tags := map[string]bool{"scene": true, "cg": true, "say": true, "pause": true, "aside": true, "ooc": true, "a": true, "b": true}
	f.Fuzz(func(t *testing.T, src string, drafting bool) {
		opts := Options{Tags: tags, Drafting: drafting}
		a, b := Plain(Parse(src, opts)), Plain(Parse(src, opts))
		if a != b {
			t.Fatalf("同一输入两次解析结果不同：%q / %q", a, b)
		}
		_ = PlainText(src)
	})
}
