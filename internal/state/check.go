package state

import (
	"fmt"
	"strings"
)

// Check 检查 v 是否符合字段类型。只检查类型，不检查范围（决策 D-22）。
// 错误文案会直接作为 update_state 的"未执行"说明返回给 AI，所以写成"可以怎么写"的正向说法。
func Check(f *Field, v any) error { return check(f, f.Name, v) }

// check 以 label 称呼被检查的值（字段路径、"clues 的元素"、"trust[林舟]"）。
func check(f *Field, label string, v any) error {
	switch f.Type {
	case Boolean:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s 是布尔字段，可以写成 true 或 false", label)
		}
	case Number:
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("%s 是数字字段，可以写成一个数字，如 3", label)
		}
	case String:
		if _, ok := v.(string); !ok {
			return fmt.Errorf("%s 是文本字段，可以写成一段文字", label)
		}
	case List:
		items, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%s 是列表字段，可以写成数组", label)
		}
		for _, it := range items {
			if err := check(f.Items, label+" 的元素", it); err != nil {
				return err
			}
		}
	case Map:
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%s 是映射字段，可以写成对象", label)
		}
		for k, val := range m {
			if strings.Contains(k, ".") {
				return fmt.Errorf("%s 的键 %q 含有“.”，可以换一个不含点号的写法", label, k)
			}
			if err := check(f.Values, label+"["+k+"]", val); err != nil {
				return err
			}
		}
	case Object:
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%s 是对象字段，可以写成对象", label)
		}
		for k, val := range m {
			sub, ok := f.Fields[k]
			if !ok {
				return fmt.Errorf("%s 没有字段 %s，可用的字段有：%s", label, k, strings.Join(f.Order, "、"))
			}
			if err := check(sub, label+"."+k, val); err != nil {
				return err
			}
		}
	}
	return nil
}
