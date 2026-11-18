package runtime

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sync"

	"github.com/wahhheee/aint/internal/compact"
	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/pkg/logx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/state"
	"github.com/wahhheee/aint/internal/vfs"
	"github.com/wahhheee/aint/internal/work"
)

// 后台整理任务：回合提交后上下文达到阈值时（玩家阅读期间），以及玩家手动整理（按钮或 /compact）。
// 整理任务占用存档的写者槽位；期间提交的输入排队，整理结束后开始。

// Compact 手动整理存档的上下文（后台进行，结果经 compaction.* 事件推送）。有任务在运行时返回 Conflict。
func (m *Manager) Compact(ctx context.Context, saveID string) error {
	return m.CompactAt(ctx, saveID, "", false)
}

// CompactAt 限定手动整理的当前进度。重新整理从指定整理节点的父节点创建兄弟版本，不提前移动 head。
func (m *Manager) CompactAt(ctx context.Context, saveID, expectedHead string, redo bool) error {
	s := m.lockSession(saveID)
	defer func() { s.mu.Unlock(); m.dropIdle(saveID, s) }()
	if err := busyError(s); err != nil {
		return err
	}
	sv, err := m.d.Saves.Get(ctx, saveID)
	if err != nil {
		return err
	}
	if expectedHead != "" && expectedHead != sv.HeadNodeID {
		return errs.Conflict("存档进度已改变，未开始整理。")
	}
	if redo {
		if expectedHead == "" {
			return errs.Invalid("重新整理需要指定原整理节点。")
		}
		n, err := m.d.Saves.Node(ctx, saveID, expectedHead)
		if err != nil {
			return err
		}
		if !canRedoCompaction(n) {
			return errs.Invalid("只能重新整理手动或回合后自动产生的整理节点。")
		}
	}
	m.startCompactionLocked(saveID, s, compact.ReasonManual, nil, redo)
	return nil
}

// handoffCompaction 把回合的写者槽位直接交给整理任务（回合提交后、让出槽位前调用），中间不会有别的写者插进来。
func (m *Manager) handoffCompaction(saveID string, slot *turnSlot) {
	s := slot.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != slot.job {
		return
	}
	m.startCompactionLocked(saveID, s, compact.ReasonThreshold, nil, false)
}

// startCompactionLocked 在写者槽位上开始一个整理任务（调用方持有 s.mu，且槽位空闲或属于即将让出的回合）。
func (m *Manager) startCompactionLocked(saveID string, s *session, reason compact.Reason, target *work.Model, redo bool) {
	ctx, cancel := context.WithTimeout(m.base, m.d.Budget.WallClock)
	j := newJob(ids.New(), jobCompaction, cancel)
	s.running = j
	m.publish(saveID, event.CompactionStarted, event.CompactionStartedData{Reason: string(reason)})
	release := sync.OnceFunc(func() { m.release(saveID, s, j) })
	ctx = logx.With(ctx, "save_id", saveID, "compaction_id", j.id)
	m.wg.Go(func() {
		defer cancel()
		defer release()
		defer func() {
			if p := recover(); p != nil {
				slog.ErrorContext(ctx, "整理 panic", "panic", p)
				release()
				m.publish(saveID, event.CompactionFailed, event.CompactionFailedData{Message: "整理上下文时出错（详见日志）"})
			}
		}()
		m.runCompactionJob(ctx, saveID, reason, target, redo, j.id, release)
	})
}

// runCompactionJob 对 head 所在的进度线执行整理。先让出槽位（排队的回合随即开始），再发布结果。
func (m *Manager) runCompactionJob(ctx context.Context, saveID string, reason compact.Reason, target *work.Model, redo bool, attemptID string, release func()) {
	sv, line, err := m.d.Saves.ProgressLine(ctx, saveID)
	if err != nil {
		release()
		slog.ErrorContext(ctx, "整理前读取进度线失败", "err", err)
		m.publish(saveID, event.CompactionFailed, event.CompactionFailedData{Message: "整理上下文时出错（详见日志）"})
		return
	}
	if redo {
		line = line[:len(line)-1]
	}
	node, stats, err := m.compactWith(ctx, attemptID, sv, line, reason, target, nil, nil, m.d.Saves.CommitCompaction)
	if err == nil {
		m.publishPanels(ctx, saveID)
	}
	release()
	if err != nil {
		msg := compactionError(err)
		if errors.Is(err, errNothingToCompact) && reason == compact.ReasonThreshold {
			// 自动触发、只是没有可以摘要掉的回合：不打扰玩家（消息为空，界面只收起"正在整理"）
			slog.InfoContext(ctx, "上下文达到阈值，但没有可以摘要掉的回合", "reason", reason)
			msg = ""
		} else {
			slog.WarnContext(ctx, "上下文整理失败", "reason", reason, "err", err)
		}
		failure := event.CompactionFailedData{AttemptID: attemptID, Reason: string(reason), Message: msg, Redo: redo}
		if target == nil && !errors.Is(err, errNothingToCompact) {
			failure.RetryHead = sv.HeadNodeID
		}
		m.publish(saveID, event.CompactionFailed, failure)
		return
	}
	m.publish(saveID, event.CompactionDone, event.CompactionDoneData{NodeID: node.ID, Stats: stats})
	m.publish(saveID, event.TimelineChanged, event.TimelineChangedData{HeadID: node.ID, Reason: "compaction"})
}

