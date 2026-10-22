package work

import (
	"fmt"
	"regexp"
	"strings"
)

// CSSIssue 是 custom.css 中的一个问题。
type CSSIssue struct {
	Line int
	Msg  string
}

// cssAtRules 是 custom.css 中允许的顶层 at 规则；@font-face 与 @keyframes 提到作用域之外，其余留在作用域内。
var cssAtRules = map[string]bool{"media": true, "supports": true, "container": true, "layer": true, "font-face": true, "keyframes": true}

var (
	reCSSForbidden = regexp.MustCompile(`(?i):root\b|image-set\s*\(|expression\s*\(|-moz-binding|behavior\s*:`)
	reCSSURL       = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)\s]*))\s*\)`)
	reCSSAnimation = regexp.MustCompile(`(?i)(animation(?:-name)?\s*:)([^;{}]*)`)
)

// ScopeCSS 校验作品的 custom.css 并把它限制在阅读页内（《02-技术方案》17）：
//
//   - 不允许 @import、外部 url()、:root、反斜杠转义（转义可以拼出被拦截的写法）与 image-set() 等；
//   - url(asset:ID) 换成作品素材的地址；
//   - 其余内容用 CSS 嵌套包在 .work-root { … } 中；@font-face 与 @keyframes 不能嵌套，提到外面，
//     动画名加上 wk- 前缀（animation / animation-name 中的引用同步改名），不会覆盖平台的动画。
//
// asset 把素材引用解析为地址。返回编译后的 CSS；有问题时返回问题列表（此时 CSS 为空）。
func ScopeCSS(src string, asset func(ref string) (string, bool)) (string, []CSSIssue) {
	src = stripCSSComments(strings.ReplaceAll(src, "\r\n", "\n"))
	var issues []CSSIssue
	lineAt := func(off int) int { return strings.Count(src[:off], "\n") + 1 }
	fail := func(off int, format string, args ...any) {
		issues = append(issues, CSSIssue{Line: lineAt(off), Msg: fmt.Sprintf(format, args...)})
	}

	if i := strings.IndexByte(src, '\\'); i >= 0 {
		fail(i, "不支持反斜杠转义，需要特殊字符时直接写出该字符")
	}
	for _, loc := range reCSSForbidden.FindAllStringIndex(src, -1) {
		fail(loc[0], "不允许使用 %s", strings.TrimSpace(src[loc[0]:loc[1]]))
	}

	// 按顶层语句切分：at 规则与普通规则
	var hoisted, body strings.Builder
	var keyframes []string
	depth, start := 0, 0
	var quote byte
	for i := range len(src) {
		ch := src[i]
		if quote != 0 {
			switch ch {
			case quote:
				quote = 0
			case '\n':
				fail(i, "字符串没有结束")
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				fail(i, "多了一个 }")
				depth = 0
				start = i + 1
				continue
			}
			if depth == 0 {
				stmt := src[start : i+1]
				if name, kind := atRule(stmt); kind != "" {
					switch {
					case !cssAtRules[kind]:
						fail(start+strings.Index(stmt, "@"), "不支持 @%s", kind)
					case kind == "font-face":
						hoisted.WriteString(strings.TrimSpace(stmt) + "\n")
					case kind == "keyframes":
						keyframes = append(keyframes, name)
						_, block, _ := strings.Cut(strings.TrimSpace(stmt), "{")
						hoisted.WriteString("@keyframes wk-" + name + " {" + block + "\n")
					default:
						body.WriteString(stmt)
					}
				} else {
					body.WriteString(stmt)
				}
				start = i + 1
			}
		case ';':
			if depth == 0 { // 顶层的 @import / @charset / @namespace 等语句
				stmt := src[start : i+1]
				if _, kind := atRule(stmt); kind == "charset" {
					// 编码声明没有意义（作品文件总是 UTF-8），去掉即可
				} else if kind != "" {
					fail(start+strings.Index(stmt, "@"), "不支持 @%s", kind)
				} else {
					body.WriteString(stmt) // 顶层声明：作用于 .work-root 本身
				}
				start = i + 1
			}
		}
	}
	if quote != 0 {
		fail(len(src), "字符串没有结束")
	}
	if depth != 0 {
		fail(len(src), "{ 与 } 不配对（少了 %d 个 }）", depth)
	}
	if rest := strings.TrimSpace(src[start:]); rest != "" && depth == 0 {
		body.WriteString(src[start:])
	}

	out := hoisted.String() + ".work-root {\n" + strings.TrimSpace(body.String()) + "\n}\n"
	// url()：只允许 asset: 引用
	out = reCSSURL.ReplaceAllStringFunc(out, func(m string) string {
		sub := reCSSURL.FindStringSubmatch(m)
		ref := sub[1] + sub[2] + sub[3]
		if r, ok := strings.CutPrefix(ref, "asset:"); ok {
			if u, ok := asset(r); ok {
				return fmt.Sprintf("url(%q)", u)
			}
			issues = append(issues, CSSIssue{Msg: fmt.Sprintf("url(%s) 引用的素材不存在", ref)})
			return m
		}
		issues = append(issues, CSSIssue{Msg: fmt.Sprintf("url(%s)：只能引用作品素材，写成 url(asset:素材ID)", ref)})
		return m
	})
	// 动画改名
	if len(keyframes) > 0 {
		out = reCSSAnimation.ReplaceAllStringFunc(out, func(m string) string {
			sub := reCSSAnimation.FindStringSubmatch(m)
			value := sub[2]
			for _, k := range keyframes {
				value = replaceIdent(value, k, "wk-"+k)
			}
			return sub[1] + value
		})
	}
	if len(issues) > 0 {
		return "", issues
	}
	return out, nil
}

// atRule 判断语句是否是 at 规则，返回名称（@keyframes 的动画名）与类型（小写，如 media、keyframes）。
func atRule(stmt string) (name, kind string) {
	s := strings.TrimSpace(stmt)
	if !strings.HasPrefix(s, "@") {
		return "", ""
	}
	end := strings.IndexAny(s, " \t\n{;(")
	if end < 0 {
		end = len(s)
	}
	kind = strings.ToLower(s[1:end])
	kind = strings.TrimPrefix(strings.TrimPrefix(kind, "-webkit-"), "-moz-")
	if kind == "keyframes" {
		rest := strings.TrimSpace(s[end:])
		if i := strings.IndexByte(rest, '{'); i >= 0 {
			name = strings.Trim(strings.TrimSpace(rest[:i]), `"'`)
		}
	}
	return name, kind
}

// replaceIdent 替换 value 中完整的标识符 old（前后不是标识符字符）。
func replaceIdent(value, old, repl string) string {
	isIdent := func(b byte) bool {
		return b == '-' || b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
	}
	var b strings.Builder
	for i := 0; i < len(value); {
		j := strings.Index(value[i:], old)
		if j < 0 {
			b.WriteString(value[i:])
			break
		}
		j += i
		end := j + len(old)
		if (j == 0 || !isIdent(value[j-1])) && (end == len(value) || !isIdent(value[end])) {
			b.WriteString(value[i:j] + repl)
		} else {
			b.WriteString(value[i:end])
		}
		i = end
	}
	return b.String()
}

// stripCSSComments 去掉 /* */ 注释，保留其中的换行（问题的行号不变）。
func stripCSSComments(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			b.WriteString(strings.Repeat("\n", strings.Count(s[i:], "\n")))
			return b.String()
		}
		b.WriteString(strings.Repeat("\n", strings.Count(s[i:i+2+j], "\n")))
		s = s[i+4+j:]
	}
}
