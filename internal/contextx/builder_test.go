package contextx_test

import (
	"context"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/wahhheee/yanyi/examples"
	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/tmpl"
	"github.com/wahhheee/yanyi/internal/tools"
	"github.com/wahhheee/yanyi/internal/work"
)

func build(t *testing.T, name string, caps llm.Caps) (*contextx.Built, prompts.FeatureSet) {
	t.Helper()
	sub, _ := fs.Sub(examples.Works(), name)
	set, err := work.FromFS(sub, work.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	m, rep := work.Compile(set)
	if rep.HasErrors() {
		t.Fatalf("%s 校验失败：%+v", name, rep.Issues)
	}
	settings := save.Settings{Pace: "medium", Reasoning: "low"}
	features := contextx.Profile(m, settings, caps, false)
	model := fake.Model()
	model.Caps = caps
	st := m.State.Defaults()
	b := contextx.NewBuilder(tmpl.New(), nil)
	env := contextx.Envelope{Input: save.TurnInput{Kind: save.InputFree}, Text: "推开门", Pace: "medium"}
	built, err := b.Build(context.Background(), contextx.BuildInput{
		SaveID: "s1", Work: m, Features: features, Model: model, Settings: settings,
		InitialState: st, InitialStateHash: "h", Tools: tools.NewRegistry().Specs(features, m),
		Turn: env.Message(features),
	})
	if err != nil {
		t.Fatal(err)
	}
	return built, features
}

func toolNames(r *llm.Request) []string {
	var out []string
	for _, s := range r.Tools {
		out = append(out, s.Name)
	}
	return out
}

func systemText(r *llm.Request) string {
	var parts []string
	for _, b := range r.System {
		parts = append(parts, b.Text)
	}
	return strings.Join(parts, "\n\n")
}

// 最小作品：只剩基础工具，没有状态、命令、标签、设定相关的任何痕迹（《02-技术方案》8.6）。
func TestMinimalWorkAssembly(t *testing.T) {
	built, _ := build(t, "first-story", llm.Caps{})
	want := []string{"read", "list", "search", "write", "edit", "remove", "offer_choices"}
	if got := toolNames(built.Request); !slices.Equal(got, want) {
		t.Fatalf("最小作品的工具应为 %v，得到 %v", want, got)
	}
	sys := systemText(built.Request)
	// 特效默认关闭、场外默认开启（D-23）：没有"叙事标记"一节，但有 <ooc> 的说明。
	for _, absent := range []string{"## 状态", "update_state", "/work", "命令", "/uploads", "设定索引", "叙事标记"} {
		if strings.Contains(sys, absent) {
			t.Errorf("最小作品的提示词不应包含 %q", absent)
		}
	}
	if !strings.Contains(sys, "<ooc>") || !strings.Contains(sys, "/memory/index.md 是总索引") {
		t.Error("应包含场外发言说明与平台默认的记忆目录约定（D-24）")
	}
	if !strings.Contains(sys, "# 作品：第一个故事") || !strings.Contains(sys, "海边小镇") {
		t.Error("应包含作品标题与执笔指令")
	}
	if len(built.Request.Messages) != 1 {
		t.Errorf("最小作品没有 L3/L4，开局时只应有本回合一条消息，得到 %d 条", len(built.Request.Messages))
	}
}

// emptyHeading 找出"后面紧跟同级或更高级标题、或已到文末"的标题。
func emptyHeading(text string) string {
	lines := strings.Split(text, "\n")
	level := func(l string) int {
		n := len(l) - len(strings.TrimLeft(l, "#"))
		if n > 0 && strings.HasPrefix(l[n:], " ") {
			return n
		}
		return 0
	}
	for i, l := range lines {
		lv := level(l)
		if lv == 0 {
			continue
		}
		next := ""
		for _, nl := range lines[i+1:] {
			if strings.TrimSpace(nl) != "" {
				next = nl
				break
			}
		}
		if next == "" || (level(next) > 0 && level(next) <= lv) {
			return l
		}
	}
	return ""
}

// 一致性：文本中提到的工具都已注册；没有空标题。
func TestAssemblyConsistency(t *testing.T) {
	allTools := []string{"read", "list", "search", "write", "edit", "remove", "get_state", "update_state",
		"offer_choices", "roll_dice", "delegate"}
	for _, name := range []string{"first-story", "misty-harbor"} {
		for _, caps := range []llm.Caps{{}, {Vision: true}} {
			built, _ := build(t, name, caps)
			sys := systemText(built.Request)
			registered := toolNames(built.Request)
			for _, tool := range allTools {
				if regexp.MustCompile(`\b`+tool+`\b`).MatchString(sys) && !slices.Contains(registered, tool) {
					t.Errorf("%s：提示词提到了未注册的工具 %s", name, tool)
				}
			}
			if h := emptyHeading(sys); h != "" {
				t.Errorf("%s：出现空标题：%q", name, h)
			}
		}
	}
}

// 前缀稳定：同样的输入两次构建，前缀哈希一致。
func TestPrefixHashStable(t *testing.T) {
	a, _ := build(t, "misty-harbor", llm.Caps{})
	b, _ := build(t, "misty-harbor", llm.Caps{})
	if a.PrefixHash != b.PrefixHash {
		t.Fatal("相同输入的前缀哈希应一致")
	}
	c, _ := build(t, "misty-harbor", llm.Caps{Vision: true})
	if a.PrefixHash == c.PrefixHash {
		t.Fatal("换成支持看图的模型后前缀应变化（提示词裁切不同）")
	}
}
