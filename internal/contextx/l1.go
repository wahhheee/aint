package contextx

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/work"
)

// L1 作品指令（《02-技术方案》20.2）：由作品内容生成，每节一个片段，没有内容的节整节省略。

type l1Section struct {
	id, title string
	when      prompts.Cond
	render    func(m *work.Model, fs prompts.FeatureSet) string
}

var l1Sections = []l1Section{
	{id: "l1.narrator", title: "执笔指令"}, // render 为空：执笔指令需要以初始状态渲染 Liquid，由 assembleL1 单独处理
	{id: "l1.markup", title: "叙事标记", when: prompts.Requires(prompts.FTags), render: renderMarkup},
	{id: "l1.commands", title: "命令", when: prompts.Requires(prompts.FCommands), render: renderCommands},
	{id: "l1.state", title: "状态说明", when: prompts.Requires(prompts.FState), render: renderStateDesc},
	{id: "l1.lore", title: "设定索引", when: prompts.Requires(prompts.FLoreIndex), render: renderLoreIndex},
	{id: "l1.assets", title: "素材索引", when: prompts.Requires(prompts.FAssets), render: renderAssets},
	{id: "l1.memory", title: "记忆约定", when: prompts.Requires(prompts.FMemoryGuide),
		render: func(m *work.Model, _ prompts.FeatureSet) string { return m.MemoryGuide }},
	// 平台默认的记忆目录约定在 L0"记忆"节（D-24）；这里只放作品自己的补充约定。
}

// assembleL1 组装作品指令。narrator 是已用初始状态渲染好的执笔指令。
func assembleL1(m *work.Model, fs prompts.FeatureSet, narrator string) (string, []FragmentReport) {
	parts := []string{"# 作品：" + m.Manifest.Title}
	var reports []FragmentReport
	for _, sec := range l1Sections {
		rep := FragmentReport{ID: sec.id, Layer: "L1", Section: sec.title}
		ok, reason := sec.when.Eval(fs)
		if !ok {
			rep.Reason = reason
			reports = append(reports, rep)
			continue
		}
		var body string
		if sec.id == "l1.narrator" {
			body = narrator
		} else {
			body = sec.render(m, fs)
		}
		// 作者文本中的标题整体降到节标题之下（## 节 → ### 起），避免与平台的节标题同级。
		body = shiftHeadings(strings.TrimSpace(body), 3)
		if body == "" {
			rep.Reason = "没有内容"
			reports = append(reports, rep)
			continue
		}
		rep.Enabled, rep.Chars = true, len([]rune(body))
		reports = append(reports, rep)
		parts = append(parts, "## "+sec.title+"\n"+body)
	}
	return strings.Join(parts, "\n\n"), reports
}

// shiftHeadings 把 Markdown 中的标题整体平移，使最高一级标题变为 top 级（跳过代码块）。
// 例：作者用 ## 作顶级标题，top=3 时 ## → ###、### → ####；最多 6 级。
func shiftHeadings(text string, top int) string {
	lines := strings.Split(text, "\n")
	level := func(l string) int {
		n := len(l) - len(strings.TrimLeft(l, "#"))
		if n > 0 && n <= 6 && strings.HasPrefix(l[n:], " ") {
			return n
		}
		return 0
	}
	minLevel, inCode := 7, false
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inCode = !inCode
		} else if lv := level(l); !inCode && lv > 0 {
			minLevel = min(minLevel, lv)
		}
	}
	if minLevel >= top {
		return text
	}
	shift, inCode := top-minLevel, false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inCode = !inCode
		} else if lv := level(l); !inCode && lv > 0 {
			lines[i] = strings.Repeat("#", min(lv+shift, 6)) + l[lv:]
		}
	}
	return strings.Join(lines, "\n")
}

func renderMarkup(m *work.Model, fs prompts.FeatureSet) string {
	var lines []string
	add := func(f prompts.Feature, line string) {
		if fs.Has(f) {
			lines = append(lines, line)
		}
	}
	add(prompts.FTagScene, `- <scene bg="背景素材ID" title="可选"/>：切换场景背景`)
	add(prompts.FTagCG, `- <cg src="图片素材ID" caption="可选"/>：插图`)
	add(prompts.FTagSay, `- <say who="人物ID">台词</say>：本作品用它标注台词的说话人`)
	add(prompts.FTagAudio, `- <bgm src="音乐ID"/>、<sfx src="音效ID"/>：音乐与音效`)
	add(prompts.FTagEffects, `- <pause ms="800"/>：停顿；<fx type="shake|glitch|fade|whisper|flicker|blur-reveal">文字</fx>：文字特效；<aside>…</aside>：旁白`)
	for _, t := range m.Tags {
		lines = append(lines, fmt.Sprintf("- <%s>…</%s>：%s", t.Name, t.Name, t.Description))
	}
	return strings.Join(lines, "\n")
}

