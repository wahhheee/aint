package markup

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxDepth = 4 // 行内元素最多嵌套 4 层

// voidTags 是空元素：没有内容，不写 "/" 也按自闭合处理。
var voidTags = map[string]bool{"scene": true, "cg": true, "bgm": true, "sfx": true, "pause": true}

// Parse 把叙事标记源码解析为块级节点列表。任何输入都有确定的结果，不会出错。
//
// 解析以"单元"推进：普通的一行，或从行首 <ooc> 到 </ooc> 的场外块（之后的文字另起一行）。
// 前端的增量解析器按同样的单元缓存已完成的部分。
func Parse(src string, opts Options) []Node {
	out := []Node{}
	pos := 0
	for {
		lineEnd := len(src)
		nl := strings.IndexByte(src[pos:], '\n')
		if nl >= 0 {
			lineEnd = pos + nl
		}
		if opts.has("ooc") {
			if off, ok := oocOpener(src[pos:lineEnd]); ok {
				bodyStart := pos + off
				inner := opts
				inner.inOOC = true
				if start, end, found := findOOCClose(src, bodyStart); found {
					inner.Drafting = false
					if children := Parse(src[bodyStart:start], inner); len(children) > 0 {
						out = append(out, Node{Type: NodeOOC, Children: children})
					}
					pos = end
					continue
				}
				// 尚未写到 </ooc>：到文末都属于场外块（流式中也先显示出来）
				if children := Parse(src[bodyStart:], inner); len(children) > 0 {
					out = append(out, Node{Type: NodeOOC, Children: children})
				}
				return out
			}
		}
		if nl < 0 {
			return append(out, lineBlocks(src[pos:], &opts, opts.Drafting)...)
		}
		out = append(out, lineBlocks(src[pos:lineEnd], &opts, false)...)
		pos = lineEnd + 1
	}
}

// oocOpener 判断一行是否以 <ooc> 开头（允许前导空白）；是则返回 <ooc> 之后的位置。
func oocOpener(line string) (int, bool) {
	k := len(line) - len(trimLeft(line))
	if k >= len(line) || line[k] != '<' {
		return 0, false
	}
	t, ok, _ := lexTag(line, k)
	if !ok || t.close || t.selfClose || t.name != "ooc" {
		return 0, false
	}
	return t.end, true
}

// findOOCClose 从 from 起查找第一个未被转义的 </ooc>，返回其起止位置。
// 转义按连续反斜杠的奇偶判断，与行内解析一致（from 总在行首或 <ooc> 之后，不会落在转义序列中间）。
func findOOCClose(src string, from int) (start, end int, found bool) {
	for i := from; ; {
		k := strings.Index(src[i:], "</ooc")
		if k < 0 {
			return 0, 0, false
		}
		i += k
		bs := 0
		for j := i - 1; j >= from && src[j] == '\\'; j-- {
			bs++
		}
		lineEnd := len(src)
		if nl := strings.IndexByte(src[i:], '\n'); nl >= 0 {
			lineEnd = i + nl
		}
		if bs%2 == 0 {
			if t, ok, _ := lexTag(src[:lineEnd], i); ok && t.close && t.name == "ooc" {
				return i, t.end, true
			}
		}
		i++
	}
}

// lineBlocks 把一行解析为块：分隔、引用、段落（段首 <scene/>、<cg/> 提升为块级，整段旁白为 aside）。
func lineBlocks(line string, opts *Options, tail bool) []Node {
	line = trimSpace(line)
	if line == "" {
		return nil
	}
	if isDivider(line) {
		return []Node{{Type: NodeDivider}}
	}
	if line[0] == '>' {
		children := parseInline(trimSpace(line[1:]), opts, tail)
		if len(children) == 0 {
			return nil
		}
		return []Node{{Type: NodeQuote, Children: children}}
	}

	children := parseInline(line, opts, tail)
	var out []Node
	for len(children) > 0 && children[0].Type == NodeTag && (children[0].Name == "scene" || children[0].Name == "cg") {
		out = append(out, Node{Type: NodeType(children[0].Name), Attrs: children[0].Attrs})
		children = children[1:]
		if len(children) > 0 && children[0].Type == NodeText {
			if t := trimLeft(children[0].Text); t == "" {
				children = children[1:]
			} else {
				children[0].Text = t
			}
		}
	}
	switch {
	case len(children) == 0:
		return out
	case len(children) == 1 && children[0].Type == NodeTag && children[0].Name == "aside":
		return append(out, Node{Type: NodeAside, Children: children[0].Children})
	default:
		return append(out, Node{Type: NodeP, Children: children})
	}
}

// isDivider：整行只有 3 个以上的同一种 "-" 或 "*"，中间可有空白。
func isDivider(line string) bool {
	c := line[0]
	if c != '-' && c != '*' {
		return false
	}
	n := 0
	for _, r := range line {
		switch {
		case r == rune(c):
			n++
		case !isSpace(r):
			return false
		}
	}
	return n >= 3
}

// ---------------------------------------------------------------------------
// 行内

