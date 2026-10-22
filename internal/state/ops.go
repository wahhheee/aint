package state

import (
	"fmt"
	"reflect"
	"strings"
)

// Op 是 update_state 的一个操作（《02-技术方案》11.1）。
type Op struct {
	Op    string `json:"op"`   // set | inc | append | remove | delete
	Path  string `json:"path"` // a.b.c；映射键可为中文，不含 "."
	Value any    `json:"value,omitzero"`
}

// 操作名。
const (
	OpSet    = "set"
	OpInc    = "inc"
	OpAppend = "append"
	OpRemove = "remove"
	OpDelete = "delete"
)

// Change 记录一次实际发生的变化，用于工具结果文案（"day 2 → 3"）、检查器状态差异与 SSE 推送。
// Before / After 是整个路径处的值；append / remove 时 Value 是追加或移除的元素。
type Change struct {
	Op     string `json:"op"`
	Path   string `json:"path"`
	Value  any    `json:"value,omitzero"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// Apply 在 st 的副本上原子执行 ops：任一操作不成立则整体不执行，返回的 error 说明哪一个操作、为什么、可以怎么写。
// st 不会被修改。
//
// 语义：
//   - set：任意类型，值需匹配字段类型；inc：数字加减（值为增量）；
//   - append / remove：列表追加 / 按值移除；append 的值是数组且元素类型不是列表时，逐个追加；
//   - delete：删除映射键；其他路径恢复字段的默认值；
//   - 路径语法 a.b.c；映射可以新增键（值为对象时以该对象的默认值起步）；对象与顶层只能写已定义的字段；
//   - 不限制数值范围，不拒绝"不合情理"的变化（那属于故事本身，D-22）。
//
// error 文案直接作为工具结果给 AI（如"day 是数字字段，可以写成一个数字，如 3"），见 Check。
func Apply(s *Schema, st map[string]any, ops []Op) (next map[string]any, changes []Change, err error) {
	if s.Empty() {
		return nil, nil, fmt.Errorf("这部作品没有定义状态")
	}
	if len(ops) == 0 {
		return nil, nil, fmt.Errorf("ops 为空，可以写成 [{\"op\": \"set\", \"path\": \"%s\", \"value\": …}]", s.Order[0])
	}
	next = Clone(st)
	for i, op := range ops {
		c, err := apply(s, next, op)
		if err != nil {
			if len(ops) > 1 {
				return nil, nil, fmt.Errorf("第 %d 个操作（%s %s）：%w", i+1, op.Op, op.Path, err)
			}
			return nil, nil, err
		}
		changes = append(changes, c)
	}
	return next, changes, nil
}

func apply(s *Schema, st map[string]any, op Op) (Change, error) {
	path := strings.TrimSpace(op.Path)
	if path == "" {
		return Change{}, fmt.Errorf("缺少 path，可用的字段有：%s", strings.Join(s.Order, "、"))
	}
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return Change{}, fmt.Errorf("路径 %q 中有空的一段，可以写成 a.b 的形式", path)
		}
	}
	f, err := s.Lookup(path)
	if err != nil {
		return Change{}, err
	}
	parent, key, isMapKey, err := locate(s, st, path)
	if err != nil {
		return Change{}, err
	}
	before, exists := parent[key]
	c := Change{Op: op.Op, Path: path, Before: deepCopy(before)}
	v := normalize(op.Value)

	switch op.Op {
	case OpSet:
		if op.Value == nil {
			return c, fmt.Errorf("set 需要 value；想恢复默认值可以用 delete")
		}
		if err := check(f, path, v); err != nil {
			return c, err
		}
		parent[key] = fillDefaults(f, v)
	case OpInc:
		if f.Type != Number {
			return c, fmt.Errorf("%s 是%s字段，inc 只用于数字字段；可以改用 set", path, typeLabel(f.Type))
		}
		delta, ok := v.(float64)
		if !ok {
			return c, fmt.Errorf("inc 的 value 是增量，可以写成一个数字，如 1 或 -2")
		}
		cur, _ := before.(float64)
		if !exists {
			cur, _ = f.zeroOrDefault().(float64)
		}
		parent[key] = cur + delta
	case OpAppend, OpRemove:
		if f.Type != List {
			return c, fmt.Errorf("%s 是%s字段，%s 只用于列表字段；可以改用 set", path, typeLabel(f.Type), op.Op)
		}
		if op.Value == nil {
			return c, fmt.Errorf("%s 需要 value（列表元素）", op.Op)
		}
		items, _ := before.([]any)
		if !exists {
			items, _ = f.zeroOrDefault().([]any)
		}
		elems := []any{v}
		if arr, ok := v.([]any); ok && f.Items.Type != List {
			elems = arr
		}
		for _, e := range elems {
			if err := check(f.Items, path+" 的元素", e); err != nil {
				return c, err
			}
		}
		if op.Op == OpAppend {
			parent[key] = append(append([]any{}, items...), elems...)
		} else {
			out := append([]any{}, items...)
			for _, e := range elems {
				i := indexOf(out, e)
				if i < 0 {
					return c, fmt.Errorf("%s 中没有%s；现有的元素：%s", path, Format(e), formatList(items))
				}
				out = append(out[:i], out[i+1:]...)
			}
			parent[key] = out
		}
		c.Value = v
	case OpDelete:
		if isMapKey {
			if !exists {
				return c, fmt.Errorf("%s 中没有键 %s，不需要删除", parentName(path), key)
			}
			delete(parent, key)
		} else {
			parent[key] = f.zeroOrDefault()
		}
	default:
		return c, fmt.Errorf("没有操作 %q，可用的操作有 set、inc、append、remove、delete", op.Op)
	}
	if after, ok := parent[key]; ok {
		c.After = deepCopy(after)
	}
	return c, nil
}

// locate 找到 path 的父容器与最后一段键，沿途补齐缺失的对象与映射项。
// isMapKey 表示最后一段是映射的键（delete 会删除它，而不是恢复默认值）。
func locate(s *Schema, st map[string]any, path string) (parent map[string]any, key string, isMapKey bool, err error) {
	parts := strings.Split(path, ".")
	cur := st
	f := s.Fields[parts[0]]
	for i, part := range parts {
		if i == len(parts)-1 {
			return cur, part, i > 0 && isMapKey, nil
		}
		next, ok := cur[part].(map[string]any)
		if !ok {
			if v, present := cur[part]; present && v != nil {
				return nil, "", false, fmt.Errorf("%s 当前的值不是对象，可以先用 set 整体写入", strings.Join(parts[:i+1], "."))
			}
			if f.Type == Object { // 缺失的对象（含新增的映射项）以默认值起步，字段完整
				next = f.zero().(map[string]any)
			} else {
				next = map[string]any{}
			}
			cur[part] = next
		}
		cur = next
		// 下一段的字段定义：对象取子字段，映射取值类型
		switch f.Type {
		case Object:
			f, isMapKey = f.Fields[parts[i+1]], false
		case Map:
			f, isMapKey = f.Values, true
		}
	}
	return nil, "", false, fmt.Errorf("路径 %q 不完整", path)
}

// fillDefaults 为对象值补齐未写出的字段（取默认值）：整体 set 一个对象时，没写到的字段不会消失。
func fillDefaults(f *Field, v any) any {
	switch f.Type {
	case Object:
		m, _ := v.(map[string]any)
		for _, n := range f.Order {
			if sub, ok := m[n]; ok {
				m[n] = fillDefaults(f.Fields[n], sub)
			} else {
				m[n] = f.Fields[n].zeroOrDefault()
			}
		}
		return m
	case List:
		items, _ := v.([]any)
		for i, it := range items {
			items[i] = fillDefaults(f.Items, it)
		}
		return items
	case Map:
		m, _ := v.(map[string]any)
		for k, it := range m {
			m[k] = fillDefaults(f.Values, it)
		}
		return m
	}
	return v
}

func parentName(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[:i]
	}
	return path
}

func indexOf(items []any, v any) int {
	for i, it := range items {
		if reflect.DeepEqual(it, v) {
			return i
		}
	}
	return -1
}

func formatList(items []any) string {
	if len(items) == 0 {
		return "（空）"
	}
	parts := make([]string, 0, min(len(items), 8))
	for i, it := range items {
		if i == 8 {
			parts = append(parts, fmt.Sprintf("等 %d 项", len(items)))
			break
		}
		parts = append(parts, Format(it))
	}
	return strings.Join(parts, "、")
}
