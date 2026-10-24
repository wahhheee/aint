package runtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/wahhheee/yanyi/internal/compact"
	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/panel"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/tools"
	"github.com/wahhheee/yanyi/internal/usage"
	"github.com/wahhheee/yanyi/internal/vfs"
	"github.com/wahhheee/yanyi/internal/work"
)

// runTurn 是一个回合的完整生命周期（在独立 goroutine 中运行）。
func (m *Manager) runTurn(ctx context.Context, saveID, turnID, parentID string, in save.TurnInput, slot *turnSlot) {
	d, err := m.prepare(ctx, saveID, turnID, parentID, in)
	if err != nil {
		code, msg, recoverable := describeError(err)
		slog.WarnContext(ctx, "回合准备失败", "err", err)
		slot.release()
		m.publish(saveID, event.TurnFailed, event.TurnFailedData{TurnID: turnID, Code: code, Message: msg, Recoverable: recoverable})
		return
	}
	slog.InfoContext(ctx, "回合开始", "kind", in.Kind, "turn_no", d.turnNo, "model", d.model.Name,
		"prefix_hash", short(d.built.PrefixHash))
	m.publish(saveID, event.TurnStarted, event.TurnStartedData{TurnID: turnID, ParentID: d.parent.ID, Input: d.input})

	d.slot, d.release = slot, slot.release
	refresh := m.panelRefresher(d)
	d.onState = func(changes []state.Change) {
		m.publish(saveID, event.TurnState, event.TurnStateData{TurnID: turnID, Changes: changes})
		refresh.schedule(d.state)
	}
	d.onCard = func(item save.DisplayItem) {
		m.publish(saveID, event.TurnDisplay, event.TurnDisplayData{TurnID: turnID, Item: item})
	}
	m.loop(ctx, d)
	refresh.stop()
	m.commit(ctx, d)
}

