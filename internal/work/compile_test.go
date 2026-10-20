package work

import (
	"io/fs"
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
