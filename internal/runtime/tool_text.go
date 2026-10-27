package runtime

import (
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/work"
)

// toolTextFilter 在流式正文中隔离以当前工具名为标签的伪调用；只识别边界，不解释参数。
// 普通文本、代码引用和作品声明的同名样式标签原样通过。未闭合的伪调用保留在模型消息与检查器中。
type toolTextFilter struct {
	names    []string
	found    []string
	seen     map[string]bool
	pending  string
	active   string
	header   bool
	closing  bool
	previous byte
	quote    byte
	ticks    int
	code     int
	escaped  bool
}

func newToolTextFilter(specs []llm.ToolSpec, tags []work.TagDef) *toolTextFilter {
	allowed := map[string]bool{}
	for _, tag := range tags {
		allowed[tag.Name] = true
	}
	f := &toolTextFilter{seen: map[string]bool{}}
	for _, spec := range specs {
		if !allowed[spec.Name] {
			f.names = append(f.names, spec.Name)
		}
	}
	return f
}

func (f *toolTextFilter) push(delta string) string {
	var out strings.Builder
	for i := range len(delta) {
		c := delta[i]
		if f.active != "" {
			f.skip(c)
			continue
		}
		if f.pending != "" {
			f.pending += delta[i : i+1]
			if f.detect(false) {
				continue
			}
			// 最后一个字符仍可能是另一个标签或代码引用的开头。
			out.WriteString(f.pending[:len(f.pending)-1])
			f.pending = ""
		}
		if c == '`' && !f.escaped {
			f.ticks++
			out.WriteByte(c)
			continue
		}
		if f.ticks > 0 {
			switch f.code {
			case 0:
				f.code = f.ticks
			case f.ticks:
				f.code = 0
			}
			f.ticks = 0
		}
		if c == '<' && f.code == 0 && !f.escaped {
			f.pending = "<"
			continue
		}
		if c == '\\' {
			f.escaped = !f.escaped
		} else {
			f.escaped = false
		}
		out.WriteByte(c)
	}
	return out.String()
}

// detect 只在完整工具名之后出现标签边界时匹配；流结束处的完整名称也视为截断的伪调用。
func (f *toolTextFilter) detect(final bool) bool {
	for _, name := range f.names {
		prefix := "<" + name
		if strings.HasPrefix(f.pending, prefix) && (len(f.pending) > len(prefix) || final) {
			if len(f.pending) > len(prefix) && !toolTagBoundary(f.pending[len(prefix)]) {
				continue
			}
			if !f.seen[name] {
				f.found = append(f.found, name)
				f.seen[name] = true
			}
			f.active, f.header = name, true
			f.previous, f.quote = 0, 0
			tail := f.pending[len(prefix):]
			// 部分兼容接口会在标签名后多写一个引号。
			if tail == "\"" || tail == "'" {
				tail = ""
			}
			f.pending = ""
			for i := range len(tail) {
				f.skip(tail[i])
			}
			return true
		}
		if !final && strings.HasPrefix(prefix, f.pending) {
			return true
		}
	}
	return false
}

func toolTagBoundary(c byte) bool {
	return c == '>' || c == '/' || c == '"' || c == '\'' || toolTagSpace(c)
}

func toolTagSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func (f *toolTextFilter) skip(c byte) {
	if f.header {
		if f.quote != 0 {
			if c == f.quote {
				f.quote = 0
			}
			return
		}
		if c == '\'' || c == '"' {
			f.quote = c
			return
		}
		if c == '>' {
			f.header = false
			if f.previous == '/' {
				f.active = ""
			}
		}
		f.previous = c
		return
	}
	if f.closing {
		if c == '>' {
			f.active, f.closing = "", false
		} else if !toolTagSpace(c) {
			f.closing = false
		}
		return
	}
	f.pending += string([]byte{c})
	end := "</" + f.active
	for f.pending != "" && !strings.HasPrefix(end, f.pending) {
		f.pending = f.pending[1:]
	}
	if f.pending == end {
		f.pending, f.closing = "", true
	}
}

func (f *toolTextFilter) flush() string {
	if f.active != "" || f.detect(true) {
		f.pending = ""
		return ""
	}
	s := f.pending
	f.pending = ""
	return s
}
