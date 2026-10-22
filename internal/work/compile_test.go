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
	if len(m.Commands) != 3 || len(m.Tags) != 3 || len(m.Layout.Panels) != 5 {
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

func TestEncodingWarnings(t *testing.T) {
	set := NewFileSet()
	_ = set.Add("work.yaml", []byte("format: 1\nid: test.enc\nversion: 0.1.0\ntitle: 编码\n"))
	_ = set.Add("lore/a.md", append([]byte{0xEF, 0xBB, 0xBF}, []byte("---\ntitle: 甲\n---\n正文\n")...))
	_ = set.Add("lore/b.md", []byte{0xD3, 0xEA, 0xB8, 0xDB}) // GBK 编码的"雨港"
	m, rep := Compile(set)
	var got []string
	for _, i := range rep.Issues {
		if i.Code == CodeEncoding {
			got = append(got, i.Path)
		}
	}
	if strings.Join(got, ",") != "lore/a.md,lore/b.md" {
		t.Fatalf("应提示 BOM 与非 UTF-8：%+v", rep.Issues)
	}
	if d, ok := m.DocByPath("lore/a.md"); !ok || d.Title != "甲" {
		t.Fatal("带 BOM 的文件应照常读取")
	}
}

func TestAssetMagic(t *testing.T) {
	set := NewFileSet()
	_ = set.Add("work.yaml", []byte("format: 1\nid: test.assets\nversion: 0.1.0\ntitle: 素材\n"))
	_ = set.Add("assets/rain.ogg", []byte("OggS\x00\x02"))
	_ = set.Add("assets/fake.mp3", []byte("<html>not audio</html>"))
	_ = set.Add("assets/font.woff2", []byte("wOF2\x00\x01"))
	_ = set.Add("assets/broken.png", []byte("\x89PNG\r\n\x1a\nbroken"))
	m, rep := Compile(set)
	var bad []string
	for _, i := range rep.Issues {
		if i.Code == CodeAsset {
			bad = append(bad, i.Path)
		}
	}
	if strings.Join(bad, ",") != "assets/broken.png,assets/fake.mp3" {
		t.Fatalf("应报告无法解码的图片与内容不符的音频：%v", rep.Issues)
	}
	if len(m.Assets) != 2 {
		t.Fatalf("有问题的素材不进入素材索引：%+v", m.Assets)
	}
}
