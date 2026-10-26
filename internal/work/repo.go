package work

import (
	"context"
	"database/sql"
	"errors"

	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/store"
)

// 本文件集中作品相关的 SQL。表结构见 store/migrations。

type versionRow struct {
	ID         string
	WorkID     string
	Version    string
	TreeHash   string
	IsDev      bool
	Source     Source
	Manifest   Manifest
	Report     Report
	ImportedAt int64
}

// findVersionByHash 按文件树哈希查找已导入的版本。开发版本不参与去重（D-37）。
func findVersionByHash(ctx context.Context, q store.Querier, workID, treeHash string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM work_versions WHERE work_id = ? AND tree_hash = ? AND is_dev = 0`,
		workID, treeHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func upsertWork(ctx context.Context, q store.Querier, id, title, latestVersionID string) error {
	now := store.NowMillis()
	_, err := q.ExecContext(ctx, `
		INSERT INTO works (id, title, latest_version_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET title = excluded.title, latest_version_id = excluded.latest_version_id,
		                               updated_at = excluded.updated_at`,
		id, title, latestVersionID, now, now)
	return err
}

func insertVersion(ctx context.Context, q store.Querier, v versionRow) error {
	src, err := jsonx.Marshal(v.Source)
	if err != nil {
		return err
	}
	mf, err := jsonx.Marshal(v.Manifest)
	if err != nil {
		return err
	}
	rep, err := jsonx.Marshal(v.Report)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO work_versions (id, work_id, version, tree_hash, is_dev, source, manifest, report, imported_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.WorkID, v.Version, v.TreeHash, v.IsDev, string(src), string(mf), string(rep), v.ImportedAt)
	return err
}

func insertFile(ctx context.Context, q store.Querier, versionID, path, blobHash string, size int) error {
	_, err := q.ExecContext(ctx, `INSERT INTO work_files (version_id, path, blob_hash, size) VALUES (?, ?, ?, ?)`,
		versionID, path, blobHash, size)
	return err
}

const versionColumns = `v.id, v.work_id, v.version, v.tree_hash, v.is_dev, v.source, v.manifest, COALESCE(v.report, '{}'), v.imported_at`

func scanVersion(sc interface{ Scan(...any) error }) (versionRow, error) {
	var v versionRow
	var src, mf, rep string
	if err := sc.Scan(&v.ID, &v.WorkID, &v.Version, &v.TreeHash, &v.IsDev, &src, &mf, &rep, &v.ImportedAt); err != nil {
		return v, err
	}
	if err := jsonx.Unmarshal([]byte(src), &v.Source); err != nil {
		return v, err
	}
	if err := jsonx.Unmarshal([]byte(mf), &v.Manifest); err != nil {
		return v, err
	}
	if err := jsonx.Unmarshal([]byte(rep), &v.Report); err != nil {
		return v, err
	}
	return v, nil
}

func getVersion(ctx context.Context, q store.Querier, versionID string) (versionRow, error) {
	return scanVersion(q.QueryRowContext(ctx, `SELECT `+versionColumns+` FROM work_versions v WHERE v.id = ?`, versionID))
}

func listVersions(ctx context.Context, q store.Querier, workID string) ([]versionRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+versionColumns+` FROM work_versions v WHERE v.work_id = ? ORDER BY v.imported_at DESC`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []versionRow
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type workRow struct {
	ID              string
	LatestVersionID string
	SaveCount       int
	UpdatedAt       int64
}

func listWorks(ctx context.Context, q store.Querier) ([]workRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT w.id, COALESCE(w.latest_version_id, ''), w.updated_at,
		       (SELECT COUNT(*) FROM saves s WHERE s.work_id = w.id)
		FROM works w ORDER BY w.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []workRow
	for rows.Next() {
		var r workRow
		if err := rows.Scan(&r.ID, &r.LatestVersionID, &r.UpdatedAt, &r.SaveCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func getWork(ctx context.Context, q store.Querier, id string) (workRow, error) {
	var r workRow
	err := q.QueryRowContext(ctx, `
		SELECT w.id, COALESCE(w.latest_version_id, ''), w.updated_at,
		       (SELECT COUNT(*) FROM saves s WHERE s.work_id = w.id)
		FROM works w WHERE w.id = ?`, id).Scan(&r.ID, &r.LatestVersionID, &r.UpdatedAt, &r.SaveCount)
	return r, err
}

type fileRow struct {
	Path     string
	BlobHash string
}

func versionFiles(ctx context.Context, q store.Querier, versionID string) ([]fileRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT path, blob_hash FROM work_files WHERE version_id = ? ORDER BY path`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fileRow
	for rows.Next() {
		var f fileRow
		if err := rows.Scan(&f.Path, &f.BlobHash); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
