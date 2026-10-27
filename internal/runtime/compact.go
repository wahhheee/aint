package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/wahhheee/aint/internal/compact"
	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/state"
	"github.com/wahhheee/aint/internal/tools"
	"github.com/wahhheee/aint/internal/usage"
	"github.com/wahhheee/aint/internal/vfs"
	"github.com/wahhheee/aint/internal/work"
)

// 上下文整理的执行（《02-技术方案》第 13 节；参数与规则见 compact 包）。
//
// inline：在当前上下文（命中缓存）末尾接上整理指令，由主笔先把要紧的事写进 /memory，再输出交接摘要；
// separate：用轻量模型单独整理，输入是上一份摘要、本阶段原文、当前状态与记忆目录，不复用主笔缓存。
// 模型报上下文超限时，原上下文已经放不下，改用 separate 的输入方式（没有轻量模型时用主笔模型）。
//
// 整理产生 compaction 节点（阶段块 + keep_from + 落盘后的记忆清单），成为 head；之后的请求以新阶段块开头，
// 后接保留的回合原文（loadContext）。整理过程中的消息存在节点上供排查，不进入之后的上下文。

// errNothingToCompact 表示上下文中的回合都要保留，没有可以摘要掉的内容。
var errNothingToCompact = errs.Invalid("内容还不多，暂时不需要整理")

// CompactStats 是一次整理的结果（compaction.done 事件、日志）。
type CompactStats struct {
	Reason       string `json:"reason"`
	Mode         string `json:"mode"`
	SummaryFrom  int    `json:"summary_from"`   // 摘要掉的回合区间
	SummaryTo    int    `json:"summary_to"`     //
	KeepFromTurn int    `json:"keep_from_turn"` // 保留原文的起点回合
	SummaryChars int    `json:"summary_chars"`
	MemoryWrites int    `json:"memory_writes"` // 落盘时改动的记忆文件数
	Before       int    `json:"before"`        // 整理前的上下文（token）
	After        int    `json:"after"`         // 整理后的上下文估算（token）
	Calls        int    `json:"calls"`
	DurationMS   int64  `json:"duration_ms"`
}

// compactAllowed 是整理阶段可用的工具；其余工具（选项、状态、掷骰、助手）返回"整理阶段暂不使用这个工具"，
// 工具定义不变（前缀稳定）。
var compactAllowed = map[string]bool{"read": true, "list": true, "search": true, "write": true, "edit": true, "remove": true, "get_state": true}

// compactVersion 对进度线执行整理；current 用于固定开发回合正在使用的作品快照，target 指定升级目标。
func (m *Manager) compactVersion(ctx context.Context, sv *save.Save, line []*save.Node, reason compact.Reason, target, current *work.Model) (*save.Node, *CompactStats, error) {
	return m.compactWith(ctx, sv, line, reason, target, current, m.d.Saves.CommitCompaction)
}