type frame struct {
	typ      NodeType
	name     string
	attrs    map[string]string
	children []Node
}

type inlineParser struct {
	line  string
	opts  *Options
	tail  bool // 草稿态的最后一行：隐藏段尾未写完的标记
	stack []*frame
	buf   strings.Builder
	stop  bool
	// noCloser[0/1]：已确认从该位置起没有 "*" / "**" 结束符（避免逐个开始符重复扫描到行尾）
	noCloser [2]int
}

func parseInline(line string, opts *Options, tail bool) []Node {
	p := &inlineParser{line: line, opts: opts, tail: tail, stack: []*frame{{}}, noCloser: [2]int{-1, -1}}
	n := len(line)
	for i := 0; i < n && !p.stop; {
		k := strings.IndexAny(line[i:], `\<*`)
		if k < 0 {
			p.buf.WriteString(line[i:])
			break
		}
		p.buf.WriteString(line[i : i+k])
		i += k
		switch line[i] {
		case '\\':
			if i+1 < n && escapable(line[i+1]) {
				p.buf.WriteByte(line[i+1])
				i += 2
				continue
			}
			if i+1 == n && tail {
				p.stop = true // 段尾的反斜杠：可能是未写完的转义
				continue
			}
		case '<':
			t, ok, partial := lexTag(line, i)
			if ok {
				p.tag(t, line[i:t.end])
				i = t.end
				continue
			}
			if partial && tail {
				p.stop = true // 段尾未写完的标签：先不显示
				continue
			}
		case '*':
			i = p.star(i)
			continue
		}
		p.buf.WriteByte(line[i])
		i++
	}
	p.closeTo(1)
	p.flush()
	return p.stack[0].children
}

func (p *inlineParser) top() *frame { return p.stack[len(p.stack)-1] }

func (p *inlineParser) flush() {
	if p.buf.Len() > 0 {
		f := p.top()
		f.children = append(f.children, Node{Type: NodeText, Text: p.buf.String()})
		p.buf.Reset()
	}
}

func (p *inlineParser) emit(n Node) {
	p.flush()
	f := p.top()
	f.children = append(f.children, n)
}

func (p *inlineParser) open(f *frame) {
	p.flush()
	p.stack = append(p.stack, f)
}

// closeTo 闭合栈中下标 ≥ idx 的元素（自顶向下），挂到各自的父元素上。
func (p *inlineParser) closeTo(idx int) {
	p.flush()
	for len(p.stack) > idx {
		f := p.top()
		p.stack = p.stack[:len(p.stack)-1]
		parent := p.top()
		parent.children = append(parent.children, Node{Type: f.typ, Name: f.name, Attrs: f.attrs, Children: f.children})
	}
}

func (p *inlineParser) depthOK() bool { return len(p.stack)-1 < maxDepth }

func (p *inlineParser) tag(t tagToken, raw string) {
	if !p.opts.has(t.name) {
		p.emit(Node{Type: NodeRaw, Text: raw})
		return
	}
	if t.close {
		if voidTags[t.name] {
			return
		}
		for i := len(p.stack) - 1; i > 0; i-- {
			if f := p.stack[i]; f.typ == NodeTag && f.name == t.name {
				p.closeTo(i)
				return
			}
		}
		return // 没有对应的开始标签：忽略
	}
	attrs := parseAttrs(t.attrs)
	switch {
	case t.selfClose || voidTags[t.name]:
		p.emit(Node{Type: NodeTag, Name: t.name, Attrs: attrs})
	case p.depthOK():
		p.open(&frame{typ: NodeTag, name: t.name, attrs: attrs})
	}
	// 超过嵌套上限：忽略这个开始标签，内容照常显示（结束标签随之被忽略）
}

// star 处理从 i 开始的一串 "*"，返回处理后的位置。
func (p *inlineParser) star(i int) int {
	line := p.line
	run := 0
	for i+run < len(line) && line[i+run] == '*' {
		run++
	}
	if p.tail && i+run == len(line) {
		p.stop = true // 段尾的星号：可能是未写完的强调符
		return i + run
	}
	canClose := i > 0 && !spaceBefore(line, i)
	top := p.top().typ
	if canClose && top == NodeStrong && run >= 2 {
		p.closeTo(len(p.stack) - 1)
		return i + 2
	}
	// 栈顶是强调：用一个星号闭合它；除非这是一个能成立的加重开始符（*斜体**加粗**斜体*）
	if canClose && top == NodeEm && (run < 2 || !p.canOpen(i+2, "**")) {
		p.closeTo(len(p.stack) - 1)
		return i + 1
	}
	delim, typ := "*", NodeEm
	if run >= 2 {
		delim, typ = "**", NodeStrong
	}
	j := i + len(delim)
	if p.canOpen(j, delim) {
		p.open(&frame{typ: typ})
	} else {
		p.buf.WriteString(delim)
	}
	return j
}

