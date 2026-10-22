package server

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/store"
	"github.com/wahhheee/yanyi/internal/work"
)

const testManifest = "format: 1\nid: test.upload\nversion: 0.1.0\ntitle: 上传测试\n"

func newImportServer(t *testing.T, lim work.Limits) http.Handler {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "yanyi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	works := work.NewService(db, store.NewBlobs(filepath.Join(dir, "blobs")))
	im, err := work.NewImporter(works, filepath.Join(dir, "imports"), lim)
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Works: works, Imports: im, Web: fstest.MapFS{}}).Handler()
}

// form 按顺序写出 multipart 字段：偶数位是字段名，奇数位是值；字段名为 file 时值作为文件内容。
func form(t *testing.T, fields ...string) (string, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for len(fields) >= 2 {
		name, value := fields[0], fields[1]
		fields = fields[2:]
		var err error
		if name == "file" {
			var w io.Writer
			if w, err = mw.CreateFormFile("file", "blob"); err == nil {
				_, err = io.WriteString(w, value)
			}
		} else {
			err = mw.WriteField(name, value)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mw.FormDataContentType(), &buf
}

func do(t *testing.T, h http.Handler, method, path, contentType string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, body)
	req.Host = "127.0.0.1:7860"
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func zipOf(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := jsonx.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("响应不是合法的 JSON：%v\n%s", err, rec.Body.String())
	}
	return v
}

func TestImportZipUpload(t *testing.T) {
	h := newImportServer(t, work.DefaultLimits)
	ct, body := form(t, "source", "zip", "name", "上传测试.zip",
		"file", zipOf(t, map[string]string{"upload/work.yaml": testManifest, "upload/lore/a.md": "甲"}))
	rec := do(t, h, http.MethodPost, "/api/v1/works/import", ct, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("预览应成功：%d %s", rec.Code, rec.Body.String())
	}
	p := decodeJSON[work.ImportPreview](t, rec)
	if p.Task == "" || p.Work.ID != "test.upload" || p.Source.Name != "上传测试.zip" || p.Files != 2 {
		t.Fatalf("预览不对：%+v", p)
	}

	rec = do(t, h, http.MethodPost, "/api/v1/works/import/"+p.Task+"/confirm", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("确认应成功：%d %s", rec.Code, rec.Body.String())
	}
	if res := decodeJSON[work.ImportResult](t, rec); !res.Created || res.WorkID != "test.upload" {
		t.Fatalf("导入结果不对：%+v", res)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/works/test.upload", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("导入后应能查到作品：%d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/works/import/"+p.Task+"/confirm", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("重复确认应返回 404：%d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/v1/works/import/"+p.Task, "", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("放弃不存在的导入也应成功：%d", rec.Code)
	}
}

func TestImportFolderUpload(t *testing.T) {
	h := newImportServer(t, work.DefaultLimits)
	ct, body := form(t, "source", "dir", "name", "upload",
		"path", "upload/work.yaml", "file", testManifest,
		"path", "upload/lore/a.md", "file", "甲",
		"path", "upload/.git/HEAD", "file", "ref")
	rec := do(t, h, http.MethodPost, "/api/v1/works/import", ct, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("预览应成功：%d %s", rec.Code, rec.Body.String())
	}
	p := decodeJSON[work.ImportPreview](t, rec)
	if p.Task == "" || p.Source.Type != "dir" || p.Source.Name != "upload" || p.Files != 2 {
		t.Fatalf("预览不对：%+v", p)
	}

	// 取消后不能再确认
	if rec := do(t, h, http.MethodDelete, "/api/v1/works/import/"+p.Task, "", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("取消应成功：%d", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/works/import/"+p.Task+"/confirm", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("取消后确认应返回 404：%d", rec.Code)
	}
}

func TestImportRejects(t *testing.T) {
	h := newImportServer(t, work.Limits{MaxTotal: 1 << 10, MaxFiles: 10, MaxFileSize: 1 << 10, MaxDepth: 4})
	cases := []struct {
		name   string
		fields []string
		status int
		want   string
	}{
		{"缺少 source", []string{"file", "x"}, http.StatusBadRequest, "source"},
		{"没有文件", []string{"source", "zip"}, http.StatusBadRequest, "没有收到"},
		{"文件缺少路径", []string{"source", "dir", "file", "x"}, http.StatusBadRequest, "相对路径"},
		{"未知字段", []string{"source", "zip", "foo", "bar"}, http.StatusBadRequest, "未知的表单字段"},
		{"路径穿越", []string{"source", "dir", "path", "../work.yaml", "file", "x"}, http.StatusUnprocessableEntity, ".."},
		{"不是 zip", []string{"source", "zip", "file", "not a zip"}, http.StatusUnprocessableEntity, "不是有效的 zip"},
		{"超过上传上限", []string{"source", "zip", "file", strings.Repeat("x", 20<<20)}, http.StatusUnprocessableEntity, "过大"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ct, body := form(t, c.fields...)
			rec := do(t, h, http.MethodPost, "/api/v1/works/import", ct, body)
			if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.want) {
				t.Fatalf("应为 %d（%s），得到 %d %s", c.status, c.want, rec.Code, rec.Body.String())
			}
		})
	}

	rec := do(t, h, http.MethodPost, "/api/v1/works/import", "application/json", strings.NewReader(`{"github":"x"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("GitHub 导入尚未实现，应返回 501：%d", rec.Code)
	}
}