// compactWith 通过提交函数选择立即采用或暂存整理结果。
func (m *Manager) compactWith(ctx context.Context, sv *save.Save, line []*save.Node, reason compact.Reason, target, current *work.Model, commit func(context.Context, save.CompactionCommit) (*save.Node, error)) (*save.Node, *CompactStats, error) {
	start := time.Now()
	head := line[len(line)-1]
	if target != nil && lastTurn(line) == nil {
		return m.upgradeEmpty(ctx, sv, line, target)
	}
	w := current
	if w == nil {
		var err error
		w, err = m.d.Works.Model(ctx, head.WorkVersionID)
		if err != nil {
			return nil, nil, err
		}
	}
	narratorProvider, narrator, err := m.d.Models.Resolve(ctx, sv.Settings.NarratorModel)
	if err != nil {
		return nil, nil, err
	}
	features := contextx.Profile(w, sv.Settings, narrator.Caps, false)
	params := m.compactParams(ctx, sv.Settings)
	threshold := params.Threshold(narrator.ContextWindow, maxOutput(narrator))

	ec, err := m.loadContext(ctx, w, features, line)
	if err != nil {
		return nil, nil, err
	}
	if target != nil && len(ec.turns) == 0 {
		return m.upgradeEmpty(ctx, sv, line, target)
	}
	idx := compact.KeepFrom(ec.sizes, params.KeepTokens(threshold))
	if target != nil && len(ec.turns) > 0 {
		idx = max(idx, 1)
	}
	if idx == 0 {
		return nil, nil, errNothingToCompact
	}

	// 整理方式与模型
	mode := params.Mode
	provider, model := narratorProvider, narrator
	if reason == compact.ReasonOverflow {
		mode = compact.ModeSeparate
	}
	if mode == compact.ModeSeparate {
		if lp, lm, ok := m.lightModel(ctx); ok {
			provider, model = lp, lm
		} else if reason != compact.ReasonOverflow {
			slog.WarnContext(ctx, "没有配置轻量模型，改用主笔在原上下文中整理")
			mode = compact.ModeInline
		}
	}

	// 记忆覆盖层与挂载表（与回合相同；落盘写入记为整理时所在的回合）
	uploads, err := m.uploads(ctx, line, nil, head.TurnNo+1)
	if err != nil {
		return nil, nil, err
	}
	fsys, overlay, err := m.buildVFS(ctx, w, features, sv.ID, line, head.TurnNo, uploads.all)
	if err != nil {
		return nil, nil, err
	}
	headState, err := m.d.Saves.LoadState(ctx, head.StateHash)
	if err != nil {
		return nil, nil, err
	}
	var migration []state.MigrationValue
	if w.IsDev {
		headState, migration = state.Migrate(w.State, headState)
	}
	task, err := contextx.RenderCompactionTask(features, params.SummaryChars(threshold), overlay.TotalSize() > vfs.MemoryWarnTotal, w.CompactionGuide)
	if err != nil {
		return nil, nil, err
	}

	var req *llm.Request
	toolFS := features
	if mode == compact.ModeInline {
		root := line[0]
		initState, err := m.d.Saves.LoadState(ctx, root.StateHash)
		if err != nil {
			return nil, nil, err
		}
		initState, _ = state.Migrate(w.State, initState)
		built, err := m.d.Builder.Build(ctx, contextx.BuildInput{
			SaveID: sv.ID, Work: w, Features: features, Model: narrator, Settings: sv.Settings,
			InitialState: initState, InitialStateHash: root.StateHash, Epoch: ec.block,
			Tools: m.d.Tools.Specs(features, w), History: ec.history,
			Turn: compactionMessage(ec.history, save.PendingNotes(line), task),
		})
		if err != nil {
			return nil, nil, err
		}
		req = built.Request
	} else {
		toolFS = contextx.Profile(w, sv.Settings, model.Caps, false)
		memIndex, err := m.d.Saves.MemoryIndex(ctx, head.MemoryHash)
		if err != nil {
			return nil, nil, err
		}
		if req, err = m.separateRequest(ctx, sv, w, toolFS, model, ec, headState, memIndex, task); err != nil {
			return nil, nil, err
		}
	}
	req.Purpose = "compact"

	env := &compactEnv{features: toolFS, work: w, fs: fsys, state: headState, images: m.d.Images, model: model}
	run := &compactCalls{m: m, provider: provider, model: model, save: sv, req: req, start: len(req.Messages) - 1, debugID: ids.New(), turnNo: head.TurnNo, env: env}
	summary, err := run.loop(ctx, env, params)
	if err != nil {
		return nil, nil, err
	}

	// 阶段块：摘要 + 阶段开始时的状态 + 落盘后的记忆目录
	kept := ec.turns[idx:]
	var keptFrom, keptTo int
	var keepID string
	if len(kept) > 0 {
		keptFrom, keptTo, keepID = kept[0].TurnNo, kept[len(kept)-1].TurnNo, kept[0].ID
	}
	var notes []string
	versionID := ""
	if target != nil {
		w = target
		versionID = w.VersionID
		var records []state.MigrationValue
		headState, records = state.Migrate(w.State, headState)
		migration = append(migration, records...)
		features = contextx.Profile(w, sv.Settings, narrator.Caps, false)
		notes = []string{"作品已更新到 " + w.Manifest.Version}
		if w.Manifest.Changelog != "" {
			notes = append(notes, w.Manifest.Changelog)
		}
	}
	memWrite := memoryWrite(overlay)
	var memIndex []save.MemoryEntry
	if memWrite != nil {
		memIndex, err = m.d.Saves.MemoryIndexOf(ctx, memWrite.Files, memWrite.Data)
	} else {
		memIndex, err = m.d.Saves.MemoryIndex(ctx, head.MemoryHash)
	}
	if err != nil {
		return nil, nil, err
	}
	epoch := contextx.RenderEpoch(features, w.State, contextx.EpochInput{
		N: ec.n + 1, FromTurn: head.TurnNo + 1, KeptFrom: keptFrom, KeptTo: keptTo,
		Summary: summary, State: headState, Memory: memIndex,
	})
	stats := &CompactStats{
		Reason: string(reason), Mode: mode, SummaryFrom: ec.turns[0].TurnNo, SummaryTo: ec.turns[idx-1].TurnNo,
		KeepFromTurn: keptFrom, SummaryChars: len([]rune(summary)), Before: contextTokensOf(line), Calls: run.calls,
	}
	written, removed := overlay.Changes()
	stats.MemoryWrites = len(written) + len(removed)

	// 新阶段的上下文规模：新阶段块 + 保留的回合（去掉推理），记在节点上（之后的阈值判断与用量环以它为准）
	var keptHistory []llm.Message
	for _, msgs := range ec.msgs[idx:] {
		keptHistory = append(keptHistory, stripNative(msgs)...)
	}
	if stats.After, err = m.estimateHistory(ctx, sv, w, features, narrator, line[0], epoch, repairOrphans(keptHistory)); err != nil {
		return nil, nil, err
	}

	node, err := commit(ctx, save.CompactionCommit{
		SaveID: sv.ID, Parent: head, Epoch: epoch, KeepFrom: keepID, Memory: memWrite,
		WorkVersionID: versionID, State: headState, Notes: notes,
		Messages: run.msgs, Usage: &run.usage, UsageRecords: run.records,
		Meta: save.NodeMeta{
			Migration: migration, WorkHash: w.TreeHash,
			Model: model.Name, Stop: "summary", Calls: run.calls, ToolCalls: run.toolCalls, DurationMS: time.Since(start).Milliseconds(),
			PromptVersion: prompts.Version, ContextTokens: stats.After, CompactReason: string(reason),
			SummaryFrom: stats.SummaryFrom, SummaryTo: stats.SummaryTo, KeepFromTurn: stats.KeepFromTurn,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("提交整理节点失败: %w", err)
	}
	stats.DurationMS = time.Since(start).Milliseconds()
	if stats.After >= threshold {
		slog.WarnContext(ctx, "整理后的上下文仍不低于整理阈值：预算可能小于常驻部分加保留原文", "after", stats.After, "threshold", threshold)
	}
	slog.InfoContext(ctx, "上下文整理完成", "reason", reason, "mode", mode, "summary_turns", fmt.Sprintf("%d-%d", stats.SummaryFrom, stats.SummaryTo),
		"keep_from", stats.KeepFromTurn, "summary_chars", stats.SummaryChars, "memory_writes", stats.MemoryWrites,
		"before", stats.Before, "after", stats.After, "calls", stats.Calls, "duration", time.Since(start).Round(time.Millisecond))
	return node, stats, nil
}

// compactionMessage 生成 inline 整理的 user 消息：上一回合以 offer_choices 结束时先补一条工具结果
// "（玩家还没有作答，系统先整理上下文）"，再接注记（如有）与整理指令。前缀与上一次请求一致，仍命中缓存。
func compactionMessage(history []llm.Message, notes []string, task string) llm.Message {
	var parts []llm.Part
	if call := pendingChoiceCall(history); call != "" {
		parts = append(parts, llm.Part{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{
			CallID: call, Parts: []llm.Part{llm.Text(prompts.CompactionPendingChoice)},
		}})
	}
	var b strings.Builder
	if len(notes) > 0 {
		b.WriteString("<notes>\n")
		for _, n := range notes {
			b.WriteString("- " + n + "\n")
		}
		b.WriteString("</notes>\n")
	}
	b.WriteString(task)
	return llm.Message{Role: llm.RoleUser, Parts: append(parts, llm.Text(b.String()))}
}

// separateRequest 组装 separate 模式的请求：整理专用的系统提示、记忆相关工具，user 消息中是上一份摘要、
// 上下文中各回合的原文（放不下时从最早的回合起省略）、当前状态、记忆目录与整理指令。
func (m *Manager) separateRequest(ctx context.Context, sv *save.Save, w *work.Model, fs prompts.FeatureSet, model *llm.Model,
	ec *epochContext, st map[string]any, memory []save.MemoryEntry, task string) (*llm.Request, error) {
	var specs []llm.ToolSpec
	for _, s := range m.d.Tools.Specs(fs, w) {
		if compactAllowed[s.Name] {
			specs = append(specs, s)
		}
	}
	texts, err := m.d.Saves.TranscriptTexts(ctx, sv.ID)
	if err != nil {
		return nil, err
	}
	var head strings.Builder
	fmt.Fprintf(&head, "<work title=%q/>\n", w.Manifest.Title)
	if ec.compaction != nil {
		if prev := between(ec.block, "<summary>\n", "\n</summary>"); prev != "" {
			fmt.Fprintf(&head, "<previous_summary>\n%s\n</previous_summary>\n", prev)
		}
	}
	var tail strings.Builder
	if !w.State.Empty() {
		if y, err := state.YAML(w.State, st); err == nil {
			fmt.Fprintf(&tail, "<state>\n%s</state>\n", y)
		}
	}
	if index := contextx.RenderMemoryIndex(memory); index != "" {
		tail.WriteString(index + "\n")
	}
	tail.WriteString("\n" + task)

	// 原文：从最近的回合往前放，直到预算用完（为输出与工具往返留出余量）
	system := prompts.CompactionSeparateSystem + "\n\n" + contextx.RenderToolGuide(specs)
	budget := model.ContextWindow - maxOutput(model) - contextx.EstimateTokens(system+head.String()+tail.String()) -
		contextx.EstimateTokens(string(jsonxMarshal(specs))) - 16_000
	var turns []string
	omitted := 0
	for i := len(ec.turns) - 1; i >= 0; i-- {
		n := ec.turns[i]
		t := save.TranscriptFile(n.TurnNo, texts[n.ID])
		cost := contextx.EstimateTokens(t)
		if cost > budget {
			omitted = i + 1
			break
		}
		budget -= cost
		turns = append(turns, t)
	}
	slices.Reverse(turns)
	var body strings.Builder
	body.WriteString(head.String())
	body.WriteString("<transcript>\n")
	if omitted > 0 {
		fmt.Fprintf(&body, "（更早的 %d 个回合从略，需要时可以用 read /transcript/NNNN 查看）\n\n", omitted)
	}
	body.WriteString(strings.Join(turns, "\n"))
	body.WriteString("</transcript>\n")
	body.WriteString(tail.String())
	return &llm.Request{
		System:    []llm.SystemBlock{{Text: system}},
		Tools:     specs,
		Messages:  []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(body.String())}}},
		MaxOutput: maxOutput(model),
		Reasoning: llm.ReasoningLow,
		CacheKey:  "aint:compact:" + sv.ID,
		CacheTTL:  sv.Settings.CacheTTL,
	}, nil
}