// canOpen：j 是开始符之后的位置。嵌套未超限、紧跟非空白字符、本行后面有结束符（草稿态最后一行不要求）。
func (p *inlineParser) canOpen(j int, delim string) bool {
	if !p.depthOK() || j >= len(p.line) {
		return false
	}
	if r, _ := utf8.DecodeRuneInString(p.line[j:]); isSpace(r) {
		return false
	}
	return p.tail || p.hasCloser(j, delim)
}

// hasCloser：from 之后（不含 from）是否有前面为非空白字符的结束符。转义的星号不算。
func (p *inlineParser) hasCloser(from int, delim string) bool {
	slot := len(delim) - 1
	if c := p.noCloser[slot]; c >= 0 && from >= c {
		return false
	}
	line := p.line
	for j := from; j < len(line); j++ {
		switch line[j] {
		case '\\':
			if j+1 < len(line) && escapable(line[j+1]) {
				j++
			}
		case '*':
			if j > from && strings.HasPrefix(line[j:], delim) && !spaceBefore(line, j) {
				return true
			}
		}
	}
	p.noCloser[slot] = from
	return false
}

// ---------------------------------------------------------------------------
// 词法

type tagToken struct {
	name      string
	close     bool   // </name>
	selfClose bool   // <name …/>
	attrs     string // 名称之后、">" 之前的原文
	end       int    // ">" 之后的位置
}

// lexTag 识别 s[i] == '<' 处的标签。ok 为 false 时，partial 表示到 s 末尾仍可能是未写完的标签。
func lexTag(s string, i int) (t tagToken, ok, partial bool) {
	j := i + 1
	if j < len(s) && s[j] == '/' {
		t.close = true
		j++
	}
	if j >= len(s) {
		return t, false, true
	}
	if !isLower(s[j]) {
		return t, false, false
	}
	start := j
	for j < len(s) && isNameChar(s[j]) {
		j++
	}
	t.name = s[start:j]
	if j >= len(s) {
		return t, false, true
	}
	if c := s[j]; c != '>' && c != '/' {
		if r, _ := utf8.DecodeRuneInString(s[j:]); !isSpace(r) {
			return t, false, false
		}
	}
	attrStart := j
	inQuote := false
	for ; j < len(s); j++ {
		if s[j] == '"' {
			inQuote = !inQuote
		} else if s[j] == '>' && !inQuote {
			t.attrs = s[attrStart:j]
			t.selfClose = !t.close && strings.HasSuffix(trimRight(t.attrs), "/")
			t.end = j + 1
			return t, true, false
		}
	}
	return t, false, true
}

// parseAttrs 取出 name="值" 形式的属性。名称由 [A-Za-z0-9_-] 组成、以字母或下划线开头；其余内容（单引号、
// 无引号的值、不合法的名称及其值、杂字）忽略。同名属性取第一个。值原样保留（不解码实体与转义）。
func parseAttrs(s string) map[string]string {
	var attrs map[string]string
	for k := 0; k < len(s); {
		switch c := s[k]; {
		case c == '"': // 游离的引号串：整体跳过
			q := strings.IndexByte(s[k+1:], '"')
			if q < 0 {
				return attrs
			}
			k += q + 2
		case isAttrChar(c):
			st := k
			for k < len(s) && isAttrChar(s[k]) {
				k++
			}
			name := s[st:k]
			m := skipSpace(s, k)
			if m >= len(s) || s[m] != '=' {
				continue
			}
			m = skipSpace(s, m+1)
			if m >= len(s) || s[m] != '"' {
				continue
			}
			q := strings.IndexByte(s[m+1:], '"')
			if q < 0 {
				return attrs
			}
			if _, dup := attrs[name]; !dup && isAttrStart(name[0]) {
				if attrs == nil {
					attrs = map[string]string{}
				}
				attrs[name] = s[m+1 : m+1+q]
			}
			k = m + q + 2
		default:
			k++
		}
	}
	return attrs
}

// isSpace 是空白字符集：Unicode 空白（含全角空格 U+3000、不换行空格）与 BOM，与前端 /[\s\u0085]/ 相同。
func isSpace(r rune) bool { return unicode.IsSpace(r) || r == 0xFEFF }

func trimSpace(s string) string { return strings.TrimFunc(s, isSpace) }
func trimLeft(s string) string  { return strings.TrimLeftFunc(s, isSpace) }
func trimRight(s string) string { return strings.TrimRightFunc(s, isSpace) }

func skipSpace(s string, k int) int { return len(s) - len(trimLeft(s[k:])) }

func spaceBefore(s string, i int) bool {
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return isSpace(r)
}

func escapable(c byte) bool {
	return c == '\\' || c == '*' || c == '<' || c == '>' || c == '-'
}

func isLower(c byte) bool     { return c >= 'a' && c <= 'z' }
func isNameChar(c byte) bool  { return isLower(c) || c >= '0' && c <= '9' || c == '-' }
func isAttrStart(c byte) bool { return isLower(c) || c >= 'A' && c <= 'Z' || c == '_' }
func isAttrChar(c byte) bool  { return isAttrStart(c) || c >= '0' && c <= '9' || c == '-' }
