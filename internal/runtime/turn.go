package runtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/markup"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/tools"
	"github.com/wahhheee/yanyi/internal/usage"
	"github.com/wahhheee/yanyi/internal/work"
)

// runTurn 是一个回合的完整生命周期（在独立 goroutine 中运行）。
func (m *Manager) runTurn(ctx context.Context, saveID, turnID string, in save.TurnInput) {
	d, err := m.prepare(ctx, saveID, turnID, in)
	if err != nil {
		code, msg, recoverable := describeError(err)
		slog.WarnContext(ctx, "回合准备失败", "err", err)
		m.publish(saveID, event.TurnFailed, event.TurnFailedData{TurnID: turnID, Code: code, Message: msg, Recoverable: recoverable})
		return
	}
	slog.InfoContext(ctx, "回合开始", "kind", in.Kind, "turn_no", d.turnNo, "model", d.model.Name,
		"prefix_hash", short(d.built.PrefixHash))
	m.publish(saveID, event.TurnStarted, event.TurnStartedData{TurnID: turnID, ParentID: d.parent.ID, Input: d.input})

	m.loop(ctx, d)
	m.commit(ctx, d)
}

// prepare 读取进度线、作品与模型，封装玩家消息，构建首个请求。
func (m *Manager) prepare(ctx context.Context, saveID, turnID string, in save.TurnInput) (*draft, error) {
	sv, line, err := m.d.Saves.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, err
	}
	root, head := line[0], line[len(line)-1]
	w, err := m.d.Works.Model(ctx, head.WorkVersionID)
	if err != nil {
		return nil, err
	}
	provider, model, err := m.d.Models.Resolve(ctx, sv.Settings.NarratorModel)
	if err != nil {
		return nil, err
	}
	features := contextx.Profile(w, sv.Settings, model.Caps, false) // TODO(M5-8)：轻量模型已配置时才允许后台助手

	// 历史消息。TODO(M3-6)：从最近一次整理的 keep_from 开始，前面接阶段块；孤儿消息修复（8.4）。
	var history []llm.Message
	turnNo := 0
	for _, n := range line {
		if n.Kind != save.KindTurn {
			continue
		}
		msgs, err := m.d.Saves.LoadMessages(ctx, n.MessagesHash)
		if err != nil {
			return nil, err
		}
		history = append(history, msgs...)
		turnNo = n.TurnNo
	}
	var headDisplay []save.DisplayItem
	if head.Kind == save.KindTurn {
		if headDisplay, err = m.d.Saves.LoadDisplay(ctx, head.DisplayHash); err != nil {
			return nil, err
		}
	}
	initState, err := m.d.Saves.LoadState(ctx, root.StateHash)
	if err != nil {
		return nil, err
	}
	headState, err := m.d.Saves.LoadState(ctx, head.StateHash)
	if err != nil {
		return nil, err
	}

	// 封装玩家消息
	env := contextx.Envelope{Input: in, Pace: firstNonEmpty(in.Pace, sv.Settings.Pace, w.Manifest.Narrator.Length)}
	switch in.Kind {
	case save.InputOpening:
		if head.Kind != save.KindRoot {
			return nil, errs.Invalid("故事已经开始，不需要再开场")
		}
		env.Text = "请按开场指引开始故事。"
		if w.Opening != nil && w.Opening.Mode == "generate" {
			body, err := m.d.Tmpl.Render("prompts/opening.md", w.Opening.Body, map[string]any{"state": initState})
			if err != nil {
				return nil, errs.Wrap(err, errs.CodeWorkInvalid, "开场模板渲染失败")
			}
			env.Text += "\n\n" + body
		}
	case save.InputChoice:
		label, ok := findChoice(headDisplay, in.ChoiceID)
		if !ok {
			return nil, errs.Invalid("当前没有可选的选项 %q", in.ChoiceID)
		}
		env.Input.ChoiceLabel, env.Text = label, label
	case save.InputCommand:
		env.Text, env.ModeLabel, err = m.expandCommand(w, in.Command.Name, in.Command.Args, in.Text, headState, turnNo+1)
		if err != nil {
			return nil, err
		}
	case save.InputFree:
		env.Text = in.Text
		if in.Mode != "" { // 输入模式：以模式命令包装自由输入
			if env.Text, env.ModeLabel, err = m.expandCommand(w, in.Mode, nil, in.Text, headState, turnNo+1); err != nil {
				return nil, err
			}
		}
	default:
		env.Text = in.Text
	}
	env.PendingCall = pendingChoiceCall(history)

	// 阶段块：目前只有第 0 阶段（开局）。TODO(M3-6)：整理后使用最近一个 compaction 节点的阶段块。
	memIndex, err := m.d.Saves.MemoryIndex(ctx, root.MemoryHash)
	if err != nil {
		return nil, err
	}
	epoch := contextx.RenderEpoch0(features, w.State, initState, memIndex)

	built, err := m.d.Builder.Build(contextx.BuildInput{
		SaveID: sv.ID, Work: w, Features: features, Model: model, Settings: sv.Settings,
		InitialState: initState, InitialStateHash: root.StateHash, Epoch: epoch,
		Tools: m.d.Tools.Specs(features, w), History: history, Turn: env.Message(features),
	})
	if err != nil {
		return nil, err
	}

	d := &draft{
		id: turnID, started: time.Now(), save: sv, parent: head, turnNo: turnNo + 1, input: env.Input,
		work: w, features: features, provider: provider, model: model, built: built,
		req: built.Request, turnMsgs: []llm.Message{built.Request.Messages[len(built.Request.Messages)-1]},
	}
	if item, ok := inputDisplay(env); ok {
		d.display = append(d.display, item)
	}
	return d, nil
}

