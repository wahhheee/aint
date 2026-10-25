package save

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/store"
)

// CollectStorage 以作品文件和全部历史节点为根，递归标记记忆文件、消息图片与附件。
// 未提交的上传与新 blob 有一小时保护期，清理失败不会删除仍被引用的内容。
func (s *Service) CollectStorage(ctx context.Context, weekly bool) (store.GCResult, error) {
	cutoff := time.Now().Add(-time.Hour)
	var result store.GCResult
	skipped := false
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		if weekly {
			var last int64
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT value FROM settings WHERE key = 'storage_gc_at'), '0')`).Scan(&last); err != nil {
				return err
			}
			if time.Since(time.UnixMilli(last)) < 7*24*time.Hour {
				skipped = true
				return nil
			}
		}
		marked := map[string]bool{}
		var pending []string
		mark := func(hash string) {
			if len(hash) == 71 && strings.HasPrefix(hash, "sha256:") && !marked[hash] {
				marked[hash] = true
				pending = append(pending, hash)
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT blob_hash, '' FROM work_files UNION ALL SELECT state_hash, input FROM nodes UNION ALL SELECT memory_hash, '' FROM nodes UNION ALL SELECT messages, '' FROM nodes UNION ALL SELECT display, '' FROM nodes UNION ALL SELECT epoch_block, '' FROM nodes UNION ALL SELECT hash, '' FROM uploads WHERE created_at >= ?`, cutoff.UnixMilli())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var hash, input sql.NullString
			if err := rows.Scan(&hash, &input); err != nil {
				return err
			}
			mark(hash.String)
			if input.Valid && input.String != "" {
				markJSON([]byte(input.String), mark)
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for len(pending) > 0 {
			hash := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			data, err := s.blobs.Get(ctx, tx, hash)
			if err != nil {
				return err
			}
			markJSON(data, mark)
		}
		uploads, err := tx.QueryContext(ctx, `SELECT hash FROM uploads WHERE created_at < ?`, cutoff.UnixMilli())
		if err != nil {
			return err
		}
		defer uploads.Close()
		var unreferenced []string
		for uploads.Next() {
			var hash string
			if err := uploads.Scan(&hash); err != nil {
				return err
			}
			if !marked[hash] {
				unreferenced = append(unreferenced, hash)
			}
		}
		if err := uploads.Err(); err != nil {
			return err
		}
		for _, hash := range unreferenced {
			if _, err := tx.ExecContext(ctx, `DELETE FROM uploads WHERE hash = ?`, hash); err != nil {
				return err
			}
		}
		result, err = s.blobs.Sweep(ctx, tx, marked, cutoff)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('storage_gc_at', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, store.NowMillis())
		return err
	})
	if err != nil || skipped {
		return store.GCResult{}, err
	}
	err = s.db.Tx(ctx, func(tx *sql.Tx) error { return s.blobs.SweepFiles(ctx, tx, cutoff) })
	return result, err
}

// markJSON 只追踪 JSON 字符串值中的完整 blob 引用，不把普通正文中的哈希当作引用。
func markJSON(data []byte, mark func(string)) {
	var v any
	if jsonx.Unmarshal(data, &v) != nil {
		return
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			mark(x)
		case []any:
			for _, value := range x {
				walk(value)
			}
		case map[string]any:
			for _, value := range x {
				walk(value)
			}
		}
	}
	walk(v)
}