// prepare 读取进度线（到父节点为止；parentID 为空时到 head）、作品与模型，封装玩家消息，构建首个请求。
func (m *Manager) prepare(ctx context.Context, saveID, turnID, parentID string, in save.TurnInput) (*draft, error) {
	sv, line, err := m.d.Saves.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, err
	}
	if parentID != "" && parentID != sv.HeadNodeID {
		if line, err = m.d.Saves.ProgressLineAt(ctx, saveID, parentID); err != nil {
			return nil, err
		}
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

	// 阶段块与历史消息：最近一次整理的阶段块 + 保留的回合 + 之后的回合（第 0 阶段为开局阶段块 + 全部回合）
	ec, err := m.loadContext(ctx, w, features, line)
	if err != nil {
		return nil, err
	}
	// 上一回合（head 可能是其后的编辑或整理节点）：选项从它的显示项中找
	turnNo := 0
	var headDisplay []save.DisplayItem
	if last := lastTurn(line); last != nil {
		turnNo = last.TurnNo
		if headDisplay, err = m.d.Saves.LoadDisplay(ctx, last.DisplayHash); err != nil {
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
	env := contextx.Envelope{Input: in, Notes: save.PendingNotes(line), Pace: firstNonEmpty(in.Pace, sv.Settings.Pace, w.Manifest.Narrator.Length)}
	if in.ChoiceID != "" {
		label, ok := findChoice(headDisplay, in.ChoiceID)
		if !ok {
			return nil, errs.Invalid("当前没有可选的选项 %q", in.ChoiceID)
		}
		env.Input.ChoiceLabel = label
	}
	switch in.Kind {
	case save.InputOpening:
		if head.Kind != save.KindRoot {
			return nil, errs.Invalid("故事已经开始，不需要再开场")
		}
		env.Text = "请按开场指引开始故事。"
		if w.Opening != nil && w.Opening.Mode == "generate" {
			body, err := m.d.Tmpl.RenderWith("prompts/opening.md", w.Opening.Body, map[string]any{"state": initState}, w.Partials)
			if err != nil {
				return nil, errs.Wrap(err, errs.CodeWorkInvalid, "开场模板渲染失败")
			}
			env.Text += "\n\n" + body
		}
	case save.InputChoice:
		env.Text = env.Input.ChoiceLabel
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
	env.PendingCall = pendingChoiceCall(ec.history)
	uploads, err := m.uploads(ctx, line, in.Attachments, turnNo+1)
	if err != nil {
		return nil, err
	}
	for _, u := range uploads.current {
		env.Uploads = append(env.Uploads, u.Path)
		if features.Has(prompts.FVision) {
			ref, err := m.d.Images.ForModel(ctx, u.Blob, model.Caps.ImageMaxEdge)
			if err != nil {
				return nil, errs.Invalid("附带的图片无法读取：%v", err)
			}
			env.Images = append(env.Images, ref)
		}
	}

	built, err := m.d.Builder.Build(ctx, contextx.BuildInput{
		SaveID: sv.ID, Work: w, Features: features, Model: model, Settings: sv.Settings,
		InitialState: initState, InitialStateHash: root.StateHash, Epoch: ec.block,
		Tools: m.d.Tools.Specs(features, w), History: ec.history, Turn: env.Message(features),
	})
	if err != nil {
		return nil, err
	}

	fsys, memory, err := m.buildVFS(ctx, w, features, sv.ID, line, turnNo+1, uploads.all)
	if err != nil {
		return nil, err
	}

	params := m.compactParams(ctx, sv.Settings)
	since := len(ec.turns) + 1 // 第 0 阶段：全部回合
	if ec.compaction != nil {
		since = 1
		for _, n := range line[slices.Index(line, ec.compaction)+1:] {
			if n.Kind == save.KindTurn {
				since++
			}
		}
	}
	d := &draft{
		id: turnID, started: time.Now(), save: sv, parent: head, root: root, initState: initState, turnNo: turnNo + 1, input: env.Input,
		work: w, features: features, provider: provider, model: model, built: built, vfs: fsys, memory: memory,
		state: headState, images: m.d.Images,
		threshold: params.Threshold(model.ContextWindow, maxOutput(model)), hardLimit: params.HardLimit(model.ContextWindow, maxOutput(model)),
		sinceCompaction: since,
		req:             built.Request, turnMsgs: []llm.Message{built.Request.Messages[len(built.Request.Messages)-1]},
	}
	if item, ok := inputDisplay(env); ok {
		d.display = append(d.display, item)
	}
	return d, nil
}

// loop 是回合主循环（《02-技术方案》9.2–9.3）。
func (m *Manager) loop(ctx context.Context, d *draft) {
	choicesNudged := false
	for {
		if reason := m.d.Budget.exceeded(d); reason != "" {
			d.stop = "budget"
			d.addNotice("warn", "本回合已达上限（"+reason+"），可以点“继续”接着写。")
			return
		}
		// 硬上限：下一次调用的上下文估算达到 窗口 − 预留 时，先在回合内整理（每回合至多一次）
		if !d.compacted && m.estimate(d.model, d.req) >= d.hardLimit {
			if err := m.compactInTurn(ctx, d, compact.ReasonLimit); err != nil && !errors.Is(err, errNothingToCompact) {
				slog.WarnContext(ctx, "回合内整理失败，照常继续", "err", err)
			}
		}
		res, err := m.callOnce(ctx, d)
		// 模型报上下文超限：紧急整理后重试一次（13.2 第 3 点）
		if err != nil && ctx.Err() == nil && llm.KindOf(err) == llm.ErrContextOverflow && !d.compacted {
			if cerr := m.compactInTurn(ctx, d, compact.ReasonOverflow); cerr == nil {
				continue
			} else {
				slog.WarnContext(ctx, "紧急整理失败", "err", cerr)
			}
		}
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
				d.stop, d.err, d.errMsg = "error", err, err.Error()
				_, msg, _ := describeError(err)
				d.addNotice("error", msg)
			}
			return
		}

		switch res.stop {
		case llm.StopToolUse:
			calls := res.msg.ToolCalls()
			if len(calls) > 0 {
				if m.runTools(ctx, d, calls) {
					d.stop = "choices"
					return
				}
				continue
			}
			fallthrough
		case llm.StopEnd:
			if d.features.Has(prompts.FForceChoices) && d.choices == nil && d.hasNarrative() {
				if !choicesNudged {
					choicesNudged = true
					notice := llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.ForceChoicesNotice)}}
					d.req.Messages = append(d.req.Messages, notice)
					d.turnMsgs = append(d.turnMsgs, notice)
					continue
				}
				d.stop, d.status = "choices_missing", save.StatusInterrupted
				d.addNotice("warn", "模型仍未给出选项，已保留正文，可以点“继续”再试。")
				return
			}
			d.stop = string(llm.StopEnd)
			return
		case llm.StopMaxTokens:
			if d.continuations >= m.d.Budget.MaxContinuations {
				d.stop = "length"
				d.addNotice("warn", "本回合篇幅已达上限，可以点“继续”接着写。")
				return
			}
			d.continuations++
			d.continuing = true // 下一次调用的开头与本段末尾去重（callOnce）
			notice := llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.ContinueNotice)}}
			d.req.Messages = append(d.req.Messages, notice)
			d.turnMsgs = append(d.turnMsgs, notice)
		case llm.StopContextOverflow:
			// 输出写到了窗口上限：紧急整理后接着写（与截断续写相同），每回合一次
			if !d.compacted {
				if err := m.compactInTurn(ctx, d, compact.ReasonOverflow); err == nil {
					d.continuing = true
					notice := llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.ContinueNotice)}}
					d.req.Messages = append(d.req.Messages, notice)
					d.turnMsgs = append(d.turnMsgs, notice)
					continue
				}
			}
			d.stop, d.status = string(res.stop), save.StatusInterrupted
			d.addNotice("error", "上下文超过了模型上限，整理也没能腾出空间。可以开启独立整理，或在设置中调低上下文预算。")
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
	d.inspection.toolsDone(ctx, out, d.vfs, d.state)
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
	// 截断后的续写：先缓冲开头，去掉与上一段末尾重叠的部分再显示（9.4；只影响显示，LLM 消息保留原文）。
	var overlap *overlapFilter
	if d.continuing {
		purpose = "continue"
		overlap = newOverlapFilter(d.lastText())
		d.continuing = false
	}
	d.req.Purpose = purpose
	start := time.Now()
	raw := contextx.EstimateRequest(d.req)

	d.inspection = m.debugStart(ctx, d.save.ID, d.id, d.turnNo, d.provider, d.model, d.req, d.vfs, d.state)
	stream, err := debugStream(ctx, d.provider, d.model, d.req, d.inspection)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := stream.Close(); err != nil {
			slog.WarnContext(ctx, "关闭模型流失败", "err", err)
		}
	}()
	show := func(s string) {
		if s == "" {
			return
		}
		d.appendText(s)
		d.seq++
		m.publish(d.save.ID, event.TurnText, event.TurnTextData{TurnID: d.id, Seq: d.seq, Delta: s})
	}

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
			show(overlap.push(ev.Text))
		case llm.Done:
			// 工具调用以 Done.Message 为准（含协议原生数据）；ToolCallStart/ArgsDelta 目前不单独处理。
			res.msg, res.stop, res.raw, u = ev.Message, ev.Stop, ev.Raw, ev.Usage
		}
	}
	show(overlap.flush()) // 续写在缓冲满之前就结束时，剩余部分在这里放行

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
	if in := u.InputTotal(); in > 0 {
		m.calibrate(d.model.ID, raw, in)
	}
	m.recordUsage(d, purpose, u, start, string(res.stop))
	return res, nil
}

