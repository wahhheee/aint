package save

import (
	"path/filepath"
	"testing"

	"github.com/wahhheee/aint/internal/store"
)

func TestProgressLineMissingHead(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	// 读取存档后，节点可能被另一个请求删除；调用方不能收到成功的空路径。
	if line, err := pathToRoot(ctx, db.Read(), "removed"); err == nil || len(line) != 0 {
		t.Fatalf("缺失进度应返回错误：%v %v", line, err)
	}
}
