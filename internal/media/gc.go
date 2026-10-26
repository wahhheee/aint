package media

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"time"

	"github.com/wahhheee/aint/internal/store"
)

// CollectCache 清除原图已回收且超过一小时的缩略图缓存。
func (s *Store) CollectCache(ctx context.Context) (store.GCResult, error) {
	var result store.GCResult
	if s.cacheDir == "" {
		return result, nil
	}
	root, err := os.OpenRoot(s.cacheDir)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer func() { _ = root.Close() }()
	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		return result, err
	}
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		for _, entry := range entries {
			name := entry.Name()
			hash, _, ok := strings.Cut(name, "-")
			if entry.IsDir() || !ok || len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
				continue
			}
			info, err := root.Stat(name)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if time.Since(info.ModTime()) < time.Hour {
				continue
			}
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM blobs WHERE hash = ?`, "sha256:"+hash).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				if err := root.Remove(name); err != nil && !os.IsNotExist(err) {
					return err
				}
				result.Blobs++
				result.Bytes += info.Size()
			}
		}
		return nil
	})
	return result, err
}
