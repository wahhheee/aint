package server

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/store"
	"github.com/wahhheee/aint/internal/usage"
)

func TestUsageSummary(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "aint.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	cost := int64(2500)
	if err := db.Tx(ctx, func(tx *sql.Tx) error {
		return usage.Insert(ctx, tx, usage.Record{
			SaveID: "s1", NodeID: "t1", CallKind: "compact", ModelID: "m",
			Usage: llm.Usage{InputUncached: 10, Output: 2}, CostMicros: &cost, Currency: "CNY",
		})
	}); err != nil {
		t.Fatal(err)
	}
	h := New(Deps{Usage: usage.NewService(db), Web: fstest.MapFS{}}).Handler()

	rec := do(t, h, http.MethodGet, "/api/v1/usage?save_id=s1", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d：%s", rec.Code, rec.Body)
	}
	var got usage.Summary
	if err := jsonx.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total.Costs["CNY"] != 2500 || got.ByKind["compact"].Calls != 1 || len(got.ByModel) != 1 {
		t.Errorf("存档用量：%s", rec.Body)
	}

	// 没有调用时分组是空集合，不是 null（前端直接遍历）
	rec = do(t, h, http.MethodGet, "/api/v1/usage?save_id=none", "", nil)
	want := `{"total":{"usage":{"input_uncached":0,"cache_read":0,"cache_write":0,"output":0,"reasoning":0},` +
		`"costs":{},"unpriced":0,"calls":0},"by_kind":{},"by_model":[]}`
	if body := rec.Body.String(); rec.Code != http.StatusOK || body != want+"\n" && body != want {
		t.Errorf("空结果：%d %s", rec.Code, body)
	}
}
