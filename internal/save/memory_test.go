package save

import (
	"fmt"
	"strings"
	"testing"
)

func TestMemoryIndexOf(t *testing.T) {
	s := &Service{}
	files := map[string]MemoryFile{}
	data := map[string][]byte{}
	for i := range 81 {
		p := fmt.Sprintf("/memory/characters/%d.md", i)
		files[p] = MemoryFile{}
		data[p] = []byte("# 人物资料\n尚未见面。")
	}
	index, err := s.MemoryIndexOf(t.Context(), files, data)
	if err != nil || index.Text != nil || len(index.Entries) != 1 || index.Entries[0].Summary != "81 个文件" {
		t.Fatalf("缺少索引文件时应使用折叠目录：%+v %v", index, err)
	}
	for _, content := range []string{"# 总索引\n" + strings.Repeat("人物背景及关系（已查明来信来源，仍有动机未确认）。\n", 90), ""} {
		files["/memory/index.md"] = MemoryFile{}
		data["/memory/index.md"] = []byte(content)
		index, err = s.MemoryIndexOf(t.Context(), files, data)
		if err != nil || index.Text == nil || *index.Text != content || len(index.Entries) != 0 {
			t.Fatalf("现有索引应完整使用，包括空文件：%+v %v", index, err)
		}
	}
}
