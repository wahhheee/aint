package panel

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/wahhheee/aint/examples"
	"github.com/wahhheee/aint/internal/tmpl"
	"github.com/wahhheee/aint/internal/work"
)

func TestRenderExamplePanels(t *testing.T) {
	sub, _ := fs.Sub(examples.Works(), "misty-harbor")
	set, err := work.FromFS(sub, work.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := work.Compile(set)
	st := m.State.Defaults()
	got := Render(tmpl.New(), m, Vars{State: st, Turn: 1})

	ids := map[string]string{}
	for _, p := range got {
		ids[p.ID] = p.HTML
	}
	if _, ok := ids["sealed"]; ok {
		t.Error("没有封印陈述时 sealed 面板应自动隐藏")
	}
	nb := ids["notebook"]
	if !strings.Contains(nb, `<ui-stat label="第几天" value="1"`) || !strings.Contains(nb, "<h3>") {
		t.Errorf("笔记本面板应保留 ui-* 组件并把 Markdown 转为 HTML：\n%s", nb)
	}
	if !strings.Contains(nb, `label="林舟" value="12"`) {
		t.Errorf("信任条应按 value + 10 渲染：\n%s", nb)
	}
}
