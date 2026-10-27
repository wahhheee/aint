package runtime

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
)

var rewriteMemoryTools = map[string]bool{"read": true, "list": true, "search": true, "get_state": true, "write": true, "edit": true, "remove": true}

// reviewRewriteMemory 以实际成稿核对本轮记忆，不携带旧版参考、修改要求或生成时的计划。
// 核对仍使用草稿覆盖层；普通回复只用于调试，工具往返合入故事历史。
func (m *Manager) reviewRewriteMemory(ctx context.Context, d *draft) error {
	var body strings.Builder
	for _, item := range d.display {
		if item.T == "text" {
			body.WriteString(item.Markup)
			body.WriteString("\n\n")
		}
	}
	changed := map[string]string{}
	for path, data := range memoryWrite(d.memory).Data {
		changed[path] = string(data)
	}
	payload := string(jsonx.MustMarshal(map[string]any{"story": body.String(), "memory": changed, "state": d.state}))
	mainReq, mainMsgs := d.req, d.turnMsgs
	lastInput, lastOutput := d.lastInput, d.lastOutput
	req := *mainReq
	input := mainMsgs[0]
	input.Parts = append(slices.Clone(input.Parts), llm.Text(prompts.RewriteMemoryCheck+"\n"+payload))
	req.Messages = append(slices.Clone(mainReq.Messages[:d.contextMessages]), input)
	d.req, d.turnMsgs, d.reviewing = &req, []llm.Message{input}, true
	defer func() {
		d.req, d.reviewing = mainReq, false
		d.lastInput, d.lastOutput = lastInput, lastOutput
	}()
	failedTools := false
	var pending []llm.Message
	if d.choices != nil {
		pending = choiceHistory(mainMsgs)
	}
	finish := func() { d.turnMsgs = mergeMemoryReview(mainMsgs, d.turnMsgs[1:], pending) }
	for range 4 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if reason := d.budget.exceeded(d); reason != "" {
			return fmt.Errorf("记忆核对未完成：%s", reason)
		}
		res, err := m.callOnce(ctx, d)
		if err != nil {
			return err
		}
		if len(res.textTools) > 0 {
			return fmt.Errorf("记忆核对未能使用原生工具调用")
		}
		d.req.Messages = append(d.req.Messages, res.msg)
		d.turnMsgs = append(d.turnMsgs, res.msg)
		switch res.stop {
		case llm.StopToolUse:
			calls := res.msg.ToolCalls()
			m.runTools(ctx, d, calls)
			failedTools = false
			wrote := false
			for _, r := range d.toolLog[len(d.toolLog)-len(calls):] {
				failedTools = failedTools || r.IsError
				wrote = wrote || (!r.IsError && (r.Name == "write" || r.Name == "edit" || r.Name == "remove"))
			}
			if wrote && !failedTools {
				finish()
				return nil
			}
		case llm.StopEnd:
			if failedTools {
				return fmt.Errorf("记忆核对仍有未完成的工具操作")
			}
			finish()
			return nil
		case llm.StopPause:
			continue
		default:
			return fmt.Errorf("记忆核对没有正常完成：%s", res.stop)
		}
	}
	return fmt.Errorf("记忆核对达到调用上限")
}

// mergeMemoryReview 保留实际工具操作，并把待答选项放在核对之后，供下一轮玩家回填。
func mergeMemoryReview(main, review, pending []llm.Message) []llm.Message {
	pendingID := ""
	if len(pending) > 0 {
		pendingID = pending[0].ToolCalls()[0].ID
	}
	var out []llm.Message
	for _, msg := range stripNative(main) {
		msg.Parts = slices.DeleteFunc(slices.Clone(msg.Parts), func(p llm.Part) bool { return p.ToolCall != nil && p.ToolCall.ID == pendingID })
		if len(msg.Parts) > 0 {
			out = append(out, msg)
		}
	}
	for _, msg := range review {
		msg.Native = nil
		msg.Parts = slices.DeleteFunc(slices.Clone(msg.Parts), func(p llm.Part) bool { return p.Kind != llm.PartToolCall && p.Kind != llm.PartToolResult })
		if len(msg.Parts) > 0 {
			out = append(out, msg)
		}
	}
	return append(out, pending...)
}
