package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/wahhheee/aint/internal/work"
)

func TestCreateValidateAndPack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "中文作品")
	if err := workInit([]string{dir}); err != nil {
		t.Fatal(err)
	}
	model, report, set, err := loadWorkFiles(dir)
	if err != nil || report.HasErrors() {
		t.Fatal(report, err)
	}
	if model.Manifest.ID == "" || len(set.Paths()) != 2 {
		t.Fatal("模板不完整")
	}
	if err := workInit([]string{dir}); err == nil {
		t.Fatal("不应覆盖已有作品")
	}
	if err := workPack([]string{dir, "-o", filepath.Join(dir, "bad.zip")}); err == nil {
		t.Fatal("输出不能位于作品内")
	}
	out := dir + ".zip"
	if err := workPack([]string{dir, "-o", out}); err != nil {
		t.Fatal(err)
	}
	packed, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := work.FromZip(bytes.NewReader(packed), int64(len(packed)), work.DefaultLimits)
	if err != nil || restored.TreeHash() != set.TreeHash() {
		t.Fatal("导出不能还原文件树", err)
	}
	if err := workPack([]string{dir, "-o", out}); err == nil {
		t.Fatal("不应覆盖已有包")
	}
}
