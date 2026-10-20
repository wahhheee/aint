package store

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) (*DB, string) {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(dir, "yanyi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	return db, dir
}

func TestMigrateIdempotent(t *testing.T) {
	db, dir := openTest(t)
	if err := db.Migrate(context.Background(), filepath.Join(dir, "backup")); err != nil {
		t.Fatalf("重复迁移应为空操作: %v", err)
	}
}

func TestReadPoolIsQueryOnly(t *testing.T) {
	db, _ := openTest(t)
	_, err := db.Read().Exec(`INSERT INTO settings (key, value) VALUES ('x', '1')`)
	if err == nil {
		t.Fatal("读连接池不应允许写入")
	}
}

func TestBlobsInlineAndFile(t *testing.T) {
	db, dir := openTest(t)
	ctx := context.Background()
	blobs := NewBlobs(filepath.Join(dir, "blobs"))

	small := []byte("雾港来信")
	large := bytes.Repeat([]byte("a"), InlineMax+1)

	var hs, hl string
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if hs, err = blobs.Put(ctx, tx, small, "text/plain"); err != nil {
			return err
		}
		hl, err = blobs.Put(ctx, tx, large, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		hash string
		want []byte
	}{{hs, small}, {hl, large}} {
		got, err := blobs.Get(ctx, db.Read(), c.hash)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, c.want) {
			t.Fatalf("blob %s 内容不一致", c.hash)
		}
	}
}

func TestFTSTrigram(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO transcript_fts (text, node_id, save_id) VALUES (?, 'n1', 's1')`,
			"他弯腰，从缆绳下抽出一把银色钥匙，锈迹里还有海水的味道。")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := db.Read().QueryRow(`SELECT node_id FROM transcript_fts WHERE transcript_fts MATCH ?`, `"银色钥匙"`).Scan(&id); err != nil {
		t.Fatalf("trigram 中文子串检索失败: %v", err)
	}
}
