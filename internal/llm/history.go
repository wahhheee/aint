package llm

// RepairToolHistory 返回可发送的工具历史，不修改已存储的消息。
// 工具调用只能由紧随其后的 user 消息组回答；未回答的调用丢弃，孤儿结果展开为普通内容。
// 修改助手消息后，其原生内容与后续推理签名不再适用于这段历史，按中立内容重建。
func RepairToolHistory(in []Message) []Message {
	type position struct{ message, part int }
	paired := map[position]bool{}
	pending := map[string]position{}
	previous := Role("")
	for i, m := range in {
		if m.Role == RoleAssistant && previous != RoleAssistant {
			clear(pending)
		}
		for j, p := range m.Parts {
			switch {
			case m.Role == RoleAssistant && p.Kind == PartToolCall && p.ToolCall != nil:
				pending[p.ToolCall.ID] = position{i, j}
			case m.Role == RoleUser && p.Kind == PartToolResult && p.ToolResult != nil:
				if call, ok := pending[p.ToolResult.CallID]; ok {
					paired[call], paired[position{i, j}] = true, true
					delete(pending, p.ToolResult.CallID)
				}
			}
		}
		previous = m.Role
	}

	out := make([]Message, 0, len(in))
	changed := false
	for i, m := range in {
		parts := make([]Part, 0, len(m.Parts))
		for j, p := range m.Parts {
			switch {
			case p.Kind == PartToolCall && !paired[position{i, j}]:
				changed = true
			case p.Kind == PartToolResult && !paired[position{i, j}]:
				changed = true
				if p.ToolResult != nil {
					parts = append(parts, p.ToolResult.Parts...)
				}
			default:
				parts = append(parts, p)
			}
		}
		if changed {
			m.Native = nil
			m.Parts = nil
			for _, p := range parts {
				if p.Kind != PartReasoning {
					m.Parts = append(m.Parts, p)
				}
			}
		}
		if len(m.Parts) > 0 || len(m.Native) > 0 {
			out = append(out, m)
		}
	}
	return out
}
