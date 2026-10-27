package runtime

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/markup"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
)

var choiceTools = map[string]bool{"read": true, "list": true, "search": true, "get_state": true, "offer_choices": true}

// Operate 重写最后一个故事回合，或为它补充选项。目标与当前进度必须同时匹配。
func (m *Manager) Operate(ctx context.Context, saveID string, op save.TurnOperation) (string, error) {
	op.Instructions = strings.TrimSpace(op.Instructions)
	if op.Kind != "rewrite" && op.Kind != "choices" {
		return "", errs.Invalid("未知的回合操作")
	}
	if op.ExpectedHead == "" {
		return "", errs.Invalid("缺少当前进度")
	}
	if op.Kind == "rewrite" && (op.Instructions == "" || len([]rune(op.Instructions)) > 8000) {
		return "", errs.Invalid("重写要求需要 1–8000 字")
	}
	sv, line, err := m.d.Saves.ProgressLine(ctx, saveID)
	if err != nil {
		return "", err
	}
	if sv.HeadNodeID != op.ExpectedHead {
		return "", errs.Conflict("存档已变化，请刷新后重试")
	}
	n := lastTurn(line)
	if n == nil || n.ID != op.NodeID || n.Input == nil {
		return "", errs.Invalid("只能操作当前进度的最后一个故事回合")
	}
	display, err := m.d.Saves.TurnDisplay(ctx, line, n)
	if err != nil {
		return "", err
	}
	var body strings.Builder
	for _, it := range display {
		if it.T == "text" {
			body.WriteString(it.Markup)
		}
	}
	if strings.TrimSpace(markup.PlainText(body.String())) == "" {
		return "", errs.Invalid("当前回合没有故事正文")
	}
	input, parent := *n.Input, n.ParentID
	if op.Kind == "rewrite" {
		if n.Meta.Stop == "static_opening" {
			return "", errs.Invalid("静态开场是作品写好的，不能按要求重写")
		}
	} else {
		if slices.ContainsFunc(display, func(it save.DisplayItem) bool { return it.T == "choices" && len(it.Items) > 0 }) {
			return "", errs.Invalid("当前回合已有可用选项")
		}
		parent, input = sv.HeadNodeID, save.TurnInput{Kind: save.InputContinue}
	}
	input.ExpectedHeadID, input.ChoiceLabel = op.ExpectedHead, ""
	return m.submitOperation(ctx, saveID, parent, input, &op, "")
}

func operationValue(op *save.TurnOperation) any {
	if op == nil {
		return nil
	}
	return op
}

func (m *Manager) prepareOperation(ctx context.Context, d *draft, op *save.TurnOperation) error {
	d.operation = op
	d.contextMessages = len(d.req.Messages) - 1
	d.input.ExpectedHeadID = ""
	if d.isChoices() {
		d.turnNo = d.parent.TurnNo
		d.display = nil
		d.stateChanged, d.migration = false, nil
	} else {
		n, err := m.d.Saves.Node(ctx, d.save.ID, op.NodeID)
		if err != nil {
			return err
		}
		d.versionParent = n.Meta.VersionParent
		if d.versionParent == "" {
			d.versionParent = n.ParentID
		}
		line, err := m.d.Saves.ProgressLineAt(ctx, d.save.ID, op.ExpectedHead)
		if err != nil {
			return err
		}
		items, err := m.d.Saves.TurnDisplay(ctx, line, n)
		if err != nil {
			return err
		}
		items = slices.DeleteFunc(items, func(it save.DisplayItem) bool { return it.T != "text" && it.T != "choices" })
		d.reference = string(jsonx.MustMarshal(items))
	}
	d.req.Messages[len(d.req.Messages)-1] = d.operationMessage(d.turnMsgs[0])
	return nil
}

// operationMessage 只改变本次调用的输入副本，普通故事历史保留原玩家输入。
func (d *draft) operationMessage(input llm.Message) llm.Message {
	if d.operation == nil {
		return input
	}
	if d.isChoices() {
		return llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.RequestChoices + "\n<current_state>" + string(jsonx.MustMarshal(d.state)) + "</current_state>")}}
	}
	input.Parts = slices.Clone(input.Parts)
	input.Parts = append(input.Parts, llm.Text(prompts.RewriteTurn+"\n<rewrite_request>\n"+string(jsonx.MustMarshal(map[string]string{
		"instructions": d.operation.Instructions, "reference": d.reference,
	}))+"\n</rewrite_request>"))
	return input
}

func (d *draft) operationToolResult(msg llm.Message) llm.Message {
	if d.operation != nil && !d.isChoices() && !d.reviewing && d.hasNarrative() {
		msg.Parts = append(slices.Clone(msg.Parts), llm.Text(prompts.RewriteContinueNotice))
	}
	return msg
}

// choiceHistory 只把实际生成的选项作为助手调用加入历史，操作旁白与只读查询留在检查器中。
func choiceHistory(msgs []llm.Message) []llm.Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, call := range msgs[i].ToolCalls() {
			if call.Name == "offer_choices" {
				return []llm.Message{{Role: llm.RoleAssistant, Parts: []llm.Part{{Kind: llm.PartToolCall, ToolCall: &call}}}}
			}
		}
	}
	return nil
}

func (m *Manager) abortOperation(ctx context.Context, d *draft) {
	ctx = context.WithoutCancel(ctx)
	slog.InfoContext(ctx, "回合操作未采用", "kind", d.operation.Kind, "stop", d.stop, "err", d.err)
	records := slices.Clone(d.records)
	if d.staged != nil {
		records = append(records, d.staged.Records...)
	}
	if err := m.d.Saves.RecordAttempt(ctx, d.save.ID, records); err != nil {
		slog.ErrorContext(ctx, "保存操作用量失败", "err", err)
	}
	code, msg := d.stop, "重写没有完成，已恢复原版。"
	if d.isChoices() {
		msg = "选项没有生成，当前正文和状态保持不变。"
	}
	if d.stop == "canceled" {
		msg = "已取消，当前正文和状态保持不变。"
	}
	if d.err != nil {
		_, detail, _ := describeError(d.err)
		msg += detail
	}
	d.release()
	m.publish(d.save.ID, event.TurnFailed, event.TurnFailedData{TurnID: d.id, Code: code, Message: msg, Recoverable: d.stop != "canceled"})
	m.publishPanels(ctx, d.save.ID)
}
