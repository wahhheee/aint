package save

import (
	"context"
	"database/sql"

	"github.com/wahhheee/yanyi/internal/usage"
)

// TitleCurrent 表示当前进度已有一次成功的标题调用。
func (s *Service) TitleCurrent(ctx context.Context, id, head string) (bool, error) {
	return titleCurrent(ctx, s.db.Read(), id, head)
}

// CommitTitle 记录用量，并且只在存档名称与进度仍匹配请求时保存标题。
func (s *Service) CommitTitle(ctx context.Context, original *Save, title string, rec usage.Record) (bool, error) {
	changed := false
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		if err := usage.Insert(ctx, tx, rec); err != nil {
			return err
		}
		if title == "" {
			return nil
		}
		var err error
		changed, err = setGeneratedTitle(ctx, tx, original, title)
		return err
	})
	return changed, err
}
