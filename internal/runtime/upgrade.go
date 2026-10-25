package runtime

import (
	"context"

	"github.com/wahhheee/yanyi/internal/compact"
	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/work"
)

// Upgrade 在单一写者槽位中用旧作品整理，再原子提交新版本、迁移后的状态与阶段块。
func (m *Manager) Upgrade(ctx context.Context, saveID, versionID string) error {
	s := m.lockSession(saveID)
	defer func() { s.mu.Unlock(); m.dropIdle(saveID, s) }()
	if err := busyError(s); err != nil {
		return err
	}
	sv, err := m.d.Saves.Get(ctx, saveID)
	if err != nil {
		return err
	}
	target, err := m.d.Works.Model(ctx, versionID)
	if err != nil {
		return err
	}
	if target.WorkID != sv.WorkID {
		return errs.Invalid("目标版本不属于此作品")
	}
	if target.IsDev {
		return errs.Invalid("开发目录自动重载，请选择已导入的固定版本")
	}
	if target.VersionID == sv.WorkVersionID {
		return errs.Invalid("存档已经使用此版本")
	}
	m.startCompactionLocked(saveID, s, compact.ReasonUpgrade, target)
	return nil
}

// upgradeEmpty 没有故事回合时只迁移初始状态，不发起模型调用。
func (m *Manager) upgradeEmpty(ctx context.Context, sv *save.Save, line []*save.Node, target *work.Model) (*save.Node, *CompactStats, error) {
	head := line[len(line)-1]
	old, err := m.d.Saves.LoadState(ctx, head.StateHash)
	if err != nil {
		return nil, nil, err
	}
	st, records := state.Migrate(target.State, old)
	index, err := m.d.Saves.MemoryIndex(ctx, head.MemoryHash)
	if err != nil {
		return nil, nil, err
	}
	features := contextx.Profile(target, sv.Settings, llm.Caps{}, false)
	n, summary := 1, ""
	for _, node := range line {
		if node.Kind == save.KindCompaction {
			n++
			b, err := m.d.Saves.Blob(ctx, node.EpochHash)
			if err != nil {
				return nil, nil, err
			}
			summary = between(string(b), "<summary>\n", "\n</summary>")
		}
	}
	epoch := contextx.RenderEpoch(features, target.State, contextx.EpochInput{N: n, FromTurn: head.TurnNo + 1, Summary: summary, State: st, Memory: index})
	notes := []string{"作品已更新到 " + target.Manifest.Version}
	if target.Manifest.Changelog != "" {
		notes = append(notes, target.Manifest.Changelog)
	}
	node, err := m.d.Saves.CommitCompaction(ctx, save.CompactionCommit{
		SaveID: sv.ID, Parent: head, Epoch: epoch, State: st, WorkVersionID: target.VersionID,
		Notes: notes,
		Meta:  save.NodeMeta{CompactReason: string(compact.ReasonUpgrade), Migration: records, WorkHash: target.TreeHash},
	})
	return node, &CompactStats{Reason: string(compact.ReasonUpgrade)}, err
}
