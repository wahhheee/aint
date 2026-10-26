package media

import (
	"context"

	"github.com/wahhheee/aint/internal/store"
)

func insertUpload(ctx context.Context, q store.Querier, u *Upload) error {
	_, err := q.ExecContext(ctx, `INSERT INTO uploads (hash, mime, width, height, size, created_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(hash) DO UPDATE SET created_at = excluded.created_at`,
		u.Hash, u.MIME, u.Width, u.Height, u.Size, store.NowMillis())
	return err
}

func getUpload(ctx context.Context, q store.Querier, hash string) (*Upload, error) {
	u := &Upload{}
	err := q.QueryRowContext(ctx, `SELECT hash, mime, width, height, size FROM uploads WHERE hash = ?`, hash).
		Scan(&u.Hash, &u.MIME, &u.Width, &u.Height, &u.Size)
	return u, err
}