// between 取 s 中 open 与 close 之间的文字（没有时返回空）。
func between(s, open, closing string) string {
	_, rest, ok := strings.Cut(s, open)
	if !ok {
		return ""
	}
	body, _, ok := strings.Cut(rest, closing)
	if !ok {
		return ""
	}
	return strings.TrimSpace(body)
}

// compactCalls 是整理过程中的模型调用：落盘循环、交接摘要、必要时的追问。
type compactCalls struct {
	debugID    string
	turnNo     int
	env        *compactEnv
	inspection *debugRecorder
	m          *Manager
	provider   llm.Provider
	model      *llm.Model
	save       *save.Save
	req        *llm.Request
	start      int // req.Messages 中整理指令消息的下标：此后的消息是整理过程（存在节点上供排查）

	msgs                   []llm.Message
	usage                  llm.Usage
	records                []usage.Record
	calls, toolCalls       int
	nudged, shortened, emp bool
}

// loop 运行整理：模型调用工具时执行（只允许读写记忆与查阅资料）并继续；给出不带工具调用的文字时视为交接摘要，
// 校验非空与长度。落盘调用次数用完时追问一次摘要；摘要超过硬上限时要求压缩一次。
func (c *compactCalls) loop(ctx context.Context, env *compactEnv, p compact.Params) (string, error) {
	c.msgs = append(c.msgs, c.req.Messages[c.start])
	var partial strings.Builder // 截断续写时累积的摘要
	for {
		if c.calls >= p.FlushMaxCalls+3 {
			return "", errors.New("整理没有完成：模型没有给出交接摘要")
		}
		if c.calls == p.FlushMaxCalls && !c.nudged {
			c.nudged = true
			c.append(llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.CompactionSummaryNudge)}})
		}
		msg, stop, err := c.call(ctx)
		if err != nil {
			return "", err
		}
		if len(msg.Parts) > 0 {
			c.append(msg)
		}
		if calls := msg.ToolCalls(); len(calls) > 0 {
			c.toolCalls += len(calls)
			partial.Reset()
			c.append(c.runTools(ctx, env, calls))
			continue
		}
		partial.WriteString(msg.TextOf())
		if stop == llm.StopMaxTokens && partial.Len() > 0 {
			c.append(llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.ContinueNotice)}})
			continue
		}
		summary := strings.TrimSpace(partial.String())
		partial.Reset()
		switch p.SummaryProblem(summary, contextx.EstimateTokens(summary)) {
		case "empty":
			if c.emp {
				return "", errors.New("整理没有完成：模型给出的交接摘要是空的")
			}
			c.emp = true
			c.append(llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.CompactionSummaryEmpty)}})
			continue
		case "long":
			if !c.shortened {
				c.shortened = true
				c.append(llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(fmt.Sprintf(prompts.CompactionSummaryShorter, p.SummaryTarget))}})
				continue
			}
			slog.WarnContext(ctx, "交接摘要仍然偏长，照常使用", "tokens", contextx.EstimateTokens(summary))
		}
		return summary, nil
	}
}

