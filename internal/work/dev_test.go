package work

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestDevelopmentReload(t *testing.T) {
	ctx := context.Background()
	works, im := newTestImporter(t)
	dir := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("work.yaml", minimalManifest)
	write("prompts/narrator.md", "最初的指令")
	preview, err := im.PreviewDirectory(ctx, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	res, err := im.Confirm(ctx, preview.Task)
	if err != nil {
		t.Fatal(err)
	}
	first, err := works.Model(ctx, res.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan *Report, 10)
	closeWatcher, err := works.WatchDevelopment(ctx, func(_ string, rep *Report) { events <- rep })
	if err != nil {
		t.Fatal(err)
	}
	defer closeWatcher()
	// 启动重载覆盖监听器尚未安装时的文件变更。
	write("prompts/narrator.md", "新的指令")
	wait := func(wantError bool) {
		t.Helper()
		timer := time.NewTimer(8 * time.Second)
		defer timer.Stop()
		for {
			select {
			case report := <-events:
				if report.HasErrors() == wantError {
					return
				}
			case <-timer.C:
				t.Fatal("未收到期望的热重载报告")
			}
		}
	}
	wait(false)
	second, err := works.Model(ctx, res.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first.TreeHash == second.TreeHash || first.Narrator == second.Narrator {
		t.Fatal("应替换缓存并保持旧模型快照")
	}
	write("work.yaml", "format: [invalid")
	wait(true)
	unchanged, err := works.Model(ctx, res.VersionID)
	if err != nil || unchanged != second {
		t.Fatal("非法修改替换了可用模型", err)
	}
	write("work.yaml", minimalManifest)
	wait(false)
	versions, err := works.Versions(ctx, res.WorkID)
	if err != nil || len(versions) != 1 || versions[0].Errors != 0 {
		t.Fatal("恢复相同内容应清除错误", versions, err)
	}
	write("lore/nested/new.md", "# 新设定\n新目录的内容")
	wait(false)
	set, err := works.FileSet(ctx, res.WorkID, res.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set.Get("lore/nested/new.md"); !ok {
		t.Fatal("没有读取新建子目录")
	}
	if _, err := im.PreviewDirectory(ctx, "", true); err == nil {
		t.Fatal("不能把空路径当作当前目录")
	}
}

func TestDeletedBuiltinStaysDeleted(t *testing.T) {
	works, _ := newTestImporter(t)
	builtin := fstest.MapFS{"demo/work.yaml": {Data: []byte(minimalManifest)}, "demo/prompts/narrator.md": {Data: []byte("讲故事")}}
	ctx := context.Background()
	works.EnsureBuiltins(ctx, builtin)
	if err := works.Delete(ctx, "test.import", ""); err != nil {
		t.Fatal(err)
	}
	works.EnsureBuiltins(ctx, builtin)
	if _, err := works.LatestVersionID(ctx, "test.import"); err == nil {
		t.Fatal("已删除的内置作品重新出现")
	}
	sub, err := builtin.Sub("demo")
	if err != nil {
		t.Fatal(err)
	}
	set, err := FromFS(sub, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := works.ImportFileSet(ctx, set, Source{Type: "zip"}); err != nil {
		t.Fatal("手动导入应允许恢复", err)
	}
}