func (m *Manager) recordUsage(d *draft, purpose string, u llm.Usage, start time.Time, stop string) {
	d.usage = d.usage.Add(u)
	rec := usage.Record{CallKind: purpose, ModelID: d.model.ID, Usage: u, LatencyMS: time.Since(start).Milliseconds(), Stop: stop}
	if cost, ok := d.model.Price.CostMicros(u, d.save.Settings.CacheTTL); ok {
		rec.CostMicros, rec.Currency = &cost, d.model.Price.CurrencyOrDefault()
	}
	d.records = append(d.records, rec)
	if in := u.InputTotal(); in > 0 {
		d.lastInput, d.lastOutput = in, u.Output
	}
	// 上下文用量：这次调用的输入 + 输出就是下一次请求的基础（只差新追加的内容），分母是整理阈值
	st := usage.Total(d.records)
	m.publish(d.save.ID, event.TurnUsage, event.TurnUsageData{
		TurnID: d.id, Usage: d.usage, Costs: st.Costs, Unpriced: st.Unpriced,
		Context: event.ContextUsage{Used: d.contextTokens(m), Threshold: d.threshold},
	})
}

// contextTokens 返回本回合当前的上下文规模：最近一次调用的输入 + 输出；没有用量数据时按请求估算。
func (d *draft) contextTokens(m *Manager) int {
	if d.lastInput > 0 {
		return d.lastInput + d.lastOutput
	}
	return m.estimate(d.model, d.req)
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
			code, msg, recoverable = describeError(d.err)
		}
		slog.InfoContext(ctx, "回合未产生内容，不提交", "stop", d.stop, "err", d.errMsg)
		d.release()
		m.publish(d.save.ID, event.TurnFailed, event.TurnFailedData{TurnID: d.id, Code: code, Message: msg, Recoverable: recoverable})
		return
	}
	if d.choices != nil {
		d.display = append(d.display, *d.choices)
	}

	// 状态、记忆有变化时写入新快照与新清单，否则沿用父节点的哈希。
	var newState map[string]any
	if d.stateChanged {
		newState = d.state
	}
	memory := memoryWrite(d.memory)
	contextTokens := d.contextTokens(m)
	node, err := m.d.Saves.CommitTurn(ctx, save.TurnCommit{
		SaveID: d.save.ID, Parent: d.parent, TurnNo: d.turnNo, Input: d.input, Messages: d.turnMsgs, Display: d.display,
		StateHash: d.parent.StateHash, State: newState, MemoryHash: d.parent.MemoryHash, Memory: memory, WorkVersionID: d.parent.WorkVersionID,
		Status: d.status, Usage: &d.usage, PlainText: save.TranscriptBody(d.display, d.work.SpeakerName), UsageRecords: d.records, NodeID: d.id,
		Meta: save.NodeMeta{
			Model: d.model.Name, Stop: d.stop, StopRaw: d.stopRaw, Calls: d.calls, ToolCalls: d.toolCalls,
			Continuations: d.continuations, DurationMS: time.Since(d.started).Milliseconds(), Error: d.errMsg,
			PrefixHash: d.built.PrefixHash, PromptVersion: prompts.Version, ContextTokens: contextTokens,
		},
	})
	// 上下文达到整理阈值：写者槽位直接交给整理任务（玩家阅读期间整理，期间的输入排队）。
	// 上次整理之后至少要有 3 个回合（整理至少保留 2 个回合，才有可以摘要掉的内容）：预算小于常驻部分加保留原文时，
	// 不会每个回合都整理一次；出错结束的回合不触发（模型服务多半同样不可用）。
	compacting := err == nil && d.stop != "error" && contextTokens >= d.threshold && d.sinceCompaction >= 3
	if compacting {
		slog.InfoContext(ctx, "上下文达到整理阈值", "context_tokens", contextTokens, "threshold", d.threshold)
		m.handoffCompaction(d.save.ID, d.slot)
	}
	d.release() // 已提交（或提交失败）：先让出写者槽位，再发布终态事件
	if err != nil {
		slog.ErrorContext(ctx, "回合提交失败", "err", err)
		m.publish(d.save.ID, event.TurnFailed, event.TurnFailedData{TurnID: d.id, Code: string(errs.CodeInternal), Message: "保存回合失败（详见日志）", Recoverable: true})
		return
	}
	m.publish(d.save.ID, event.TurnCommitted, event.TurnCommittedData{TurnID: d.id, NodeID: node.ID, Status: string(node.Status)})
	if d.stop == "error" {
		code, msg, recoverable := describeError(d.err)
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
	text, err = m.d.Tmpl.RenderWith("commands/"+name+".md", cmd.Template,
		map[string]any{"args": argVars, "input": input, "state": st, "turn": panel.TurnVar(turn)}, w.Partials)
	if err != nil {
		return "", "", errs.Wrap(err, errs.CodeWorkInvalid, "命令 /%s 的模板渲染失败", name)
	}
	return strings.TrimSpace(text), modeLabel, nil
}

// memoryWrite 把记忆覆盖层的改动转为提交内容；没有改动时返回 nil（沿用父节点的清单）。
func memoryWrite(o *vfs.Overlay) *save.MemoryWrite {
	if o == nil || !o.Dirty() {
		return nil
	}
	files, data := o.Result()
	w := &save.MemoryWrite{Files: make(map[string]save.MemoryFile, len(files)), Data: data}
	for p, f := range files {
		w.Files[p] = save.MemoryFile{Blob: f.Blob, Size: f.Size, Turn: f.Turn}
	}
	return w
}

// lastTurn 返回进度线上最后一个回合节点（没有回合时返回 nil）。
func lastTurn(line []*save.Node) *save.Node {
	for i := len(line) - 1; i >= 0; i-- {
		if line[i].Kind == save.KindTurn {
			return line[i]
		}
	}
	return nil
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
