package anthropic

import (
	"strings"

	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// markRolling 在最后一个可缓存内容块放断点；thinking 块不能显式标记。
func markRolling(messages []message, cc cacheControl) {
	for i := len(messages) - 1; i >= 0; i-- {
		for j := len(messages[i].Content) - 1; j >= 0; j-- {
			b := messages[i].Content[j]
			var block struct {
				Type string `json:"type"`
			}
			if jsonx.Unmarshal(b, &block) != nil {
				continue
			}
			if block.Type == "thinking" || block.Type == "redacted_thinking" {
				continue
			}
			if !hasCache(b) {
				messages[i].Content[j] = withCache(b, cc)
			}
			return
		}
	}
}

// anchorCache 在新增内容超出 20 个回溯位置时保留前一次 user 消息末尾的断点。
// 连续 tool_use / tool_result 各算一个位置；最多四个断点，优先保留 system、历史锚点与最新末尾。
func anchorCache(out *request, cc cacheControl) {
	anchor := -1
	for i := len(out.Messages) - 2; i >= 0; i-- {
		if out.Messages[i].Role == "user" {
			anchor = i
			break
		}
	}
	if anchor < 0 {
		return
	}
	positions := 0
	for _, m := range out.Messages[anchor+1:] {
		previous := ""
		for _, b := range m.Content {
			var block struct {
				Type string `json:"type"`
			}
			if jsonx.Unmarshal(b, &block) != nil {
				return
			}
			if block.Type != previous || (block.Type != "tool_use" && block.Type != "tool_result") {
				positions++
			}
			previous = block.Type
		}
	}
	if positions < 20 {
		return
	}
	last := len(out.Messages[anchor].Content) - 1
	if last < 0 || hasCache(out.Messages[anchor].Content[last]) {
		return
	}
	count := 0
	for _, b := range out.System {
		if hasCache(b) {
			count++
		}
	}
	for _, m := range out.Messages {
		for _, b := range m.Content {
			if hasCache(b) {
				count++
			}
		}
	}
	if count >= maxBreakpoints {
		// 较晚的静态消息断点让出名额，不改动内容字段的排列。
		for i := anchor; i >= 0 && count >= maxBreakpoints; i-- {
			for j := len(out.Messages[i].Content) - 1; j >= 0 && count >= maxBreakpoints; j-- {
				b := out.Messages[i].Content[j]
				if hasCache(b) {
					// withCache 总在对象末尾追加字段，直接去掉尾部成员以保持前缀字节。
					out.Messages[i].Content[j] = withoutCache(b)
					count--
				}
			}
		}
	}
	if count < maxBreakpoints {
		out.Messages[anchor].Content[last] = withCache(out.Messages[anchor].Content[last], cc)
	}
}

func withoutCache(b jsonx.Value) jsonx.Value {
	s := string(b)
	if i := strings.LastIndex(s, `,"cache_control":`); i >= 0 {
		return jsonx.Value(s[:i] + "}")
	}
	return b
}
