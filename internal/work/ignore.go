package work

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// IgnoreFile 是作品根的忽略规则文件（gitignore 语法，《03-作品格式规范》第 3 节）。
const IgnoreFile = ".aintignore"

// maxIgnoreFileSize 是 .aintignore 的大小上限：规则文件不需要更大，超过多半是放错了文件。
const maxIgnoreFileSize = 64 << 10

// alwaysIgnored 是始终忽略的文件名与目录名（任意层级）。
// __MACOSX 是 macOS"压缩"生成的资源分支目录，不忽略的话 zip 根目录会多出一个顶层目录，导致找不到作品根。
var alwaysIgnored = []string{".git", ".github", ".DS_Store", "Thumbs.db", "desktop.ini", "node_modules", "__MACOSX"}

// alwaysIgnoredPath 判断路径中是否有始终忽略的段。
func alwaysIgnoredPath(p string) bool {
	for seg := range strings.SplitSeq(p, "/") {
		if slices.Contains(alwaysIgnored, seg) {
			return true
		}
	}
	return false
}

// Ignore 是解析后的 .aintignore 规则。
//
// 自行实现 gitignore 语法（https://git-scm.com/docs/gitignore），而不是引入依赖：规则不多且有明确规范；
// 现有的 Go 实现要么多年未维护、`**` 处理有缺陷，要么随 go-git 一起引入大量代码。本实现覆盖：
// 注释与转义（`#`、`\#`、`\!`、`\ `）、否定（`!`）、只匹配目录（结尾 `/`）、锚定（开头或中间有 `/`）、
// `*` `?` `[...]`、`**`（开头、结尾、中间）；目录被忽略后其中的文件不能再用 `!` 找回（与 git 一致）。
// 不支持的只有子目录中的规则文件——作品只读取根目录的 .aintignore。
type Ignore struct {
	rules []ignoreRule
}

type ignoreRule struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool
}

// ParseIgnore 解析 .aintignore 内容。无法理解的行（如不完整的字符类导致正则无效）跳过。
func ParseIgnore(data []byte) *Ignore {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	ig := &Ignore{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if r, ok := parseIgnoreLine(strings.TrimSuffix(line, "\r")); ok {
			ig.rules = append(ig.rules, r)
		}
	}
	return ig
}

// Ignored 判断相对作品根的文件路径是否被忽略：任一上级目录被忽略，或文件本身被忽略（最后一条匹配的规则决定）。
func (ig *Ignore) Ignored(p string) bool {
	if ig == nil || len(ig.rules) == 0 {
		return false
	}
	for i := range len(p) {
		if p[i] == '/' && ig.match(p[:i], true) {
			return true
		}
	}
	return ig.match(p, false)
}

func (ig *Ignore) match(p string, isDir bool) bool {
	ignored := false
	for _, r := range ig.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.re.MatchString(p) {
			ignored = !r.negate
		}
	}
	return ignored
}

func parseIgnoreLine(line string) (ignoreRule, bool) {
	line = trimUnescapedSpaces(line)
	if line == "" || line[0] == '#' {
		return ignoreRule{}, false
	}
	var r ignoreRule
	switch {
	case line[0] == '!':
		r.negate, line = true, line[1:]
	case strings.HasPrefix(line, `\!`), strings.HasPrefix(line, `\#`):
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly, line = true, strings.TrimRight(line, "/")
	}
	if line == "" {
		return ignoreRule{}, false
	}
	// 开头或中间有 "/" 的模式相对作品根；否则在任意层级匹配
	anchored := strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")

	var b strings.Builder
	b.WriteString("^")
	if !anchored {
		b.WriteString("(?:.*/)?")
	}
	globToRegexp(&b, line)
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return ignoreRule{}, false
	}
	r.re = re
	return r, true
}

// globToRegexp 把 gitignore 的通配写法转为正则（不含首尾锚点）。
func globToRegexp(b *strings.Builder, g string) {
	for i := 0; i < len(g); {
		switch c := g[i]; {
		case c == '*':
			j := i
			for j < len(g) && g[j] == '*' {
				j++
			}
			atStart := i == 0 || g[i-1] == '/'
			atEnd := j == len(g) || g[j] == '/'
			switch {
			case j-i < 2 || !atStart || !atEnd: // 单个 *，或不在段边界上的 **：不跨目录
				b.WriteString("[^/]*")
			case j == len(g): // 结尾的 **：其中的一切
				b.WriteString(".*")
			default: // 开头或中间的 **/：零个或多个目录
				b.WriteString("(?:.*/)?")
				j++ // 连同后面的 "/"
			}
			i = j
		case c == '?':
			b.WriteString("[^/]")
			i++
		case c == '[':
			end := classEnd(g, i)
			if end < 0 {
				b.WriteString(`\[`)
				i++
				continue
			}
			class := g[i+1 : end]
			if class[0] == '!' {
				class = "^" + class[1:]
			}
			b.WriteString("[" + class + "]")
			i = end + 1
		case c == '\\' && i+1 < len(g):
			r, size := utf8.DecodeRuneInString(g[i+1:])
			b.WriteString(regexp.QuoteMeta(string(r)))
			i += 1 + size
		default:
			r, size := utf8.DecodeRuneInString(g[i:])
			b.WriteString(regexp.QuoteMeta(string(r)))
			i += size
		}
	}
}

// classEnd 返回从 g[start] == '[' 开始的字符类的结束位置（']' 的下标）；未闭合返回 -1。
// 紧跟在 '[' 或 '[!' 之后的 ']' 是字面量。
func classEnd(g string, start int) int {
	j := start + 1
	if j < len(g) && (g[j] == '!' || g[j] == '^') {
		j++
	}
	if j < len(g) && g[j] == ']' {
		j++
	}
	for ; j < len(g); j++ {
		switch g[j] {
		case '\\':
			j++
		case ']':
			return j
		}
	}
	return -1
}

// trimUnescapedSpaces 去掉行尾未被 "\" 转义的空格。
func trimUnescapedSpaces(s string) string {
	for strings.HasSuffix(s, " ") && !strings.HasSuffix(s, `\ `) {
		s = s[:len(s)-1]
	}
	return s
}