// loop 是回合主循环（《02-技术方案》9.2–9.3）。
func (m *Manager) loop(ctx context.Context, d *draft) {
	for {
		if reason := m.d.Budget.exceeded(d); reason != "" {
			d.stop = "budget"
			d.addNotice("warn", "本回合已达上限（"+reason+"），可以点“继续”接着写。")
			return
		}
		res, err := m.callOnce(ctx, d)
		if res != nil {
			d.stopRaw = res.raw
			if len(res.msg.Parts) > 0 {
				d.req.Messages = append(d.req.Messages, res.msg)
				d.turnMsgs = append(d.turnMsgs, res.msg)
			}
		}
		if err != nil {
			d.status = save.StatusInterrupted
			if ctx.Err() != nil {
				d.stop = "canceled"
				d.addNotice("info", "已停止")
			} else {
				d.stop, d.errMsg = "error", err.Error()
				_, msg, _ := describeError(err)
				d.addNotice("error", msg)
			}
			return
		}

		switch res.stop {
		case llm.StopToolUse:
			calls := res.msg.ToolCalls()
			if len(calls) == 0 {
				d.stop = string(llm.StopEnd)
				return
			}
			if m.runTools(ctx, d, calls) {
				d.stop = "choices"
				return
			}
		case llm.StopMaxTokens:
			if d.continuations >= m.d.Budget.MaxContinuations {
				d.stop = "length"
				d.addNotice("warn", "本回合篇幅已达上限，可以点“继续”接着写。")
				return
			}
			d.continuations++
			notice := llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.ContinueNotice)}}
			d.req.Messages = append(d.req.Messages, notice)
			d.turnMsgs = append(d.turnMsgs, notice)
			// TODO(M4-5)：续写开头与上一段末尾的重叠去重（见 dedupOverlap），需要缓冲续写的前若干字再推送。
		case llm.StopEnd:
			d.stop = string(llm.StopEnd)
			return
		case llm.StopContextOverflow:
			// TODO(M3-7)：紧急整理后重建请求再试一次（每回合一次）。
			d.stop, d.status = string(res.stop), save.StatusInterrupted
			d.addNotice("error", "上下文超过了模型上限。")
			return
		default: // refusal / pause / error
			// TODO(M4)：pause（Anthropic pause_turn）应原样续发请求。
			d.stop = string(res.stop)
			if res.stop == llm.StopRefusal {
				d.addNotice("warn", "模型拒绝继续这段内容，可以换一种写法再试。")
			}
			return
		}
	}
}

