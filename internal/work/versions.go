package work

import (
	"context"
	"database/sql"
	"errors"
	"io"

	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

func (s *Service) Versions(ctx context.Context, workID string) ([]VersionInfo, error) {
	if _, err := s.LatestVersionID(ctx, workID); err != nil {
		return nil, err
	}
	versions, err := listVersions(ctx, s.db.Read(), workID)
	if err != nil {
		return nil, err
	}
	out := make([]VersionInfo, 0, len(versions))
	for _, v := range versions {
		out = append(out, versionInfo(v))
	}
	return out, nil
}

// FileSet 读取一个版本的完整文件集，用于导出与快照。
func (s *Service) FileSet(ctx context.Context, workID, versionID string) (*FileSet, error) {
	set := NewFileSet()
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		v, err := getVersion(ctx, tx, versionID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && v.WorkID != workID) {
			return errs.NotFound("作品版本不存在")
		}
		if err != nil {
			return err
		}
		files, err := versionFiles(ctx, tx, versionID)
		if err != nil {
			return err
		}
		for _, f := range files {
			b, err := s.blobs.Get(ctx, tx, f.BlobHash)
			if err != nil {
				return err
			}
			if err := set.Add(f.Path, b); err != nil {
				return err
			}
		}
		return nil
	})
	return set, err
}

func (s *Service) Export(ctx context.Context, workID, versionID string, w io.Writer) error {
	set, err := s.FileSet(ctx, workID, versionID)
	if err != nil {
		return err
	}
	return WriteZip(w, set)
}

// Delete 仅删除没有存档引用的作品或版本；历史分支的引用也受保护。
func (s *Service) Delete(ctx context.Context, workID, versionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		w, err := getWork(ctx, tx, workID)
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NotFound("作品不存在")
		}
		if err != nil {
			return err
		}
		var refs int
		if versionID == "" {
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM saves WHERE work_id = ?`, workID).Scan(&refs); err != nil {
				return err
			}
		} else {
			v, err := getVersion(ctx, tx, versionID)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && v.WorkID != workID) {
				return errs.NotFound("作品版本不存在")
			}
			if err != nil {
				return err
			}
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM saves s WHERE s.work_version_id = ? OR EXISTS (SELECT 1 FROM nodes n WHERE n.save_id = s.id AND n.work_version_id = ?)`, versionID, versionID).Scan(&refs); err != nil {
				return err
			}
		}
		if refs > 0 {
			return errs.Conflict("有 %d 个存档或其历史分支引用此作品版本，请先处理这些存档", refs)
		}
		// 删除标记以作品 ID 为单位；手动导入不受影响，启动时不再自动补回。
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, 'true') ON CONFLICT(key) DO NOTHING`, "deleted_builtin:"+workID); err != nil {
			return err
		}
		if versionID == "" {
			_, err := tx.ExecContext(ctx, `DELETE FROM works WHERE id = ?`, workID)
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM work_versions WHERE id = ?`, versionID); err != nil {
			return err
		}
		versions, err := listVersions(ctx, tx, workID)
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			_, err := tx.ExecContext(ctx, `DELETE FROM works WHERE id = ?`, workID)
			return err
		}
		hasDev := false
		for _, v := range versions {
			hasDev = hasDev || v.IsDev
		}
		if !hasDev {
			if _, err := tx.ExecContext(ctx, `UPDATE works SET dev_path = NULL WHERE id = ?`, workID); err != nil {
				return err
			}
		}
		if w.LatestVersionID == versionID {
			return upsertWork(ctx, tx, workID, versions[0].Manifest.Title, versions[0].ID)
		}
		return nil
	})
	if err == nil {
		clear(s.cache)
	}
	return err
}

// setLatest 同内容导入仍更新作品选中的版本和来源，保持显式导入的语义。
func setLatest(ctx context.Context, tx *sql.Tx, workID, versionID string, src Source) error {
	v, err := getVersion(ctx, tx, versionID)
	if err != nil {
		return err
	}
	if err := upsertWork(ctx, tx, workID, v.Manifest.Title, versionID); err != nil {
		return err
	}
	encoded, err := jsonx.Marshal(src)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE work_versions SET source = ? WHERE id = ?`, string(encoded), versionID)
	return err
}
