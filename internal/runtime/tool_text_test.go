package runtime

import (
	"slices"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/work"
)

func TestToolTextFilter(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		found             []string
		tags              []work.TagDef
	}{
		{name: "截图中的属性与子元素", input: `雾港。<offer_choices prompt="第一夜"><choice id="a" label="邮局"/><choice id="b" label="灯塔"/></offer_choices>灯亮了。`, want: "雾港。灯亮了。", found: []string{"offer_choices"}},
		{name: "JSON参数", input: `前<update_state>{"op":"inc","path":"day","value":1}</update_state>后<offer_choices>{"choices":[]}</offer_choices>`, want: "前后", found: []string{"update_state", "offer_choices"}},
		{name: "兼容接口的多余引号", input: `<offer_choices"><choices><item/></choices></offer_choices>后`, want: "后", found: []string{"offer_choices"}},
		{name: "自闭合", input: `前<read path="/work/lore/main.md"/>后`, want: "前后", found: []string{"read"}},
		{name: "属性中的结束符", input: `前<read path="/memory/a/>b.txt"/>后`, want: "前后", found: []string{"read"}},
		{name: "重复标签", input: `<read/>中<read/>`, want: "中", found: []string{"read"}},
		{name: "未闭合标签", input: `前<offer_choices choices="中`, want: "前", found: []string{"offer_choices"}},
		{name: "截断在完整名称", input: `前<offer_choices`, want: "前", found: []string{"offer_choices"}},
		{name: "名称尚不完整", input: `前<offer_choice`, want: `前<offer_choice`},
		{name: "名称边界", input: `<read_more>内容</read_more><readonly/>`, want: `<read_more>内容</read_more><readonly/>`},
		{name: "正常叙事标签", input: `<say who="lin">读信。</say><gold>午夜。</gold>`, want: `<say who="lin">读信。</say><gold>午夜。</gold>`},
		{name: "中文与普通比较符", input: `中文<文字，1 < 2，>，<`, want: `中文<文字，1 < 2，>，<`},
		{name: "仅匹配可用工具", input: `<delegate>{}</delegate>`, want: `<delegate>{}</delegate>`},
		{name: "同名作品样式", input: `<read>文字</read>`, want: `<read>文字</read>`, tags: []work.TagDef{{Name: "read"}}},
		{name: "代码引用", input: "`<read/>`与``<offer_choices/>``", want: "`<read/>`与``<offer_choices/>``"},
		{name: "围栏代码", input: "```xml\n<read/>\n```\n<read/>后", want: "```xml\n<read/>\n```\n后", found: []string{"read"}},
		{name: "转义引用", input: `\<read/> &lt;offer_choices&gt;`, want: `\<read/> &lt;offer_choices&gt;`},
		{name: "闭合边界", input: `<read>内容</reader>内容</read >后`, want: "后", found: []string{"read"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 穷举每个字节分割点，包括 UTF-8 字符和标签名称内部。
			for split := 0; split <= len(tc.input); split++ {
				f := newToolTextFilter([]llm.ToolSpec{{Name: "offer_choices"}, {Name: "update_state"}, {Name: "read"}}, tc.tags)
				got := f.push(tc.input[:split]) + f.push(tc.input[split:]) + f.flush()
				if got != tc.want || !slices.Equal(f.found, tc.found) {
					t.Fatalf("分割点 %d：文本 %q，工具 %v；期望 %q、%v", split, got, f.found, tc.want, tc.found)
				}
			}
		})
	}
}

func TestToolTextFilterStreamsNarrative(t *testing.T) {
	f := newToolTextFilter([]llm.ToolSpec{{Name: "offer_choices"}}, nil)
	if got := f.push("雾港的灯亮了。<offer_"); got != "雾港的灯亮了。" {
		t.Fatalf("普通正文应立即输出，标签前缀应等待完整名称：%q", got)
	}
	if got := f.push("choices>" + strings.Repeat("参数", 10000)); got != "" || len(f.pending) > 30 {
		t.Fatal("伪调用内容不应显示或无限缓冲")
	}
	if got := f.push("</offer_choices>脚步声渐近。"); got != "脚步声渐近。" {
		t.Fatalf("标签之后的故事应立即输出：%q", got)
	}
}