func (c *compactCalls) append(msg llm.Message) {
	c.req.Messages = append(c.req.Messages, msg)
	c.msgs = append(c.msgs, msg)
}

// call 发起一次（非流式消费的）模型调用，返回完整的助手消息。
func (c *compactCalls) call(ctx context.Context) (llm.Message, llm.StopReason, error) {
	c.calls++
	start := time.Now()
	raw := contextx.EstimateRequest(c.req)
	c.inspection = c.m.debugStart(ctx, c.save.ID, c.debugID, c.turnNo, c.provider, c.model, c.req, c.env.fs, c.env.state)
	stream, err := debugStream(ctx, c.provider, c.model, c.req, c.inspection)
	if err != nil {
		return llm.Message{}, "", err
	}
	defer func() { _ = stream.Close() }()
	var done *llm.Done
	var text strings.Builder
	for {
		ev, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return llm.Message{}, "", err
		}
		switch ev := ev.(type) {
		case llm.TextDelta:
			text.WriteString(ev.Text)
		case llm.Done:
			done = &ev
		}
	}
	if done == nil {
		if ctx.Err() != nil {
			return llm.Message{}, "", ctx.Err()
		}
		// 流中途断开：保留已收到的文字，按截断处理
		msg := llm.Message{Role: llm.RoleAssistant}
		if text.Len() > 0 {
			msg.Parts = []llm.Part{llm.Text(text.String())}
		}
		return msg, llm.StopMaxTokens, nil
	}
	u := done.Usage
	c.usage = c.usage.Add(u)
	if in := u.InputTotal(); in > 0 {
		c.m.calibrate(c.model.ID, raw, in)
	}
	rec := usage.Record{CallKind: "compact", ModelID: c.model.ID, Usage: u, LatencyMS: time.Since(start).Milliseconds(), Stop: string(done.Stop)}
	if cost, ok := c.model.Price.CostMicros(u, c.save.Settings.CacheTTL); ok {
		rec.CostMicros, rec.Currency = &cost, c.model.Price.CurrencyOrDefault()
	}
	c.records = append(c.records, rec)
	return done.Message, done.Stop, nil
}

