package contextx_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/tmpl"
	"github.com/wahhheee/aint/internal/work"
)

func TestDevelopmentPrefixTracksContent(t *testing.T) {
	b := contextx.NewBuilder(tmpl.New(), nil)
	for _, text := range []string{"旧的执笔指令", "新的执笔指令"} {
		set, err := work.FromFS(fstest.MapFS{"work.yaml": {Data: []byte("format: 1\nid: test.reload\nversion: 1.0.0\ntitle: 缓存测试\n")}, "prompts/narrator.md": {Data: []byte(text)}}, work.DefaultLimits)
		if err != nil {
			t.Fatal(err)
		}
		w, report := work.Compile(set)
		if report.HasErrors() {
			t.Fatal(report)
		}
		w.VersionID = "same-dev-version"
		w.IsDev = true
		model := fake.Model()
		got, err := b.Build(context.Background(), contextx.BuildInput{Work: w, Features: contextx.Profile(w, save.Settings{}, model.Caps, false), Model: model})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Request.System[1].Text, text) {
			t.Fatal("热重载后命中旧前缀", got.Request.System[1].Text)
		}
	}
}