// runTools 执行一批工具调用并把结果追加到消息中；返回是否遇到了终止型工具（offer_choices）。
func (m *Manager) runTools(ctx context.Context, d *draft, calls []llm.ToolCall) (terminal bool) {
	d.toolCalls += len(calls)
	for _, c := range calls {
		m.publish(d.save.ID, event.TurnTool, event.TurnToolData{TurnID: d.id, CallID: c.ID, Name: c.Name, Phase: "start", Display: m.displayOf(c.Name)})
	}
	out := m.d.Tools.Execute(ctx, d, calls)
	d.toolLog = append(d.toolLog, out.Records...)

	reads := 0
	for _, r := range out.Records {
		phase := "done"
		if r.IsError {
			phase = "error"
		}
		m.publish(d.save.ID, event.TurnTool, event.TurnToolData{TurnID: d.id, CallID: r.ID, Name: r.Name, Phase: phase, Display: m.displayOf(r.Name)})
		if def, ok := m.d.Tools.Def(r.Name); ok && def.Display == tools.DisplaySubtle && !r.IsError {
			reads++
		}
	}
	d.addToolSummary(reads)

	if len(out.Results) > 0 {
		msg := llm.Message{Role: llm.RoleUser}
		for _, r := range out.Results {
			msg.Parts = append(msg.Parts, llm.Part{Kind: llm.PartToolResult, ToolResult: &r})
		}
		d.req.Messages = append(d.req.Messages, msg)
		d.turnMsgs = append(d.turnMsgs, msg)
	}
	if out.Terminal != nil && d.choices != nil {
		m.publish(d.save.ID, event.TurnChoices, event.TurnChoicesData{TurnID: d.id, Choices: *d.choices})
		return true
	}
	return false
}

func (m *Manager) displayOf(name string) string {
	if def, ok := m.d.Tools.Def(name); ok {
		return string(def.Display)
	}
	return ""
}

type callResult struct {
	msg  llm.Message
	stop llm.StopReason
	raw  string
}

// callOnce 发起一次流式调用：正文增量实时推送；返回完整的助手消息与停止原因。
func (m *Manager) callOnce(ctx context.Context, d *draft) (*callResult, error) {
	d.calls++
	purpose := "narrate"
	if d.continuations > 0 {
		purpose = "continue"
	}
	d.req.Purpose = purpose
	start := time.Now()

	stream, err := d.provider.Stream(ctx, d.model, d.req)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var text strings.Builder
	res := &callResult{}
	var u llm.Usage
	var streamErr error
	for {
		ev, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			streamErr = err
			break
		}
		switch ev := ev.(type) {
		case llm.TextDelta:
			text.WriteString(ev.Text)
			d.appendText(ev.Text)
			d.seq++
			m.publish(d.save.ID, event.TurnText, event.TurnTextData{TurnID: d.id, Seq: d.seq, Delta: ev.Text})
		case llm.Done:
			// 工具调用以 Done.Message 为准（含协议原生数据）；ToolCallStart/ArgsDelta 目前不单独处理。
			res.msg, res.stop, res.raw, u = ev.Message, ev.Stop, ev.Raw, ev.Usage
		}
	}

	if res.msg.Role == "" {
		// 没有收到 Done（流中途断开 / 被取消）：保留已收到的正文；参数不完整的工具调用丢弃（9.4）。
		res.msg = llm.Message{Role: llm.RoleAssistant}
		if text.Len() > 0 {
			res.msg.Parts = append(res.msg.Parts, llm.Text(text.String()))
		}
		res.stop = llm.StopMaxTokens
		if streamErr == nil && ctx.Err() != nil {
			streamErr = ctx.Err()
		}
		if streamErr != nil && (ctx.Err() != nil || text.Len() == 0) {
			m.recordUsage(d, purpose, u, start, "interrupted")
			return res, streamErr
		}
		// 已有正文的断流按"截断"处理：交给续写逻辑接着写（计入续写次数）。
		slog.WarnContext(ctx, "模型流中途断开，按截断续写处理", "err", streamErr, "chars", text.Len())
	}
	m.recordUsage(d, purpose, u, start, string(res.stop))
	return res, nil
}

