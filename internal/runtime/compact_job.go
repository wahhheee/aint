package runtime

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/wahhheee/yanyi/internal/compact"
	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/ids"
	"github.com/wahhheee/yanyi/internal/pkg/logx"
	"github.com/wahhheee/yanyi/internal/vfs"
)

// 后台整理任务：回合提交后上下文达到阈值时（玩家阅读期间），以及玩家手动整理（按钮或 /compact）。
// 整理任务占用存档的写者槽位；期间提交的输入排队，整理结束后开始。

// Compact 手动整理存档的上下文（后台进行，结果经 compaction.* 事件推送）。有任务在运行时返回 Conflict。
func (m *Manager) Compact(ctx context.Context, saveID string) error {
	if _, err := m.d.Saves.Get(ctx, saveID); err != nil {
		return err
	}
	s := m.lockSession(saveID)
	defer s.mu.Unlock()
	if err := busyError(s); err != nil {
		return err
	}
	m.startCompactionLocked(saveID, s, compact.ReasonManual)
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
	m.startCompactionLocked(saveID, s, compact.ReasonThreshold)
}

// startCompactionLocked 在写者槽位上开始一个整理任务（调用方持有 s.mu，且槽位空闲或属于即将让出的回合）。
func (m *Manager) startCompactionLocked(saveID string, s *session, reason compact.Reason) {
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
		m.runCompactionJob(ctx, saveID, reason, release)
	})
}

// runCompactionJob 对 head 所在的进度线执行整理。先让出槽位（排队的回合随即开始），再发布结果。
func (m *Manager) runCompactionJob(ctx context.Context, saveID string, reason compact.Reason, release func()) {
	sv, line, err := m.d.Saves.ProgressLine(ctx, saveID)
	if err != nil {
		release()
		slog.ErrorContext(ctx, "整理前读取进度线失败", "err", err)
		m.publish(saveID, event.CompactionFailed, event.CompactionFailedData{Message: "整理上下文时出错（详见日志）"})
		return
	}
	node, stats, err := m.compactLine(ctx, sv, line, reason)
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
		m.publish(saveID, event.CompactionFailed, event.CompactionFailedData{Message: msg})
		return
	}
	m.publish(saveID, event.CompactionDone, event.CompactionDoneData{NodeID: node.ID, Stats: stats})
	m.publish(saveID, event.TimelineChanged, event.TimelineChangedData{HeadID: node.ID, Reason: "compaction"})
}

// compactionError 把整理失败写成给玩家的一句话。
func compactionError(err error) string {
	if e, ok := errs.As(err); ok {
		return e.Message
	}
	_, msg, _ := describeError(err)
	return "整理上下文没有成功：" + msg
}

// compactInTurn 在回合内整理（13.2 第 2、3 点）：对本回合之前的历史执行整理，整理节点成为本回合的父节点，
// 再以新阶段块 + 保留的回合 + 本回合已有的消息重建请求；本回合的消息全部保留，记忆写入沿用。
func (m *Manager) compactInTurn(ctx context.Context, d *draft, reason compact.Reason) error {
	line, err := m.d.Saves.ProgressLineAt(ctx, d.save.ID, d.parent.ID)
	if err != nil {
		return err
	}
	m.publish(d.save.ID, event.CompactionStarted, event.CompactionStartedData{Reason: string(reason)})
	node, stats, err := m.compactLine(ctx, d.save, line, reason)
	if err != nil {
		m.publish(d.save.ID, event.CompactionFailed, event.CompactionFailedData{Message: compactionError(err)})
		return err
	}
	m.publish(d.save.ID, event.CompactionDone, event.CompactionDoneData{NodeID: node.ID, Stats: stats})
	d.compacted, d.parent = true, node

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
		Tools: m.d.Tools.Specs(d.features, d.work), History: ec.history, Turn: d.turnMsgs[0],
	})
	if err != nil {
		return err
	}
	msgs := built.Request.Messages // 以本回合的玩家消息结尾，再接本回合已有的其余消息
	msgs = append(msgs, d.turnMsgs[1:]...)
	d.req.Messages = msgs
	built.Request = d.req
	d.built = built
	d.lastInput = 0 // 上下文已经换了，之前的用量不再代表下一次请求
	return nil
}
