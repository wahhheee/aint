package contextx

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/wahhheee/yanyi/internal/prompts"
)

// FragmentReport 是一个片段的组装结果（检查器"上下文"页，USG-03）。
type FragmentReport struct {
	ID      string `json:"id"`
	Layer   string `json:"layer"` // L0 | L1 | L3 | L4
	Section string `json:"section,omitzero"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitzero"` // 未启用的原因
	Chars   int    `json:"chars"`
}

// section 是组装中的一节：标题 + 已启用片段的文本。
type section struct {
	title string
	lines []string
}

// renderFragments 按启用条件渲染片段，节内没有启用的片段时整节（含标题）省略。
// headingLevel 是节标题的级别（L0 用 "##"）。
func renderFragments(layer string, frags []prompts.Fragment, fs prompts.FeatureSet, headingLevel string) (string, []FragmentReport, error) {
	funcs := template.FuncMap{"has": func(f string) bool { return fs.Has(prompts.Feature(f)) }}
	var order []string
	sections := map[string]*section{}
	var reports []FragmentReport

	for _, f := range frags {
		rep := FragmentReport{ID: f.ID, Layer: layer, Section: f.Section}
		ok, reason := f.When.Eval(fs)
		if !ok {
			rep.Reason = reason
			reports = append(reports, rep)
			continue
		}
		tpl, err := template.New(f.ID).Funcs(funcs).Parse(f.Text)
		if err != nil {
			return "", nil, fmt.Errorf("提示词片段 %s 模板错误: %w", f.ID, err)
		}
		var b strings.Builder
		if err := tpl.Execute(&b, nil); err != nil {
			return "", nil, fmt.Errorf("提示词片段 %s 渲染失败: %w", f.ID, err)
		}
		text := strings.TrimSpace(b.String())
		if text == "" {
			rep.Reason = "渲染结果为空"
			reports = append(reports, rep)
			continue
		}
		rep.Enabled, rep.Chars = true, len([]rune(text))
		reports = append(reports, rep)

		sec, exists := sections[f.Section]
		if !exists {
			sec = &section{title: f.Section}
			sections[f.Section] = sec
			order = append(order, f.Section)
		}
		sec.lines = append(sec.lines, text)
	}
	return joinSections(order, sections, headingLevel), reports, nil
}

func joinSections(order []string, sections map[string]*section, headingLevel string) string {
	var parts []string
	for _, name := range order {
		sec := sections[name]
		body := strings.Join(sec.lines, "\n")
		if sec.title != "" {
			body = headingLevel + " " + sec.title + "\n" + body
		}
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n")
}
