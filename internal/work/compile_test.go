package work

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/wahhheee/yanyi/examples"
)

func compileExample(t *testing.T, name string) (*Model, *Report) {
	t.Helper()
	sub, err := fs.Sub(examples.Works(), name)
	if err != nil {
		t.Fatal(err)
	}
	set, err := FromFS(sub, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	return Compile(set)
}

func TestCompileExamples(t *testing.T) {
	for _, name := range []string{"first-story", "misty-harbor"} {
		t.Run(name, func(t *testing.T) {
			_, rep := compileExample(t, name)
			for _, i := range rep.Issues {
				t.Log(i.String())
			}
			if rep.HasErrors() {
				t.Fatal("示例作品不应有校验错误")
			}
		})
	}
}

func TestCompileMistyHarbor(t *testing.T) {
	m, _ := compileExample(t, "misty-harbor")
	if m.Opening == nil || m.Opening.Mode != "static" || len(m.Opening.Choices) != 3 {
		t.Fatalf("开场解析不对：%+v", m.Opening)
	}
	if m.State.Empty() || m.State.Fields["trust"].Values.Type != "number" {
		t.Fatal("状态定义解析不对")
	}
	if len(m.Commands) != 2 || len(m.Tags) != 3 || len(m.Layout.Panels) != 4 {
		t.Fatalf("命令/标签/面板数量不对：%d/%d/%d", len(m.Commands), len(m.Tags), len(m.Layout.Panels))
	}
}

func TestTreeHashStable(t *testing.T) {
	a, _ := compileExample(t, "misty-harbor")
	b, _ := compileExample(t, "misty-harbor")
	if a.TreeHash != b.TreeHash {
		t.Fatal("同内容的文件树哈希应一致")
	}
}

func TestCompileReportsManifestErrors(t *testing.T) {
	set := NewFileSet()
	_ = set.Add("work.yaml", []byte("format: 2\nid: X\nversion: 1\ntitl: 拼错了\n"))
	_, rep := Compile(set)
	codes := map[string]int{}
	for _, i := range rep.Issues {
		codes[i.Code]++
	}
	if codes[CodeManifest] < 3 || codes[CodeUnknownField] != 1 {
		t.Fatalf("应报告 format/id/version/title 错误与未知字段，得到 %+v", rep.Issues)
	}
}

func TestDocLinks(t *testing.T) {
	m, _ := compileExample(t, "misty-harbor")
	got := m.LinkDocs("- [[old-chen]]：师徒；[[lin]](已有链接)；[[nobody]]")
	want := "- [[old-chen]](/work/lore/characters/old-chen.md)：师徒；[[lin]](已有链接)；[[nobody]]"
	if got != want {
		t.Fatalf("链接补全不对：\n%s", got)
	}

	set := NewFileSet()
	_ = set.Add("work.yaml", []byte("format: 1\nid: test.links\nversion: 0.1.0\ntitle: 链接\n"))
	_ = set.Add("lore/a.md", []byte("---\ntitle: 甲\n---\n第一行\n见 [[b-missing]]\n"))
	_, rep := Compile(set)
	if len(rep.Issues) == 0 || rep.Issues[0].Code != CodeRef || rep.Issues[0].Line != 5 {
		t.Fatalf("应报告不存在的文档链接并定位到第 5 行：%+v", rep.Issues)
	}
}

func TestTemplateChecks(t *testing.T) {
	set := NewFileSet()
	_ = set.Add("work.yaml", []byte("format: 1\nid: test.tpl\nversion: 0.1.0\ntitle: 模板\n"))
	_ = set.Add("prompts/narrator.md", []byte("你好 {{ state.name }}\n{% if x %}\n没有结束\n"))
	_ = set.Add("commands/look.md", []byte("---\ndescription: 看\n---\n看看\n{% include 'x' %}\n"))
	_ = set.Add("ui/layout.yaml", []byte("panels:\n  - {id: a, place: sidebar, template: ui/panels/a.liquid}\n"))
	_ = set.Add("ui/panels/a.liquid", []byte("{% render 'panels/_row', v: 1 %}\n{% render 'panels/_nope' %}\n"))
	_ = set.Add("ui/panels/_row.liquid", []byte("{{ v }}"))
	m, rep := Compile(set)
	var got []string
	for _, i := range rep.Issues {
		got = append(got, i.String())
	}
	want := []string{
		"[E-TEMPLATE] prompts/narrator.md:2",
		"[E-TEMPLATE] commands/look.md:5 不支持 include",
		"[E-REF] ui/panels/a.liquid:2 render 引用的模板不存在：panels/_nope",
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			found = found || strings.HasPrefix(g, w)
		}
		if !found {
			t.Errorf("缺少问题 %q，得到：\n%s", w, strings.Join(got, "\n"))
		}
	}
	if len(m.Partials) != 2 {
		t.Fatalf("ui/ 下的模板集：%v", m.Partials)
	}
}
