package vfs

import (
	"bytes"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

// mediaKinds 按扩展名识别不以文字读取的文件（与《03-作品格式规范》13.1 的素材格式一致）。
var mediaKinds = map[string]struct{ kind, mime string }{
	".png": {"图片", "image/png"}, ".jpg": {"图片", "image/jpeg"}, ".jpeg": {"图片", "image/jpeg"},
	".webp": {"图片", "image/webp"}, ".gif": {"图片", "image/gif"},
	".mp3": {"音频文件", ""}, ".ogg": {"音频文件", ""}, ".m4a": {"音频文件", ""},
	".woff2": {"字体文件", ""},
}

// mediaOf 返回文件的媒体类别（图片 / 音频文件 / 字体文件）与 MIME；文本或未知类型返回空。
func mediaOf(p string) (kind, mime string) {
	m := mediaKinds[strings.ToLower(path.Ext(p))]
	return m.kind, m.mime
}

const bom = "\ufeff"

// isText 判断内容能否作为文本读取：合法 UTF-8 且不含 NUL。
func isText(data []byte) bool {
	return utf8.Valid(data) && bytes.IndexByte(data, 0) < 0
}

// Lines 把文本拆成行（规则同 read：去掉 BOM，统一换行为 LF，末尾的换行不产生空行）。
func Lines(data []byte) []string { return splitLines(data) }

// splitLines 把文本拆成行：去掉 BOM，统一换行为 LF，末尾的换行不产生空行。
func splitLines(data []byte) []string {
	text := strings.TrimPrefix(string(data), bom)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// sliceLines 从第 offset 行（1 起）开始取至多 limit 行，总字数不超过 MaxReadRunes：
// 字数到达上限时在整行处结束；第一行本身就超过上限时截取该行的开头（lineCut）。
// 调用方保证 1 ≤ offset ≤ len(lines)。
func sliceLines(lines []string, offset, limit int) (text string, to int, lineCut bool) {
	var b strings.Builder
	budget := MaxReadRunes
	to = offset - 1
	for i := offset - 1; i < len(lines) && i < offset-1+limit; i++ {
		n := utf8.RuneCountInString(lines[i]) + 1 // 含换行
		if n > budget {
			if i == offset-1 { // 第一行就放不下：截取开头
				r := []rune(lines[i])
				b.WriteString(string(r[:min(len(r), MaxReadRunes)]))
				return b.String(), i + 1, true
			}
			break
		}
		if i > offset-1 {
			b.WriteByte('\n')
		}
		b.WriteString(lines[i])
		budget -= n
		to = i + 1
	}
	return b.String(), to, false
}

// Summary 取文本的一行说明（《02-技术方案》12.6）：frontmatter 的 summary，
// 否则取正文首个非空行（去掉标题符号，跳过 HTML 注释与分隔线），最多 60 字。
func Summary(text string) string {
	text = strings.ReplaceAll(strings.TrimPrefix(text, bom), "\r\n", "\n")
	if front, body, ok := cutFrontmatter(text); ok {
		var fm struct {
			Summary string `yaml:"summary"`
		}
		if yaml.Unmarshal([]byte(front), &fm) == nil && strings.TrimSpace(fm.Summary) != "" {
			return clip(strings.TrimSpace(fm.Summary), 60)
		}
		text = body
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
		if line == "" || strings.HasPrefix(line, "<!--") || line == "---" {
			continue
		}
		return clip(line, 60)
	}
	return ""
}

// cutFrontmatter 拆出开头 --- 包围的 frontmatter。
func cutFrontmatter(text string) (front, body string, ok bool) {
	rest, found := strings.CutPrefix(text, "---\n")
	if !found {
		return "", text, false
	}
	if strings.HasPrefix(rest, "---\n") || rest == "---" { // 空 frontmatter
		return "", strings.TrimPrefix(strings.TrimPrefix(rest, "---"), "\n"), true
	}
	i := strings.Index(rest, "\n---\n")
	if i < 0 {
		if !strings.HasSuffix(rest, "\n---") {
			return "", text, false
		}
		return rest[:len(rest)-4], "", true
	}
	return rest[:i], rest[i+5:], true
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