// runTools 执行整理过程中的工具调用：允许的工具照常执行，其余返回"整理阶段暂不使用这个工具"。结果按原顺序。
func (c *compactCalls) runTools(ctx context.Context, env *compactEnv, calls []llm.ToolCall) llm.Message {
	var allowed []llm.ToolCall
	for _, tc := range calls {
		if compactAllowed[tc.Name] {
			allowed = append(allowed, tc)
		}
	}
	results := map[string]llm.ToolResult{}
	out := c.m.d.Tools.Execute(ctx, env, allowed)
	for _, r := range out.Results {
		results[r.CallID] = r
	}
	msg := llm.Message{Role: llm.RoleUser}
	for _, tc := range calls {
		r, ok := results[tc.ID]
		if !ok {
			r = llm.ToolResult{CallID: tc.ID, Parts: []llm.Part{llm.Text(prompts.CompactionToolUnavailable)}, IsError: true}
			out.Records = append(out.Records, tools.CallRecord{ID: tc.ID, Name: tc.Name, Args: tc.Args, Result: prompts.CompactionToolUnavailable, IsError: true})
			out.Results = append(out.Results, r)
		}
		msg.Parts = append(msg.Parts, llm.Part{Kind: llm.PartToolResult, ToolResult: &r})
	}
	c.inspection.toolsDone(ctx, out, env.fs, env.state)
	return msg
}

