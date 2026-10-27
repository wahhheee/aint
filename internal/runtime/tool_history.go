package runtime

import (
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/work"
)

// toolHistoryCleaner 构建不含伪调用示范的发送历史，不修改存储消息。
// 历史被改写后，其后的原生内容与推理签名不再对应当前前缀，使用中立消息重建。
type toolHistoryCleaner struct {
	specs   []llm.ToolSpec
	tags    []work.TagDef
	changed bool
}

func (c *toolHistoryCleaner) clean(messages []llm.Message) []llm.Message {
	rejected := map[string]bool{}
	for _, msg := range messages {
		for _, p := range msg.Parts {
			if r := p.ToolResult; p.Kind == llm.PartToolResult && r != nil && r.IsError {
				for _, result := range r.Parts {
					if result.Kind == llm.PartText && result.Text == prompts.ToolTextRejectedCall {
						rejected[r.CallID] = true
					}
				}
			}
		}
	}
	var out []llm.Message
	for _, msg := range messages {
		parts := make([]llm.Part, 0, len(msg.Parts))
		// 文本块共用过滤器，标签可以横跨同一条消息的多个文本块。
		filter := newToolTextFilter(c.specs, c.tags)
		for _, p := range msg.Parts {
			switch {
			case msg.Role == llm.RoleAssistant && p.Kind == llm.PartText:
				text := filter.push(p.Text)
				if text != "" {
					parts = append(parts, llm.Text(text))
				}
			case p.Kind == llm.PartToolCall && p.ToolCall != nil && rejected[p.ToolCall.ID]:
				c.changed = true
			case p.Kind == llm.PartToolResult && p.ToolResult != nil && rejected[p.ToolResult.CallID]:
				c.changed = true
			case msg.Role == llm.RoleUser && p.Kind == llm.PartText && isToolRepairNotice(p.Text):
				c.changed = true
			default:
				parts = append(parts, p)
			}
		}
		if tail := filter.flush(); tail != "" {
			parts = append(parts, llm.Text(tail))
		}
		if len(filter.found) > 0 {
			c.changed = true
		}
		if c.changed {
			msg.Native = nil
			msg.Parts = nil
			for _, p := range parts {
				if p.Kind != llm.PartReasoning {
					msg.Parts = append(msg.Parts, p)
				}
			}
		}
		if len(msg.Parts) > 0 || len(msg.Native) > 0 {
			out = append(out, msg)
		}
	}
	return out
}

func isToolRepairNotice(text string) bool {
	prefix, suffix, _ := strings.Cut(prompts.ToolTextRepairNotice, "%s")
	return strings.HasPrefix(text, prefix) && strings.HasSuffix(text, suffix)
}
