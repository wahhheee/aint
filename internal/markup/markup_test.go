package markup

import "testing"

func TestPlainText(t *testing.T) {
	cases := []struct{ in, want string }{
		{`<scene bg="x" title="序章"/>雾很浓。`, "雾很浓。"},
		{`<say who="林舟">「你来了。」</say>`, "林舟：「你来了。」"},
		{"<ooc>好的，接下来节奏放慢。</ooc>\n雨停了。", "雨停了。"},
		{"雨停了。\n<ooc>场外\n跨行回应", "雨停了。"}, // 未闭合的场外块延续到结尾
		{"这是**很重要**的线索", "这是很重要的线索"},
	}
	for _, c := range cases {
		if got := PlainText(c.in); got != c.want {
			t.Errorf("PlainText(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}
