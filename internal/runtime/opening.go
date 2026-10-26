package runtime

import "context"

// SwitchOpening 与回合、整理、分支编辑共用写者槽位；成功后所有阅读页刷新时间线与面板。
func (m *Manager) SwitchOpening(ctx context.Context, saveID, openingID, expectedHead string) error {
	return m.Exclusive(ctx, saveID, "opening", func() error {
		return m.d.Saves.SwitchOpening(ctx, saveID, openingID, expectedHead)
	})
}
