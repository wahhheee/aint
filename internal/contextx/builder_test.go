package contextx_test

import (
	"context"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/wahhheee/aint/examples"
	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/tmpl"
	"github.com/wahhheee/aint/internal/tools"
	"github.com/wahhheee/aint/internal/work"
)

func build(t *testing.T, name string, caps llm.Caps) (*contextx.Built, prompts.FeatureSet) {
	t.Helper()
	source := examples.Works()
	if name == "misty-harbor" {
		source = os.DirFS("../../testdata/works")
	}
	sub, _ := fs.Sub(source, name)
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

func TestForceChoicesAssembly(t *testing.T) {
	sub, err := fs.Sub(examples.Works(), "first-story")
	if err != nil {
		t.Fatal(err)
	}
	set, err := work.FromFS(sub, work.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	w, rep := work.Compile(set)
	if rep.HasErrors() {
		t.Fatalf("作品校验失败：%+v", rep.Issues)
	}
	b := contextx.NewBuilder(tmpl.New(), nil)
	for _, free := range []bool{true, false} {
		var previousHash string
		for _, force := range []bool{false, true} {
			w.Manifest.Features.FreeInput, w.Manifest.Features.ForceChoices = free, force
			features := contextx.Profile(w, save.Settings{}, llm.Caps{}, false)
			if features.Has(prompts.FForceChoices) != force {
				t.Fatalf("force_choices 能力不符：%v", features)
			}
			built, err := b.Build(t.Context(), contextx.BuildInput{
				SaveID: "s1", Work: w, Features: features, Model: fake.Model(),
				InitialState: map[string]any{}, InitialStateHash: "h", Tools: tools.NewRegistry().Specs(features, w),
				Turn: llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("推开门")}},
			})
			if err != nil {
				t.Fatal(err)
			}
			sys := systemText(built.Request)
			if got := strings.Contains(sys, "暂时没有合适的选项时，写完直接结束即可"); got != (free && !force) {
				t.Errorf("free_input=%v force_choices=%v：可省略选项的说明不符", free, force)
			}
			if got := strings.Contains(sys, "每个回合的正文写完后，请调用 offer_choices"); got != force {
				t.Errorf("free_input=%v force_choices=%v：正文结束后的选项要求不符", free, force)
			}
			if got := strings.Contains(sys, "玩家也可以自由输入"); got != free {
				t.Errorf("free_input=%v force_choices=%v：自由输入的说明不符", free, force)
			}
			if previousHash == built.PrefixHash {
				t.Error("force_choices 影响提示词，前缀哈希应不同")
			}
			previousHash = built.PrefixHash
		}
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
	for _, name := range []string{"first-story", "inverted-stars", "misty-harbor"} {
		for _, caps := range []llm.Caps{{}, {Vision: true}} {
			built, _ := build(t, name, caps)
			sys := systemText(built.Request)
			registered := toolNames(built.Request)
			for _, spec := range built.Request.Tools {
				if !strings.Contains(sys, "- `"+spec.Name+"`："+spec.Description) {
					t.Errorf("%s：系统提示词缺少 %s 的自然语言用法", name, spec.Name)
				}
			}
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

func TestToolGuideCacheTracksDefinitions(t *testing.T) {
	sub, _ := fs.Sub(examples.Works(), "first-story")
	set, err := work.FromFS(sub, work.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	w, rep := work.Compile(set)
	if rep.HasErrors() {
		t.Fatalf("作品校验失败：%+v", rep.Issues)
	}
	fs := contextx.Profile(w, save.Settings{}, llm.Caps{}, false)
	b := contextx.NewBuilder(tmpl.New(), nil)
	in := contextx.BuildInput{Work: w, Features: fs, Model: fake.Model(), InitialState: w.State.Defaults(),
		InitialStateHash: "h", Tools: tools.NewRegistry().Specs(fs, w)}
	first, err := b.Build(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	in.Tools = slices.Clone(in.Tools)
	in.Tools[0].Description = "读取指定路径的测试内容。"
	second, err := b.Build(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(systemText(second.Request), "- `read`："+in.Tools[0].Description) ||
		strings.Contains(systemText(first.Request), in.Tools[0].Description) || first.PrefixHash == second.PrefixHash {
		t.Fatal("工具描述变化应重建提示词与前缀哈希，不应污染之前的请求")
	}
	in.Tools = nil
	empty, err := b.Build(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(systemText(empty.Request), "## 工具用法") {
		t.Fatal("没有工具定义时不应残留缓存中的工具用法")
	}
	for _, report := range empty.Fragments {
		if report.ID == "tools.guide" && (report.Enabled || report.Reason == "") {
			t.Fatalf("工具用法片段应标记为未启用：%+v", report)
		}
	}
}

// token 估算（I-TOKENS）：各层都有数，常驻图片按尺寸估算；超过建议值时给出警告。
func TestAnalyzeTokens(t *testing.T) {
	sub := os.DirFS("../../testdata/works/misty-harbor")
	set, err := work.FromFS(sub, work.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	m, rep := work.Compile(set)
	contextx.Analyze(m, rep, tools.NewRegistry().Specs)
	tk := rep.Tokens
	if tk == nil || tk.Platform == 0 || tk.Work == 0 || tk.Index == 0 || tk.Tools == 0 || tk.Resident == 0 || tk.Images == 0 {
		t.Fatalf("各层都应有估算：%+v", tk)
	}
	if tk.Total != tk.Platform+tk.Work+tk.Tools+tk.Resident+tk.Images {
		t.Fatalf("合计不对：%+v", tk)
	}
	found := false
	for _, i := range rep.Issues {
		found = found || (i.Code == work.CodeTokens && i.Level == work.LevelInfo && strings.Contains(i.Message, "常驻部分约"))
	}
	if !found {
		t.Fatalf("应有 I-TOKENS 信息：%+v", rep.Issues)
	}
	if got := contextx.EstimateTokens("雾港abc"); got != 3 { // 2 × 1.0 + 3 × 0.3 = 2.9 → 3
		t.Fatalf("EstimateTokens = %d", got)
	}
	if got := contextx.EstimateImageTokens(3136, 1568); got != contextx.EstimateImageTokens(1568, 784) {
		t.Fatalf("大图应先缩放到长边 1568 再估算：%d", got)
	}
}
