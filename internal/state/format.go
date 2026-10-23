package state

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// YAML 把状态渲染为给 AI 看的 YAML：字段按声明顺序，映射按键排序，整数不带小数点。
// 结果字节稳定（阶段块属于前缀，同一状态必须渲染出同样的文本）。
// paths 为空时输出全部字段，否则只输出这些路径（键为路径本身）。
func YAML(s *Schema, st map[string]any, paths ...string) (string, error) {
	var out yaml.MapSlice
	if len(paths) == 0 {
		for _, name := range s.Order {
			out = append(out, yaml.MapItem{Key: name, Value: orderedValue(s.Fields[name], st[name])})
		}
	} else {
		for _, p := range paths {
			f, err := s.Lookup(p)
			if err != nil {
				return "", err
			}
			v, _ := Get(st, p)
			if v == nil {
				v = f.zeroOrDefault()
			}
			out = append(out, yaml.MapItem{Key: p, Value: orderedValue(f, v)})
		}
	}
	b, err := yaml.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Get 读取路径处的值；路径不存在时 ok 为假。
func Get(st map[string]any, path string) (v any, ok bool) {
	var cur any = st
	for _, part := range strings.Split(path, ".") {
		m, isMap := cur.(map[string]any)
		if !isMap {
			return nil, false
		}
		if cur, ok = m[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func orderedValue(f *Field, v any) any {
	if f.Type != Object {
		return plainValue(v)
	}
	m, _ := v.(map[string]any)
	var out yaml.MapSlice
	for _, name := range f.Order {
		out = append(out, yaml.MapItem{Key: name, Value: orderedValue(f.Fields[name], m[name])})
	}
	return out
}

// plainValue 让输出稳定且易读：映射按键排序（不依赖 YAML 库的行为），整数值不带小数点。
func plainValue(v any) any {
	switch x := v.(type) {
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return int64(x)
		}
	case []any:
		out := make([]any, len(x))
		for i, it := range x {
			out[i] = plainValue(it)
		}
		return out
	case map[string]any:
		var out yaml.MapSlice
		for _, k := range slices.Sorted(maps.Keys(x)) {
			out = append(out, yaml.MapItem{Key: k, Value: plainValue(x[k])})
		}
		return out
	}
	return v
}

// maxFormat 是 Format 输出的长度上限（字符），超出部分以"…"代替。
const maxFormat = 80

// Format 把一个值写成一行简短的文字（工具结果中的"day 2 → 3"、"追加「柜门上的新划痕」"）：
// 文本加「」，整数不带小数点，列表与映射紧凑书写。
func Format(v any) string {
	s := format(v)
	if r := []rune(s); len(r) > maxFormat {
		s = string(r[:maxFormat]) + "…"
	}
	return s
}

func format(v any) string {
	switch x := v.(type) {
	case nil:
		return "（空）"
	case string:
		return "「" + x + "」"
	case bool:
		return strconv.FormatBool(x)
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, len(x))
		for i, it := range x {
			parts[i] = format(it)
		}
		return "[" + strings.Join(parts, "、") + "]"
	case map[string]any:
		keys := slices.Sorted(maps.Keys(x))
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ": " + format(x[k])
		}
		return "{" + strings.Join(parts, "，") + "}"
	}
	return fmt.Sprint(v)
}

// Describe 把一次变化写成一句话："day 2 → 3"、"clues 追加「……」"。
func Describe(c Change) string {
	switch c.Op {
	case OpAppend:
		return fmt.Sprintf("%s 追加%s", c.Path, listElems(c.Value))
	case OpRemove:
		return fmt.Sprintf("%s 移除%s", c.Path, listElems(c.Value))
	case OpDelete:
		if c.After == nil {
			return fmt.Sprintf("删除 %s（原为 %s）", c.Path, Format(c.Before))
		}
		return fmt.Sprintf("%s 恢复默认值 %s", c.Path, Format(c.After))
	}
	if c.Before == nil {
		return fmt.Sprintf("%s 设为 %s", c.Path, Format(c.After))
	}
	return fmt.Sprintf("%s %s → %s", c.Path, Format(c.Before), Format(c.After))
}

// listElems 写出 append / remove 的元素：单个元素直接写，数组逐个列出。
func listElems(v any) string {
	if arr, ok := v.([]any); ok {
		parts := make([]string, len(arr))
		for i, e := range arr {
			parts[i] = Format(e)
		}
		return strings.Join(parts, "、")
	}
	return Format(v)
}
