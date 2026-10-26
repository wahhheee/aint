package contextx

import (
	"fmt"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
)

// RenderToolGuide 用本次请求的工具描述生成自然语言用法，顺序与 API 工具定义一致。
// 参数的类型、必填项和完整约束由 API schema 提供。
func RenderToolGuide(specs []llm.ToolSpec) string {
	if len(specs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## 工具用法")
	for _, spec := range specs {
		fmt.Fprintf(&b, "\n- `%s`：%s", spec.Name, spec.Description)
	}
	return b.String()
}