func (m *Manager) recordUsage(d *draft, purpose string, u llm.Usage, start time.Time, stop string) {
	d.usage = d.usage.Add(u)
	rec := usage.Record{CallKind: purpose, ModelID: d.model.ID, Usage: u, LatencyMS: time.Since(start).Milliseconds(), Stop: stop}
	if cost, ok := d.model.Price.CostMicros(u, d.save.Settings.CacheTTL); ok {
		rec.CostMicros = &cost
	}
	d.records = append(d.records, rec)
	// TODO(M3-8)：上下文用量（已用 / 整理阈值）需要 token 估算与校准，目前以本次输入总量近似。
	m.publish(d.save.ID, event.TurnUsage, event.TurnUsageData{
		TurnID: d.id, Usage: d.usage,
		Context: event.ContextUsage{Used: u.InputTotal() + u.Output, Threshold: min(d.model.ContextWindow, 128_000)},
	})
}

// commit 提交回合（单事务），然后发布事件。使用不可取消的 ctx：玩家停止后仍要把已生成的内容存下来。
func (m *Manager) commit(ctx context.Context, d *draft) {
	ctx = context.WithoutCancel(ctx)
	if d.status == "" {
		d.status = save.StatusCommitted
	}
	if d.status == save.StatusInterrupted && !d.hasContent() {
		// 什么也没生成：不产生节点，head 不变。
		code, msg, recoverable := "canceled", "已停止", true
		if d.stop == "error" {
			code, msg, recoverable = describeError(errors.New(d.errMsg))
		}
		slog.InfoContext(ctx, "回合未产生内容，不提交", "stop", d.stop, "err", d.errMsg)
		m.publish(d.save.ID, event.TurnFailed, event.TurnFailedData{TurnID: d.id, Code: code, Message: msg, Recoverable: recoverable})
		return
	}
	if d.choices != nil {
		d.display = append(d.display, *d.choices)
	}

	// TODO(M2-1/M3-2)：状态与记忆有变化时写入新快照（目前工具尚未实现，沿用父节点）。
	node, err := m.d.Saves.CommitTurn(ctx, save.TurnCommit{
		SaveID: d.save.ID, Parent: d.parent, TurnNo: d.turnNo, Input: d.input, Messages: d.turnMsgs, Display: d.display,
		StateHash: d.parent.StateHash, MemoryHash: d.parent.MemoryHash, WorkVersionID: d.parent.WorkVersionID,
		Status: d.status, Usage: &d.usage, PlainText: markup.PlainText(d.markupText()), UsageRecords: d.records, NodeID: d.id,
		Meta: save.NodeMeta{
			Model: d.model.Name, Stop: d.stop, StopRaw: d.stopRaw, Calls: d.calls, ToolCalls: d.toolCalls,
			Continuations: d.continuations, DurationMS: time.Since(d.started).Milliseconds(), Error: d.errMsg,
			PrefixHash: d.built.PrefixHash, PromptVersion: prompts.Version,
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "回合提交失败", "err", err)
		m.publish(d.save.ID, event.TurnFailed, event.TurnFailedData{TurnID: d.id, Code: string(errs.CodeInternal), Message: "保存回合失败（详见日志）", Recoverable: true})
		return
	}
	m.publish(d.save.ID, event.TurnCommitted, event.TurnCommittedData{TurnID: d.id, NodeID: node.ID, Status: string(node.Status)})
	if d.stop == "error" {
		code, msg, recoverable := describeError(errors.New(d.errMsg))
		m.publish(d.save.ID, event.TurnFailed, event.TurnFailedData{TurnID: d.id, Code: code, Message: msg, Recoverable: recoverable})
	}
	m.publishPanels(ctx, d.save.ID)
	slog.InfoContext(ctx, "回合结束", "stop", d.stop, "status", d.status, "calls", d.calls, "tool_calls", d.toolCalls,
		"duration", time.Since(d.started).Round(time.Millisecond), "output_tokens", d.usage.Output)
}

// ---------------------------------------------------------------------------
// 辅助

// expandCommand 解析命令并展开模板（《02-技术方案》11.3）。
func (m *Manager) expandCommand(w *work.Model, name string, args map[string]string, input string, st map[string]any, turn int) (text, modeLabel string, err error) {
	var cmd *work.Command
	for i := range w.Commands {
		if w.Commands[i].Name == name {
			cmd = &w.Commands[i]
		}
	}
	if cmd == nil {
		return "", "", errs.Invalid("没有命令 /%s", name)
	}
	for _, a := range cmd.Args {
		if a.Required && strings.TrimSpace(args[a.Name]) == "" {
			return "", "", errs.Invalid("命令 /%s 需要参数 %s", name, a.Name)
		}
	}
	if cmd.HasPresent("mode") {
		modeLabel = firstNonEmpty(cmd.ModeLabel, cmd.Title)
	}
	if cmd.Template == "" {
		return input, modeLabel, nil
	}
	argVars := map[string]any{}
	for k, v := range args {
		argVars[k] = v
	}
	text, err = m.d.Tmpl.Render("commands/"+name+".md", cmd.Template,
		map[string]any{"args": argVars, "input": input, "state": st, "turn": turn})
	if err != nil {
		return "", "", errs.Wrap(err, errs.CodeWorkInvalid, "命令 /%s 的模板渲染失败", name)
	}
	return strings.TrimSpace(text), modeLabel, nil
}

// pendingChoiceCall 返回历史末尾未作答的 offer_choices 调用 ID（上一回合以选项结束）。
func pendingChoiceCall(history []llm.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		if msg.Role != llm.RoleAssistant {
			continue
		}
		for _, c := range msg.ToolCalls() {
			if c.Name == "offer_choices" {
				return c.ID
			}
		}
		return ""
	}
	return ""
}

