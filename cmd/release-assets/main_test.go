package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/wahhheee/aint/internal/work"
)

func TestPrepare(t *testing.T) {
	usage, err := os.ReadFile("../../docs/usage.md")
	if err != nil {
		t.Fatal(err)
	}
	guide, err := os.ReadFile("../../docs/authoring.md")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	for _, dir := range []string{"docs", ".release/examples"} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string][]byte{
		"docs/usage.md": usage, "docs/authoring.md": guide,
		".release/examples/obsolete.zip": []byte("过期作品"),
	} {
		if err := os.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := prepare(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(".release/examples")
	if err != nil || len(entries) != 2 {
		t.Fatalf("应只包含两部内置作品：%v %v", entries, err)
	}
	for _, name := range []string{"first-story", "inverted-stars"} {
		data, err := os.ReadFile(filepath.Join(".release/examples", name+".zip"))
		if err != nil {
			t.Fatal(err)
		}
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		set, err := work.FromFS(reader, work.DefaultLimits)
		if err != nil {
			t.Fatal(err)
		}
		model, report := work.Compile(set)
		if report.HasErrors() || model.Manifest.ID != "aint."+name {
			t.Fatalf("作品包内容无效：%s %+v", name, report.Issues)
		}
	}
	got, err := os.ReadFile(".release/使用说明.md")
	if err != nil || !bytes.Equal(got, usage) {
		t.Fatalf("使用说明与源码不一致：%v", err)
	}
	got, err = os.ReadFile(".release/创作指南.md")
	if err != nil || bytes.Contains(got, []byte("(../skills/")) || !bytes.Contains(got, []byte("(skills/")) {
		t.Fatalf("创作指南的技能链接无效：%v", err)
	}
}
