package work

import (
	"bytes"
	"strings"
)

// splitFrontmatter 把 Markdown 拆成 frontmatter（YAML）与正文。
// bodyLine 是正文在原文件中的起始行号（1 起），用于让正文中的问题定位到正确行号。
func splitFrontmatter(data []byte) (front []byte, body string, bodyLine int) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // 容忍 UTF-8 BOM（规范要求无 BOM，TODO：校验时提示）
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return nil, s, 1
	}
	rest := s[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, s, 1
	}
	front = []byte(rest[:end])
	after := rest[end+4:]
	after = strings.TrimPrefix(after, "\n")
	bodyLine = strings.Count(s[:len(s)-len(after)], "\n") + 1
	return front, after, bodyLine
}