// renderAssets 列出素材（20.2）：背景按 ID 列出（供 <scene bg>），其他图片按路径列出（可 read 查看），音频按 ID。
func renderAssets(m *work.Model, fs prompts.FeatureSet) string {
	var bg, images, audio []string
	for _, a := range m.Assets {
		desc := ""
		if a.Description != "" {
			desc = "（" + a.Description + "）"
		}
		switch {
		case a.Kind == "image" && a.Role == "background":
			bg = append(bg, a.ID+desc)
		case a.Kind == "image":
			line := fmt.Sprintf("/work/%s", a.Path)
			if a.Description != "" {
				line += " — " + a.Description
			}
			images = append(images, line)
		case a.Kind == "audio":
			audio = append(audio, a.ID+desc)
		}
	}
	var lines []string
	if len(bg) > 0 && fs.Has(prompts.FTagScene) {
		lines = append(lines, "- 背景（用于 <scene bg>）："+strings.Join(bg, "、"))
	}
	if len(images) > 0 {
		hint := ""
		if fs.Has(prompts.FVision) {
			hint = "（可 read 查看）"
		}
		lines = append(lines, "- 图片"+hint+"："+nl+"  - "+strings.Join(images, nl+"  - "))
	}
	if len(audio) > 0 && fs.Has(prompts.FTagAudio) {
		lines = append(lines, "- 音乐与音效："+strings.Join(audio, "、"))
	}
	return strings.Join(lines, nl)
}

const nl = "\n"

func renderCommands(m *work.Model, fs prompts.FeatureSet) string {
	lines := []string{"玩家可以使用以下命令，命令展开后的文字会出现在 <input> 中："}
	for _, c := range m.Commands {
		if !c.AIVisible {
			continue
		}
		if c.HasPresent("slash") || c.HasPresent("button") {
			sig := "/" + c.Name
			for _, a := range c.Args {
				if a.Required {
					sig += " <" + a.Name + ">"
				} else {
					sig += " [" + a.Name + "]"
				}
			}
			lines = append(lines, fmt.Sprintf("- %s：%s", sig, c.Description))
		}
		if c.HasPresent("mode") && fs.Has(prompts.FInputModes) {
			label := c.ModeLabel
			if label == "" {
				label = c.Title
			}
			lines = append(lines, fmt.Sprintf("- 输入模式「%s」：%s", label, c.Description))
		}
	}
	return strings.Join(lines, "\n")
}

func renderStateDesc(m *work.Model, _ prompts.FeatureSet) string {
	var lines []string
	var walk func(prefix, indent string, f *state.Field)
	walk = func(prefix, indent string, f *state.Field) {
		line := fmt.Sprintf("%s- %s（%s）", indent, prefix, typeDesc(f))
		if f.Desc != "" {
			line += "：" + f.Desc
		}
		lines = append(lines, line)
		if f.Type == state.Object {
			for _, n := range f.Order {
				walk(prefix+"."+n, indent+"  ", f.Fields[n])
			}
		}
	}
	for _, name := range m.State.Order {
		walk(name, "", m.State.Fields[name])
	}
	return strings.Join(lines, "\n")
}

func typeDesc(f *state.Field) string {
	label := map[state.Type]string{
		state.Boolean: "布尔", state.Number: "数字", state.String: "文本",
		state.List: "列表", state.Map: "映射", state.Object: "对象",
	}
	switch f.Type {
	case state.List:
		return "列表：" + label[f.Items.Type]
	case state.Map:
		return "映射 → " + label[f.Values.Type]
	}
	return label[f.Type]
}

// kindOrder 是设定索引分组的固定顺序与中文名；其他 kind 原样显示并排在后面，"其他"最后。
var kindOrder = []struct{ kind, label string }{
	{"world", "世界"}, {"outline", "主线"}, {"character", "人物"}, {"place", "地点"}, {"faction", "势力"}, {"item", "物品"},
}

func kindLabel(kind string) (label string, rank int) {
	for i, k := range kindOrder {
		if k.kind == kind {
			return k.label, i
		}
	}
	if kind == "" {
		return "其他", len(kindOrder) + 1
	}
	return kind, len(kindOrder)
}

func renderLoreIndex(m *work.Model, _ prompts.FeatureSet) string {
	// 目录摘要（_index.md）通常不写 kind：归入同目录文档的分组。
	dirKind := map[string]string{}
	for _, d := range m.Docs {
		if !d.IsIndex && d.Kind != "" {
			if _, ok := dirKind[path.Dir(d.Path)]; !ok {
				dirKind[path.Dir(d.Path)] = d.Kind
			}
		}
	}
	type group struct {
		label string
		rank  int
		lines []string
	}
	var groups []*group
	byLabel := map[string]*group{}
	for _, d := range m.Docs {
		if d.Load != "index" {
			continue // always 的正文已在 L3；none 不进入索引
		}
		kind := d.Kind
		var line string
		if d.IsIndex {
			dir := path.Dir(d.Path)
			if kind == "" {
				kind = dirKind[dir]
			}
			line = fmt.Sprintf("- /work/%s/ — %s", dir, d.Summary)
		} else {
			name := d.Title
			if len(d.Aliases) > 0 {
				name += "（" + strings.Join(d.Aliases, "、") + "）"
			}
			line = fmt.Sprintf("- /work/%s — %s", d.Path, name)
			if d.Summary != "" {
				line += "：" + d.Summary
			}
		}
		label, rank := kindLabel(kind)
		g, ok := byLabel[label]
		if !ok {
			g = &group{label: label, rank: rank}
			byLabel[label] = g
			groups = append(groups, g)
		}
		if d.IsIndex {
			g.lines = append([]string{line}, g.lines...) // 目录摘要排在组首
		} else {
			g.lines = append(g.lines, line)
		}
	}
	slices.SortStableFunc(groups, func(a, b *group) int {
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		return strings.Compare(a.label, b.label)
	})
	if len(groups) == 1 {
		return strings.Join(groups[0].lines, nl) // 只有一个分组时不出现分组标题
	}
	var parts []string
	for _, g := range groups {
		parts = append(parts, "### "+g.label+nl+strings.Join(g.lines, nl))
	}
	return strings.Join(parts, nl)
}
