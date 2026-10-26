package tools

import (
	"strings"

	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// decodeArgs 解码工具参数。
//
// 实测 Claude Sonnet 5.5 在数组 / 对象参数的文字里含有 ASCII 双引号（如 写给"明天的你"）时，会把整个参数写成 JSON 字符串，
// 而且字符串内部的引号没有转义（D-53）。直接解码失败时，把这类字符串参数还原为数组 / 对象（必要时先修复未转义的引号）
// 后再解码一次；仍然失败时返回第一次的错误。
func decodeArgs(raw jsonx.Value, v any) error {
	err := jsonx.Unmarshal(raw, v)
	if err == nil {
		return nil
	}
	var m map[string]any
	if jsonx.Unmarshal(raw, &m) != nil {
		return err
	}
	changed := false
	for k, val := range m {
		s, ok := val.(string)
		if !ok {
			continue
		}
		t := strings.TrimSpace(s)
		if !strings.HasPrefix(t, "[") && !strings.HasPrefix(t, "{") {
			continue
		}
		var inner any
		if jsonx.Unmarshal([]byte(t), &inner) == nil || jsonx.Unmarshal([]byte(repairQuotes(t)), &inner) == nil {
			m[k], changed = inner, true
		}
	}
	if !changed {
		return err
	}
	fixed, merr := jsonx.Marshal(m)
	if merr != nil || jsonx.Unmarshal(fixed, v) != nil {
		return err
	}
	return nil
}

// repairQuotes 转义 JSON 字符串值内部未转义的双引号。判断依据：字符串中的引号如果后面（跳过空白）紧跟
// , : } ] 或文本结尾，就是字符串的结束；否则是文字里的引号。文字里的引号恰好后跟 ASCII 逗号等时会判断错，
// 此时修复后仍无法解析，调用方照常报告参数错误。
func repairQuotes(s string) string {
	var b strings.Builder
	in, escaped := false, false
	for i := range len(s) {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case in && c == '\\':
			escaped = true
		case c == '"' && !in:
			in = true
		case c == '"' && in:
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\n' || s[j] == '\r' || s[j] == '\t') {
				j++
			}
			if j < len(s) && !strings.ContainsRune(",:}]", rune(s[j])) {
				b.WriteString(`\"`)
				continue
			}
			in = false
		}
		b.WriteByte(c)
	}
	return b.String()
}
