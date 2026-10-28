package work

import (
	"bytes"
	"image"
	"image/png"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/wahhheee/aint/examples"
)

func compileExample(t *testing.T, name string) (*Model, *Report) {
	t.Helper()
	source := examples.Works()
	if name == "misty-harbor" {
		source = os.DirFS("../../testdata/works")
	}
	sub, err := fs.Sub(source, name)
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
	for _, name := range []string{"first-story", "inverted-stars", "misty-harbor"} {
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

func TestForceChoicesManifest(t *testing.T) {
	for _, tc := range []struct {
		name, feature string
		want, invalid bool
	}{
		{name: "默认关闭"},
		{name: "显式关闭", feature: "features:\n  force_choices: false\n"},
		{name: "作品开启", feature: "features:\n  force_choices: true\n", want: true},
		{name: "拒绝非布尔值", feature: "features:\n  force_choices: invalid\n", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := NewFileSet()
			if err := set.Add("work.yaml", []byte("format: 1\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nid: test.choices\nversion: 0.1.0\ntitle: 选项\n"+tc.feature)); err != nil {
				t.Fatal(err)
			}
			m, rep := Compile(set)
			if rep.HasErrors() != tc.invalid {
				t.Fatalf("校验结果不符：%+v", rep.Issues)
			}
			if !tc.invalid && m.Manifest.Features.ForceChoices != tc.want {
				t.Fatalf("force_choices = %v，应为 %v", m.Manifest.Features.ForceChoices, tc.want)
			}
		})
	}
}

func TestCompileMistyHarbor(t *testing.T) {
	m, _ := compileExample(t, "misty-harbor")
	if len(m.Openings) != 2 || m.Openings[0].Mode != "static" || len(m.Openings[0].Choices) != 3 {
		t.Fatalf("开场解析不对：%+v", m.Openings)
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
	_ = set.Add("work.yaml", []byte("format: 1\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nid: test.links\nversion: 0.1.0\ntitle: 链接\n"))
	_ = set.Add("lore/a.md", []byte("---\ntitle: 甲\n---\n第一行\n见 [[b-missing]]\n"))
	_, rep := Compile(set)
	if len(rep.Issues) == 0 || rep.Issues[0].Code != CodeRef || rep.Issues[0].Line != 5 {
		t.Fatalf("应报告不存在的文档链接并定位到第 5 行：%+v", rep.Issues)
	}
}

func TestTemplateChecks(t *testing.T) {
	set := NewFileSet()
	_ = set.Add("work.yaml", []byte("format: 1\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nid: test.tpl\nversion: 0.1.0\ntitle: 模板\n"))
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
	_ = set.Add("work.yaml", []byte("format: 1\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nid: test.enc\nversion: 0.1.0\ntitle: 编码\n"))
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

func TestUIAssetConflicts(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, assets, lore, want string
	}{
		{name: "界面专用", lore: "display: {avatar: fog}"},
		{name: "背景角色", assets: "role: background", want: "这张图片用作 work.yaml 的 cover 与 ui/theme.yaml 的 background.image，只给界面用，不会进入 AI 的素材索引；它同时在 assets.yaml 中标为 role: background"},
		{name: "插图与文档图片", assets: "role: cg", lore: "images: [fog]", want: "它同时在 assets.yaml 中标为 role: cg、被 lore/lin.md 的 images 引用"},
		{name: "文档图片", lore: "images: [assets/fog.png]", want: "它同时被 lore/lin.md 的 images 引用"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := NewFileSet()
			_ = set.Add("work.yaml", []byte("format: 1\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nid: test.ui\nversion: 0.1.0\ntitle: 界面素材\ncover: fog\n"))
			_ = set.Add("ui/theme.yaml", []byte("background: {image: assets/fog.png}\n"))
			_ = set.Add("assets.yaml", []byte("- {path: assets/fog.png, "+tc.assets+"}\n"))
			_ = set.Add("lore/lin.md", []byte("---\ntitle: 林舟\n"+tc.lore+"\n---\n正文\n"))
			_ = set.Add("assets/fog.png", buf.Bytes())
			m, rep := Compile(set)
			var got []Issue
			for _, i := range rep.Issues {
				if i.Code == CodeUIAsset {
					got = append(got, i)
				}
			}
			switch {
			case tc.want == "" && len(got) > 0:
				t.Fatalf("只给界面用的素材不需要提示：%+v", got)
			case tc.want != "" && (len(got) != 1 || got[0].Path != "assets/fog.png" || !strings.Contains(got[0].Message, tc.want)):
				t.Fatalf("应提示 %q，得到 %+v", tc.want, rep.Issues)
			}
			if a, _ := m.Asset("fog"); !m.UIOnly(a) {
				t.Fatal("封面与主题背景只给界面用")
			}
		})
	}
}

func TestStateSchemaErrors(t *testing.T) {
	for _, tc := range []struct {
		name, state, code, msg string
		line                   int
	}{
		{name: "引号错误", state: "fields:\n  name:\n    type: string\n    desc: \"我\"的名字\n", code: CodeParse, line: 4},
		{name: "未知类型", state: "fields:\n  name: {type: text, desc: 名字}\n", code: CodeState},
		{name: "默认值类型", state: "fields:\n  day: {type: number, default: 第一天, desc: 天数}\n", code: CodeState},
		{name: "未知顶层键", state: "feilds:\n  day: {type: number, desc: 天数}\n", code: CodeUnknownField, msg: "未知字段：feilds"},
		{name: "字段定义的未知键", state: "fields:\n  day: {type: number, defaults: 3, desc: 天数}\n", code: CodeUnknownField, msg: "状态字段 day 的未知字段：defaults"},
		{name: "嵌套字段的未知键", state: "fields:\n  player:\n    type: object\n    fields:\n      name: {type: string, desc: 名字, label: 姓名}\n", code: CodeUnknownField, msg: "状态字段 player.name 的未知字段：label"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := NewFileSet()
			_ = set.Add("work.yaml", []byte("format: 1\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nid: test.state\nversion: 0.1.0\ntitle: 状态\n"))
			_ = set.Add("state.yaml", []byte(tc.state))
			_, rep := Compile(set)
			if len(rep.Issues) != 1 || rep.Issues[0].Code != tc.code || rep.Issues[0].Path != "state.yaml" || rep.Issues[0].Line != tc.line {
				t.Fatalf("应报告 %s（第 %d 行），得到 %+v", tc.code, tc.line, rep.Issues)
			}
			if !strings.Contains(rep.Issues[0].Message, tc.msg) {
				t.Fatalf("消息应含 %q，得到 %q", tc.msg, rep.Issues[0].Message)
			}
			if tc.code == CodeParse && strings.Contains(rep.Issues[0].Hint, "六种") {
				t.Fatalf("语法错误不应提示类型：%+v", rep.Issues[0])
			}
		})
	}
}

func TestAssetMagic(t *testing.T) {
	set := NewFileSet()
	_ = set.Add("work.yaml", []byte("format: 1\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nid: test.assets\nversion: 0.1.0\ntitle: 素材\n"))
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
