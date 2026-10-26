package contextx

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/state"
)

// RenderEpoch0 渲染第 0 阶段（开局）的阶段块（《02-技术方案》13.4）：
// 只含初始状态与记忆目录中存在的部分；全部为空时返回空串（不生成 L4，也不放断点 ②）。
//
// 第 0 阶段块只由存档创建时的初始状态与初始记忆决定（二者不可变），因此每次构建时即时渲染即可，
// 结果字节稳定，不需要像整理产生的阶段块那样存为 blob。
// 整理产生的阶段块由 RenderEpoch 渲染，存为 compaction 节点的 blob，阶段内原样复用。
func RenderEpoch0(fs prompts.FeatureSet, schema *state.Schema, st map[string]any, memory []save.MemoryEntry) string {
	parts := epochParts(fs, schema, st, "开局", memory)
	if len(parts) == 0 {
		return ""
	}
	return "<epoch n=\"0\">\n" + strings.Join(parts, "\n") + "\n</epoch>"
}

// EpochInput 是整理产生的阶段块的内容（13.4）。
type EpochInput struct {
	N        int // 阶段序号（第几次整理）
	FromTurn int // 新阶段从哪一回合开始（整理时的最后一个回合 + 1）
	KeptFrom int // 保留原文的起止回合；KeptFrom 为 0 表示没有保留原文
	KeptTo   int
	Summary  string
	State    map[string]any // 阶段开始时的状态
	Memory   []save.MemoryEntry
}

// RenderEpoch 渲染整理产生的阶段块：说明首句、交接摘要、阶段开始时的状态（作品定义了状态时）、记忆目录（/memory 非空时）。
func RenderEpoch(fs prompts.FeatureSet, schema *state.Schema, in EpochInput) string {
	intro := prompts.EpochIntroNoKept
	if in.KeptFrom > 0 {
		intro = fmt.Sprintf(prompts.EpochIntro, in.KeptFrom, in.KeptTo)
	}
	parts := []string{intro, "<summary>\n" + strings.TrimSpace(in.Summary) + "\n</summary>"}
	parts = append(parts, epochParts(fs, schema, in.State, fmt.Sprintf("阶段开始（第 %d 回合）", in.FromTurn), in.Memory)...)
	return fmt.Sprintf("<epoch n=\"%d\" from_turn=\"%d\">\n%s\n</epoch>", in.N, in.FromTurn, strings.Join(parts, "\n"))
}

// epochParts 是阶段块中的状态快照与记忆目录（各自只在有内容时出现）。
func epochParts(fs prompts.FeatureSet, schema *state.Schema, st map[string]any, asOf string, memory []save.MemoryEntry) []string {
	var parts []string
	if fs.Has(prompts.FState) && !schema.Empty() {
		if b, err := state.YAML(schema, st); err == nil {
			parts = append(parts, fmt.Sprintf("<state as_of=\"%s\">\n%s</state>", asOf, b))
		}
	}
	if index := RenderMemoryIndex(memory); index != "" {
		parts = append(parts, index)
	}
	return parts
}

// RenderMemoryIndex 渲染阶段块与独立整理共用的记忆目录；没有记忆时不生成块。
func RenderMemoryIndex(memory []save.MemoryEntry) string {
	if len(memory) == 0 {
		return ""
	}
	var lines []string
	for _, e := range memory {
		line := "- " + e.Path
		if e.Summary != "" {
			line += " — " + e.Summary
		}
		lines = append(lines, line)
	}
	return "<memory_index>\n" + strings.Join(lines, "\n") + "\n</memory_index>"
}

// RenderCompactionTask 渲染整理指令（prompts.CompactionTask）。
func RenderCompactionTask(fs prompts.FeatureSet, summaryChars int, memoryLarge bool, guide string) (string, error) {
	tpl, err := template.New("compaction").
		Funcs(template.FuncMap{"has": func(f string) bool { return fs.Has(prompts.Feature(f)) }}).
		Parse(prompts.CompactionTask)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = tpl.Execute(&b, map[string]any{"SummaryChars": summaryChars, "MemoryLarge": memoryLarge, "Guide": strings.TrimSpace(guide)})
	return b.String(), err
}