func canRedoCompaction(n *save.Node) bool {
	return n.Kind == save.KindCompaction && (n.Meta.CompactReason == string(compact.ReasonThreshold) || n.Meta.CompactReason == string(compact.ReasonManual))
}

// compactionError 把整理失败写成给玩家的一句话。
func compactionError(err error) string {
	_, msg, _ := describeError(err)
	return msg
}

// compactInTurn 在回合内整理（13.2 第 2、3 点）：对本回合之前的历史执行整理，整理节点成为本回合的父节点，
// 再以新阶段块 + 保留的回合 + 本回合已有的消息重建请求；本回合的状态与记忆写入保留在整理后的快照上。
func (m *Manager) compactInTurn(ctx context.Context, d *draft, reason compact.Reason) error {
	d.compactionAttempted = true
	line, err := m.d.Saves.ProgressLineAt(ctx, d.save.ID, d.parent.ID)
	if err != nil {
		m.publish(d.save.ID, event.CompactionFailed, event.CompactionFailedData{Message: compactionError(err)})
		return err
	}
	attemptID := ids.New()
	m.publish(d.save.ID, event.CompactionStarted, event.CompactionStartedData{Reason: string(reason)})
	var merged map[string]any
	commit := func(ctx context.Context, c save.CompactionCommit) (*save.Node, error) {
		var mergeErr error
		merged, mergeErr = rebaseTurnState(d.work.State, c.State, d.stateLog)
		if mergeErr != nil {
			return nil, mergeErr
		}
		if d.operation != nil {
			p, err := m.d.Saves.StageCompaction(ctx, c)
			if err != nil {
				return nil, err
			}
			d.staged = p
			return p.Node, nil
		}
		return m.d.Saves.CommitCompaction(ctx, c)
	}
	node, stats, err := m.compactWith(ctx, attemptID, d.save, line, reason, nil, d.work, d.stateLog, commit)
	if err != nil {
		msg := compactionError(err)
		if errors.Is(err, errNothingToCompact) {
			msg = ""
		}
		m.publish(d.save.ID, event.CompactionFailed, event.CompactionFailedData{AttemptID: attemptID, Reason: string(reason), Message: msg})
		return err
	}
	result := event.CompactionDoneData{NodeID: node.ID, Stats: stats}
	if d.staged != nil {
		d.stagedResult = &result
		d.stagedAttempt = attemptID
	} else {
		m.publish(d.save.ID, event.CompactionDone, result)
	}
	d.parent = node
	var changed []state.Change
	for _, name := range d.work.State.Order {
		if !reflect.DeepEqual(d.state[name], merged[name]) {
			changed = append(changed, state.Change{Op: state.OpSet, Path: name, Before: d.state[name], After: merged[name]})
		}
	}
	d.state = merged
	if len(changed) > 0 && d.onState != nil {
		d.onState(changed)
	}
	// 整理节点已保存基准；本回合只有实际写过状态时才需要另存快照。
	d.stateChanged = len(d.stateLog) > 0

	mf, err := m.d.Saves.LoadMemory(ctx, node.MemoryHash)
	if err != nil {
		return err
	}
	base := make(map[string]vfs.FileRef, len(mf.Files))
	for p, f := range mf.Files {
		base[p] = vfs.FileRef{Blob: f.Blob, Size: f.Size, Turn: f.Turn}
	}
	d.memory.Rebase(base)

	ec, err := m.loadContext(ctx, d.work, d.features, append(line, node))
	if err != nil {
		return err
	}
	built, err := m.d.Builder.Build(ctx, contextx.BuildInput{
		SaveID: d.save.ID, Work: d.work, Features: d.features, Model: d.model, Settings: d.save.Settings,
		InitialState: d.initState, InitialStateHash: d.root.StateHash, Epoch: ec.block,
		Tools: m.d.Tools.Specs(d.features, d.work), History: ec.history, Turn: d.operationMessage(d.turnMsgs[0]),
	})
	if err != nil {
		return err
	}
	msgs := built.Request.Messages // 以本回合的玩家消息结尾，再接本回合已有的其余消息
	d.contextMessages = len(msgs) - 1
	msgs = append(msgs, d.turnMsgs[1:]...)
	notice := llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.CompactionResume)}}
	msgs = append(msgs, notice)
	d.turnMsgs = append(d.turnMsgs, notice)
	d.req.Messages = msgs
	built.Request = d.req
	d.built = built
	d.lastInput = 0 // 上下文已经换了，之前的用量不再代表下一次请求
	return nil
}

// rebaseTurnState 在整理后的状态上恢复本回合已经完成的写入结果，不重复执行增减或追加。
func rebaseTurnState(schema *state.Schema, base map[string]any, changes []state.Change) (map[string]any, error) {
	if len(changes) == 0 {
		return base, nil
	}
	ops := make([]state.Op, 0, len(changes))
	for _, change := range changes {
		op := state.Op{Op: state.OpSet, Path: change.Path, Value: change.After}
		if change.After == nil {
			op.Op = state.OpDelete
		}
		ops = append(ops, op)
	}
	next, _, err := state.Apply(schema, base, ops)
	return next, err
}
