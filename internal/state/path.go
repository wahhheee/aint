package state

import (
	"fmt"
	"strings"
)

// Lookup 按路径 a.b.c 找到字段定义；映射的键段使用 values 的定义。
func (s *Schema) Lookup(path string) (*Field, error) {
	parts := strings.Split(path, ".")
	f, ok := s.Fields[parts[0]]
	if !ok {
		return nil, fmt.Errorf("没有状态字段 %s，可用的字段有：%s", parts[0], strings.Join(s.Order, "、"))
	}
	for _, part := range parts[1:] {
		switch f.Type {
		case Object:
			sub, ok := f.Fields[part]
			if !ok {
				return nil, fmt.Errorf("%s 没有字段 %s，可用的字段有：%s", f.Name, part, strings.Join(f.Order, "、"))
			}
			f = sub
		case Map:
			f = f.Values
		default:
			return nil, fmt.Errorf("%s 是%s字段，没有下级字段", f.Name, typeLabel(f.Type))
		}
	}
	return f, nil
}

// Set 把 path 处的值设为 v（先检查类型）。st 被原地修改；调用方需要事务语义时先 Clone。
// 开局表单写入初始值、update_state 的 set 操作都用它。
func Set(s *Schema, st map[string]any, path string, v any) error {
	f, err := s.Lookup(path)
	if err != nil {
		return err
	}
	v = normalize(v)
	if err := Check(f, v); err != nil {
		return err
	}
	parts := strings.Split(path, ".")
	cur := st
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = v
	return nil
}

// Clone 深拷贝状态。
func Clone(st map[string]any) map[string]any {
	return deepCopy(st).(map[string]any)
}

func typeLabel(t Type) string {
	switch t {
	case Boolean:
		return "布尔"
	case Number:
		return "数字"
	case String:
		return "文本"
	case List:
		return "列表"
	case Map:
		return "映射"
	case Object:
		return "对象"
	}
	return string(t)
}
