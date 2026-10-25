package runtime

import (
	"bytes"
	"context"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// PatchSettings 在回合与整理都空闲时修改存档参数，下一回合使用新设置。
func (m *Manager) PatchSettings(ctx context.Context, id string, patch []byte) error {
	return m.Exclusive(ctx, id, "settings", func() error {
		sv, err := m.d.Saves.Get(ctx, id)
		if err != nil {
			return err
		}
		if err := jsonx.UnmarshalStrict(bytes.NewReader(patch), &sv.Settings); err != nil {
			return errs.Invalid("存档设置格式不正确：%v", err)
		}
		if _, _, err := m.d.Models.Resolve(ctx, sv.Settings.NarratorModel); err != nil {
			return err
		}
		if sv.Settings.Helpers {
			if _, _, ok := m.lightModel(ctx); !ok {
				return errs.Invalid("后台助手需要先配置轻量模型")
			}
		}
		return m.d.Saves.UpdateSettings(ctx, id, sv.Settings)
	})
}
