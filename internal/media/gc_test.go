package media

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/yanyi/internal/store"
)

func TestCollectCacheProtectsExistingAndRecentImages(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	blobs := store.NewBlobs(filepath.Join(dir, "blobs"))
	var kept string
	if err := db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		kept, err = blobs.Put(ctx, tx, []byte("仍在使用的原图"), "image/png")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	kept = strings.TrimPrefix(kept, "sha256:") + "-200.jpg"
	removed := strings.Repeat("a", 64) + "-200.jpg"
	recent := strings.Repeat("b", 64) + "-200.jpg"
	for _, name := range []string{kept, removed, recent, "other.txt"} {
		p := filepath.Join(cache, name)
		if err := os.WriteFile(p, []byte("cache"), 0o600); err != nil {
			t.Fatal(err)
		}
		if name != recent {
			old := time.Now().Add(-2 * time.Hour)
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err := NewStore(db, blobs, cache).CollectCache(ctx)
	if err != nil || result.Blobs != 1 || result.Bytes != 5 {
		t.Fatalf("清理结果不符：%+v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(cache, removed)); !os.IsNotExist(err) {
		t.Fatal("孤立的旧缩略图没有删除", err)
	}
	for _, name := range []string{kept, recent, "other.txt"} {
		if _, err := os.Stat(filepath.Join(cache, name)); err != nil {
			t.Fatal("不应删除的缓存被清理", name, err)
		}
	}
}
