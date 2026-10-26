package work

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/store"
)

// newTestImporter 创建使用临时数据库的作品服务与导入器。
func newTestImporter(t *testing.T) (*Service, *Importer) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "aint.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	works := NewService(db, store.NewBlobs(filepath.Join(dir, "blobs")))
	im, err := NewImporter(works, filepath.Join(dir, "tmp", "imports"), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	return works, im
}

func previewZip(t *testing.T, im *Importer, files ...zf) *ImportPreview {
	t.Helper()
	p, err := im.PreviewZip(context.Background(), "upload/misty.zip", bytes.NewReader(makeZip(t, files...)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// stagedEntries 返回暂存目录中的条目名（上传的临时 zip 与各任务的目录）。
func stagedEntries(t *testing.T, im *Importer) []string {
	t.Helper()
	entries, err := os.ReadDir(im.dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestImportTwoPhase(t *testing.T) {
	ctx := context.Background()
	works, im := newTestImporter(t)

	p := previewZip(t, im,
		zf{name: "misty/work.yaml", data: minimalManifest},
		zf{name: "misty/README.md", data: "# 导入测试"},
		zf{name: "misty/.git/HEAD", data: "ref: refs/heads/main"})
	if p.Task == "" || p.ExpiresAt == 0 {
		t.Fatalf("没有错误时应返回任务：%+v", p)
	}
	if p.Work.ID != "test.import" || p.Work.Title != "导入测试" || p.Work.Version != "0.1.0" {
		t.Fatalf("作品摘要不对：%+v", p.Work)
	}
	if p.Source.Type != "zip" || p.Source.Name != "misty.zip" || p.Files != 2 || p.Existing != nil {
		t.Fatalf("预览不对：%+v", p)
	}
	// 解析阶段不写入作品库
	if list, _ := works.List(ctx); len(list) != 0 {
		t.Fatalf("确认之前不应写入作品库：%d 部", len(list))
	}

	res, err := im.Confirm(ctx, p.Task)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.WorkID != "test.import" {
		t.Fatalf("应新建版本：%+v", res)
	}
	d, err := works.Get(ctx, "test.import")
	if err != nil {
		t.Fatal(err)
	}
	if d.Latest.ID != res.VersionID || d.Latest.Source.Type != "zip" || d.ReadmeHTML == "" {
		t.Fatalf("作品详情不对：%+v", d.Latest)
	}
	// 任务只能确认一次，暂存文件已删除
	if _, err := im.Confirm(ctx, p.Task); !errs.Is(err, errs.CodeNotFound) {
		t.Fatalf("重复确认应返回 not_found，得到 %v", err)
	}
	if left := stagedEntries(t, im); len(left) != 0 {
		t.Fatalf("确认后暂存目录应为空：%v", left)
	}
}

func TestImportReusesSameVersion(t *testing.T) {
	ctx := context.Background()
	works, im := newTestImporter(t)
	files := []zf{{name: "work.yaml", data: minimalManifest}, {name: "lore/a.md", data: "甲"}}

	first, err := im.Confirm(ctx, previewZip(t, im, files...).Task)
	if err != nil {
		t.Fatal(err)
	}

	// 同内容再导入：预览提示已有同哈希版本，确认后复用，不产生新版本
	again := previewZip(t, im, files...)
	if again.Existing == nil || again.Existing.SameVersionID != first.VersionID || again.Existing.LatestVersion != "0.1.0" {
		t.Fatalf("应提示已有相同版本：%+v", again.Existing)
	}
	res, err := im.Confirm(ctx, again.Task)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created || res.VersionID != first.VersionID {
		t.Fatalf("同作品同哈希应复用旧版本：%+v", res)
	}

	// 同内容的文件夹上传（套一层文件夹、带 .git）：同一条流水线，得到同一个文件树哈希
	u := im.NewFolderUpload()
	for _, f := range []struct{ path, data string }{
		{"misty/work.yaml", minimalManifest}, {"misty/lore/a.md", "甲"}, {"misty/.git/HEAD", "ref"},
	} {
		if err := u.Add(f.path, strings.NewReader(f.data)); err != nil {
			t.Fatal(err)
		}
	}
	fp, err := im.PreviewFolder(ctx, u, "")
	if err != nil {
		t.Fatal(err)
	}
	if fp.Source.Type != "dir" || fp.Source.Name != "misty" || fp.TreeHash != again.TreeHash {
		t.Fatalf("文件夹导入应与 zip 得到同一版本：%+v", fp)
	}
	if res, err := im.Confirm(ctx, fp.Task); err != nil || res.Created {
		t.Fatalf("文件夹导入应复用版本：%+v %v", res, err)
	}

	// 内容变化：作为新版本导入，成为最新版本
	changed := previewZip(t, im, zf{name: "work.yaml", data: strings.Replace(minimalManifest, "0.1.0", "0.2.0", 1)}, files[1])
	if changed.Existing == nil || changed.Existing.SameVersionID != "" || changed.Existing.Title != "导入测试" {
		t.Fatalf("应提示已有同 ID 作品、但没有相同版本：%+v", changed.Existing)
	}
	res, err = im.Confirm(ctx, changed.Task)
	if err != nil || !res.Created {
		t.Fatalf("内容变化应产生新版本：%+v %v", res, err)
	}
	d, err := works.Get(ctx, "test.import")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Versions) != 2 || d.Latest.ID != res.VersionID || d.Latest.Version != "0.2.0" {
		t.Fatalf("应有两个版本且最新为 0.2.0：%+v", d.Versions)
	}
}

func TestImportWithErrorsCannotConfirm(t *testing.T) {
	ctx := context.Background()
	works, im := newTestImporter(t)
	p := previewZip(t, im,
		zf{name: "work.yaml", data: "format: 1\nid: test.broken\nversion: 1\ntitle: 有错误的作品\n"},
		zf{name: "assets/cover.png", data: "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a\nsize 1\n"})
	if p.Task != "" {
		t.Fatal("有 error 时不应返回任务")
	}
	var codes []string
	for _, i := range p.Report.Issues {
		codes = append(codes, i.Code)
	}
	// 清单错误与其他文件的 LFS 错误都要报告（不因为有一处错误就停止检查）
	if got := strings.Join(codes, ","); !strings.Contains(got, CodeLFS) || !strings.Contains(got, CodeManifest) {
		t.Fatalf("报告应同时含 E-LFS 与 E-MANIFEST：%s", got)
	}
	if p.Work.Title != "有错误的作品" {
		t.Fatalf("清单有错误时仍应尽量给出摘要：%+v", p.Work)
	}
	if left := stagedEntries(t, im); len(left) != 0 {
		t.Fatalf("有错误时不应暂存：%v", left)
	}
	if _, err := im.Confirm(ctx, "no-such-task"); !errs.Is(err, errs.CodeNotFound) {
		t.Fatalf("不存在的任务应返回 not_found，得到 %v", err)
	}
	// 绕过预览直接写入同样被拒绝
	set := NewFileSet()
	_ = set.Add("work.yaml", []byte("format: 2\n"))
	if _, err := works.ImportFileSet(ctx, set, Source{Type: "zip"}); !errs.Is(err, errs.CodeWorkInvalid) {
		t.Fatalf("有错误的作品不能写入，得到 %v", err)
	}
}

func TestImportCancelAndExpire(t *testing.T) {
	ctx := context.Background()
	_, im := newTestImporter(t)
	files := []zf{{name: "work.yaml", data: minimalManifest}}

	p := previewZip(t, im, files...)
	im.Cancel(ctx, p.Task)
	im.Cancel(ctx, p.Task) // 重复取消无妨
	if _, err := im.Confirm(ctx, p.Task); !errs.Is(err, errs.CodeNotFound) {
		t.Fatalf("取消后不能确认，得到 %v", err)
	}
	if left := stagedEntries(t, im); len(left) != 0 {
		t.Fatalf("取消后暂存目录应为空：%v", left)
	}

	im.ttl = 20 * time.Millisecond
	p = previewZip(t, im, files...)
	deadline := time.Now().Add(5 * time.Second)
	for len(stagedEntries(t, im)) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("过期的导入没有被清理")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := im.Confirm(ctx, p.Task); !errs.Is(err, errs.CodeNotFound) {
		t.Fatalf("过期后不能确认，得到 %v", err)
	}
}

func TestImportStagedLimit(t *testing.T) {
	_, im := newTestImporter(t)
	var tasks []string
	for i := range maxStagedImports + 1 {
		p := previewZip(t, im, zf{name: "work.yaml", data: minimalManifest}, zf{name: "lore/a.md", data: strings.Repeat("甲", i+1)})
		tasks = append(tasks, p.Task)
		time.Sleep(2 * time.Millisecond) // 过期时间不同，"最早"才确定
	}
	if n := len(stagedEntries(t, im)); n != maxStagedImports {
		t.Fatalf("暂存数应保持在 %d，得到 %d", maxStagedImports, n)
	}
	if _, err := im.Confirm(context.Background(), tasks[0]); !errs.Is(err, errs.CodeNotFound) {
		t.Fatalf("最早的暂存应被丢弃，得到 %v", err)
	}
}

func TestNewImporterClearsLeftovers(t *testing.T) {
	works, im := newTestImporter(t)
	if err := os.WriteFile(filepath.Join(im.dir, "upload-1.zip"), []byte("上次运行留下的"), 0o600); err != nil {
		t.Fatal(err)
	}
	im2, err := NewImporter(works, im.dir, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if left := stagedEntries(t, im2); len(left) != 0 {
		t.Fatalf("启动时应清空暂存目录：%v", left)
	}
}

func TestPreviewZipRejectsBadPackage(t *testing.T) {
	_, im := newTestImporter(t)
	_, err := im.PreviewZip(context.Background(), "evil.zip", bytes.NewReader(makeZip(t,
		zf{name: "work.yaml", data: minimalManifest}, zf{name: "../../evil.txt", data: "x"})))
	wantReject(t, err, "..")
	if left := stagedEntries(t, im); len(left) != 0 {
		t.Fatalf("上传的临时 zip 应被删除：%v", left)
	}
}
