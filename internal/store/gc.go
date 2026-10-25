package store

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GCResult 是已清除的 blob 数量与字节数。
type GCResult struct {
	Blobs int   `json:"blobs"`
	Bytes int64 `json:"bytes"`
}

// Sweep 删除标记集合外且早于 cutoff 的 blob 记录。调用方在同一写事务内收集全部引用。
// 磁盘文件由 SweepFiles 在记录提交删除后清理，避免事务回滚造成已引用内容丢失。
func (s *Blobs) Sweep(ctx context.Context, tx *sql.Tx, marked map[string]bool, cutoff time.Time) (GCResult, error) {
	rows, err := tx.QueryContext(ctx, `SELECT hash, size FROM blobs WHERE created_at < ?`, cutoff.UnixMilli())
	if err != nil {
		return GCResult{}, err
	}
	defer rows.Close()
	type candidate struct {
		hash string
		size int64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.hash, &c.size); err != nil {
			return GCResult{}, err
		}
		if !marked[c.hash] {
			candidates = append(candidates, c)
		}
	}
	if err := rows.Err(); err != nil {
		return GCResult{}, err
	}
	var result GCResult
	for _, c := range candidates {
		if _, err := tx.ExecContext(ctx, `DELETE FROM blobs WHERE hash = ?`, c.hash); err != nil {
			return GCResult{}, err
		}
		result.Blobs++
		result.Bytes += c.size
	}
	return result, nil
}

// SweepFiles 在写事务内核对磁盘残留，防止与同哈希重新写入竞争。
// 仅删除数据库已不再引用且超过保护期的内容文件，也覆盖崩溃留下的孤立文件。
func (s *Blobs) SweepFiles(ctx context.Context, tx *sql.Tx, cutoff time.Time) error {
	if _, err := os.Stat(s.dir); os.IsNotExist(err) {
		return nil
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return filepath.WalkDir(s.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if len(name) != 64 || strings.Trim(name, "0123456789abcdef") != "" {
			return nil
		}
		if p != s.filePath("sha256:"+name) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.ModTime().Before(cutoff) {
			return nil
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM blobs WHERE hash = ?`, "sha256:"+name).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			rel, err := filepath.Rel(s.dir, p)
			if err != nil {
				return err
			}
			return root.Remove(rel)
		}
		return nil
	})
}