// compactEnv 是整理过程中的工具环境：可以查阅资料、读写记忆、读取状态，不改状态、不给选项、不出卡片。
type compactEnv struct {
	features prompts.FeatureSet
	work     *work.Model
	fs       *vfs.Table
	state    map[string]any
	images   interface {
		ForModel(ctx context.Context, hash string, maxEdge int) (llm.ImageRef, error)
	}
	model *llm.Model
}

var _ tools.Env = (*compactEnv)(nil)

func (e *compactEnv) Features() prompts.FeatureSet  { return e.features }
func (e *compactEnv) Work() *work.Model             { return e.work }
func (e *compactEnv) OfferChoices(save.DisplayItem) {}
func (e *compactEnv) ShowCard(save.DisplayItem)     {}
func (e *compactEnv) VFS() vfs.FS                   { return e.fs }
func (e *compactEnv) State() map[string]any         { return e.state }
func (e *compactEnv) UpdateState([]state.Op) ([]state.Change, error) {
	return nil, errors.New(prompts.CompactionToolUnavailable)
}
func (e *compactEnv) ModelImage(ctx context.Context, blob string) (llm.ImageRef, error) {
	return e.images.ForModel(ctx, blob, e.model.Caps.ImageMaxEdge)
}

// maxOutput 是单次调用的输出上限（与 contextx.Builder 一致）。
func maxOutput(m *llm.Model) int {
	if m.MaxOutput <= 0 {
		return 32_000
	}
	return min(m.MaxOutput, 32_000)
}
