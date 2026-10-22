package contextx

import (
	"fmt"
	"strings"

	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/state"
)

// RenderEpoch0 渲染第 0 阶段（开局）的阶段块（《02-技术方案》13.4）：
// 只含初始状态与记忆目录中存在的部分；全部为空时返回空串（不生成 L4，也不放断点 ②）。
//
// 第 0 阶段块只由存档创建时的初始状态与初始记忆决定（二者不可变），因此每次构建时即时渲染即可，
// 结果字节稳定，不需要像整理产生的阶段块那样存为 blob。
//
// TODO(M3-6)：整理产生的阶段块（含交接摘要、from_turn、保留原文说明）在 compact 中渲染并存入 compaction 节点。
func RenderEpoch0(fs prompts.FeatureSet, schema *state.Schema, st map[string]any, memory []save.MemoryEntry) string {
	var parts []string
	if fs.Has(prompts.FState) && !schema.Empty() {
		b, err := state.YAML(schema, st)
		if err == nil {
			parts = append(parts, fmt.Sprintf("<state as_of=\"开局\">\n%s</state>", b))
		}
	}
	if len(memory) > 0 {
		var lines []string
		for _, e := range memory {
			line := "- " + e.Path
			if e.Summary != "" {
				line += " — " + e.Summary
			}
			lines = append(lines, line)
		}
		parts = append(parts, "<memory_index>\n"+strings.Join(lines, "\n")+"\n</memory_index>")
	}
	if len(parts) == 0 {
		return ""
	}
	return "<epoch n=\"0\">\n" + strings.Join(parts, "\n") + "\n</epoch>"
}
