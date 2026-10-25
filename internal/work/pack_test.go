package work

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestPackRoundTrip(t *testing.T) {
	set := NewFileSet()
	for p, data := range map[string][]byte{
		"work.yaml": []byte(minimalManifest), "资料/人物.md": []byte("林先生住在港口。"),
		"大量重复.md": bytes.Repeat([]byte("故事"), 300000),
	} {
		if err := set.Add(p, data); err != nil {
			t.Fatal(err)
		}
	}
	var first, second bytes.Buffer
	if err := WriteZip(&first, set); err != nil {
		t.Fatal(err)
	}
	if err := WriteZip(&second, set); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("同一作品的两次打包必须逐字节一致")
	}
	archive, err := zip.NewReader(bytes.NewReader(first.Bytes()), int64(first.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range archive.File {
		if entry.Flags&0x800 == 0 {
			t.Fatal("文件名必须标记为 UTF-8", entry.Name)
		}
	}
	got, err := FromZip(bytes.NewReader(first.Bytes()), int64(first.Len()), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if got.TreeHash() != set.TreeHash() {
		t.Fatal("导出再导入必须保持文件树哈希")
	}
}
