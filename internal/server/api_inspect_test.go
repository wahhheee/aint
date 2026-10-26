package server

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wahhheee/aint/examples"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/store"
	"github.com/wahhheee/aint/internal/tmpl"
	"github.com/wahhheee/aint/internal/work"
)

func TestDebugAPI(t *testing.T) {
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
	blobs := store.NewBlobs(filepath.Join(dir, "blobs"))
	works := work.NewService(db, blobs)
	works.EnsureBuiltins(ctx, examples.Works())
	saves := save.NewService(db, blobs, works, tmpl.New())
	sv, err := saves.Create(ctx, save.CreateParams{WorkID: "aint.first-story"})
	if err != nil {
		t.Fatal(err)
	}
	mgr := runtime.NewManager(runtime.Deps{Saves: saves})
	h := New(Deps{Saves: saves, Runtime: mgr, Web: fstest.MapFS{}}).Handler()
	base := "/api/v1/saves/" + sv.ID + "/debug"
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, base, "", 200},
		{http.MethodPatch, base, `{}`, 400},
		{http.MethodPatch, base, `{"enabled":true}`, 200},
		{http.MethodGet, base + "/calls/missing", "", 404},
		{http.MethodGet, base + "/calls/missing/file?phase=after&path=/memory/a.md&offset=no", "", 400},
		{http.MethodGet, base + "/calls/missing/file?phase=oops&path=/memory/a.md", "", 400},
		{http.MethodGet, base + "/calls/missing/images/secret", "", 404},
		{http.MethodGet, "/api/v1/saves/absent/debug", "", 404},
		{http.MethodDelete, base, "", 200},
		{http.MethodPatch, base, `{"enabled":false}`, 200},
	} {
		rec := do(t, h, tc.method, tc.path, "application/json", strings.NewReader(tc.body))
		if rec.Code != tc.status {
			t.Fatalf("%s %s：%d %s", tc.method, tc.path, rec.Code, rec.Body)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("调试响应不可缓存")
		}
	}
}
