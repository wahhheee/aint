package save

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wahhheee/aint/internal/store"
	"github.com/wahhheee/aint/internal/work"
)

func TestStorageCollectionPreservesBranchesAndUploads(t *testing.T) {
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
	works := work.NewService(db, blobs)
	set, err := work.FromFS(fstest.MapFS{"work.yaml": {Data: []byte("format: 1\nid: test.gc\nversion: 1.0.0\ntitle: 清理测试\n")}, "prompts/narrator.md": {Data: []byte("故事")}}, work.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := works.ImportFileSet(ctx, set, work.Source{Type: "zip"})
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, blobs, works, nil)
	sv, err := s.Create(ctx, CreateParams{WorkID: imported.WorkID})
	if err != nil {
		t.Fatal(err)
	}
	a := "第一条分支的独有记忆"
	first, err := s.EditMemory(ctx, sv.ID, "/memory/a.md", &a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetHead(ctx, sv.ID, sv.HeadNodeID, HeadHere); err != nil {
		t.Fatal(err)
	}
	b := "第二条分支的记忆"
	second, err := s.EditMemory(ctx, sv.ID, "/memory/b.md", &b)
	if err != nil {
		t.Fatal(err)
	}
	orphan, attached, recentUpload, fresh := "", "", "", ""
	oldTime := time.Now().Add(-2 * time.Hour)
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		orphan, err = blobs.Put(ctx, tx, []byte(strings.Repeat("无引用的大文件", 10000)), "")
		if err != nil {
			return err
		}
		attached, err = blobs.Put(ctx, tx, []byte("历史分支的上传图片"), "image/png")
		if err != nil {
			return err
		}
		recentUpload, err = blobs.Put(ctx, tx, []byte("刚上传还未提交的图片"), "image/png")
		if err != nil {
			return err
		}
		for _, hash := range []string{attached, recentUpload} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO uploads (hash,mime,width,height,size,created_at) VALUES (?, 'image/png', 1, 1, 20, ?)`, hash, oldTime.UnixMilli()); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE nodes SET input = ? WHERE id = ?`, `{"kind":"free","attachments":["`+attached+`"]}`, first.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE blobs SET created_at = ?`, oldTime.UnixMilli()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE uploads SET created_at = ? WHERE hash = ?`, store.NowMillis(), recentUpload); err != nil {
			return err
		}
		fresh, err = blobs.Put(ctx, tx, []byte("尚在保护期的新内容"), "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	hex := strings.TrimPrefix(orphan, "sha256:")
	path := filepath.Join(dir, "blobs", hex[:2], hex[2:4], hex)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	result, err := s.CollectStorage(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Blobs != 1 || result.Bytes < store.InlineMax {
		t.Fatal("清理数量错误", result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("磁盘文件未清理", err)
	}
	for _, hash := range []string{first.MemoryHash, second.MemoryHash, attached, recentUpload, fresh} {
		if _, err := blobs.Get(ctx, db.Read(), hash); err != nil {
			t.Fatalf("仍需使用的 blob 被清除 %s: %v", hash, err)
		}
	}
	mf, err := s.LoadMemory(ctx, first.MemoryHash)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := blobs.Get(ctx, db.Read(), mf.Files["/memory/a.md"].Blob); err != nil || string(data) != a {
		t.Fatal("旧分支的嵌套记忆丢失", err)
	}
	if err := s.Delete(ctx, sv.ID); err != nil {
		t.Fatal(err)
	}
	if result, err := s.CollectStorage(ctx, true); err != nil || result.Blobs != 0 {
		t.Fatal("一周内不应重复启动清理", result, err)
	}
	if _, err := s.CollectStorage(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Get(ctx, db.Read(), attached); !errors.Is(err, store.ErrBlobNotFound) {
		t.Fatal("存档删除后旧附件未清理", err)
	}
	if _, err := works.FileSet(ctx, imported.WorkID, imported.VersionID); err != nil {
		t.Fatal("清理破坏作品文件", err)
	}
}