// findChoice 在上一回合的显示项中找到选项文字。
func findChoice(display []save.DisplayItem, id string) (string, bool) {
	for i := len(display) - 1; i >= 0; i-- {
		if display[i].T != "choices" {
			continue
		}
		for _, it := range display[i].Items {
			if strings.EqualFold(it.ID, id) {
				return it.Label, true
			}
		}
	}
	return "", false
}

// inputDisplay 生成玩家输入行的显示项（开场不显示输入行）。
func inputDisplay(env contextx.Envelope) (save.DisplayItem, bool) {
	in := env.Input
	item := save.DisplayItem{T: "input", Kind: in.Kind, Mode: env.ModeLabel, Attachments: in.Attachments}
	switch in.Kind {
	case save.InputOpening:
		return item, false
	case save.InputChoice:
		item.Text = in.ChoiceLabel
	case save.InputCommand:
		item.Text = "/" + in.Command.Name
		for _, v := range in.Command.Args {
			item.Text += " " + v
		}
		if in.Text != "" {
			item.Text += " " + in.Text
		}
	case save.InputContinue:
		item.Text = firstNonEmpty(in.Text, "继续")
	default:
		item.Text = in.Text
	}
	return item, true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func short(hash string) string {
	if len(hash) > 19 {
		return hash[:19]
	}
	return hash
}
