// Package state 是作品状态：定义（state.yaml）、类型检查、update_state 操作。
//
// 定位（决策 D-22，《02-技术方案》11.1）：状态是 AI 维护的结构化记录，用来驱动面板显示、
// 帮助保持前后一致。平台只检查数据类型，不判断剧情能否发生；范围与含义写在 desc 里由 AI 把握。
//
// 值的内存表示与 JSON 一致：boolean→bool，number→float64，string→string，
// list→[]any，map/object→map[string]any。状态快照以确定性 JSON 存为 blob。
//
// 本包是纯逻辑，不依赖存储与运行时，便于单元测试。
package state

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
)

// Type 是六种字段类型之一。
type Type string

const (
	Boolean Type = "boolean"
	Number  Type = "number"
	String  Type = "string"
	List    Type = "list"
	Map     Type = "map"
	Object  Type = "object"
)

func (t Type) valid() bool {
	switch t {
	case Boolean, Number, String, List, Map, Object:
		return true
	}
	return false
}

// Field 是一个字段定义。
type Field struct {
	Name    string            `json:"name"`
	Type    Type              `json:"type"`
	Default any               `json:"default"`
	Desc    string            `json:"desc,omitzero"`
	Items   *Field            `json:"items,omitzero"`  // list 的元素类型
	Values  *Field            `json:"values,omitzero"` // map 的值类型
	Fields  map[string]*Field `json:"fields,omitzero"` // object 的字段
	Order   []string          `json:"order,omitzero"`  // object 字段的声明顺序
}

// Schema 是作品的状态定义。字段按声明顺序保存（提示词中"状态说明"按作者的顺序列出）。
type Schema struct {
	Fields map[string]*Field `json:"fields"`
	Order  []string          `json:"order"`
}

// Empty 表示作品没有定义状态（能力画像中的 state 特征为假，相关提示词与工具都会被裁掉）。
func (s *Schema) Empty() bool { return s == nil || len(s.Order) == 0 }

// ParseSchema 解析 state.yaml。
func ParseSchema(data []byte) (*Schema, error) {
	var root any
	if err := yaml.UnmarshalWithOptions(data, &root, yaml.UseOrderedMap()); err != nil {
		return nil, fmt.Errorf("%s", yaml.FormatError(err, false, true))
	}
	top, ok := root.(yaml.MapSlice)
	if !ok {
		return nil, fmt.Errorf("state.yaml 顶层应为映射，且包含 fields")
	}
	s := &Schema{Fields: map[string]*Field{}}
	for _, item := range top {
		if fmt.Sprint(item.Key) != "fields" {
			continue // 未知顶层键由校验器报告 W-UNKNOWN-FIELD
		}
		fields, ok := item.Value.(yaml.MapSlice)
		if !ok {
			return nil, fmt.Errorf("fields 应为映射")
		}
		for _, f := range fields {
			name := fmt.Sprint(f.Key)
			field, err := parseField(name, f.Value)
			if err != nil {
				return nil, err
			}
			s.Fields[name] = field
			s.Order = append(s.Order, name)
		}
	}
	return s, nil
}

func parseField(path string, v any) (*Field, error) {
	name := path[strings.LastIndex(path, ".")+1:]
	// 简写：items: string
	if t, ok := v.(string); ok {
		f := &Field{Name: name, Type: Type(t)}
		if !f.Type.valid() {
			return nil, fmt.Errorf("%s：未知类型 %q（可用：boolean、number、string、list、map、object）", path, t)
		}
		f.Default = zeroValue(f.Type)
		return f, nil
	}
	m, ok := v.(yaml.MapSlice)
	if !ok {
		return nil, fmt.Errorf("%s：字段定义应为映射或类型名", path)
	}
	f := &Field{Name: name}
	var rawDefault any
	hasDefault := false
	for _, it := range m {
		key := fmt.Sprint(it.Key)
		switch key {
		case "type":
			f.Type = Type(fmt.Sprint(it.Value))
		case "desc":
			f.Desc = fmt.Sprint(it.Value)
		case "default":
			rawDefault, hasDefault = it.Value, true
		case "items", "values":
			sub, err := parseField(path+"[]", it.Value)
			if err != nil {
				return nil, err
			}
			if key == "items" {
				f.Items = sub
			} else {
				f.Values = sub
			}
		case "fields":
			fields, ok := it.Value.(yaml.MapSlice)
			if !ok {
				return nil, fmt.Errorf("%s.fields 应为映射", path)
			}
			f.Fields = map[string]*Field{}
			for _, sf := range fields {
				sn := fmt.Sprint(sf.Key)
				sub, err := parseField(path+"."+sn, sf.Value)
				if err != nil {
					return nil, err
				}
				f.Fields[sn] = sub
				f.Order = append(f.Order, sn)
			}
		}
	}
	if !f.Type.valid() {
		return nil, fmt.Errorf("%s：未知类型 %q", path, f.Type)
	}
	// items / values 缺省为 string（《03-作品格式规范》8.1）
	if f.Type == List && f.Items == nil {
		f.Items = &Field{Name: "items", Type: String, Default: ""}
	}
	if f.Type == Map && f.Values == nil {
		f.Values = &Field{Name: "values", Type: String, Default: ""}
	}
	if hasDefault {
		d := normalize(rawDefault)
		if err := Check(f, d); err != nil {
			return nil, fmt.Errorf("%s 的默认值不合法：%w", path, err)
		}
		f.Default = d
	} else {
		f.Default = f.zero()
	}
	return f, nil
}

// Defaults 返回全部字段取默认值的初始状态。
func (s *Schema) Defaults() map[string]any {
	out := map[string]any{}
	if s == nil {
		return out
	}
	for _, name := range s.Order {
		out[name] = s.Fields[name].zeroOrDefault()
	}
	return out
}

func (f *Field) zeroOrDefault() any {
	if f.Type == Object && f.Default == nil {
		return f.zero()
	}
	return deepCopy(f.Default)
}

func (f *Field) zero() any {
	if f.Type == Object {
		m := map[string]any{}
		for _, n := range f.Order {
			m[n] = f.Fields[n].zeroOrDefault()
		}
		return m
	}
	return zeroValue(f.Type)
}

func zeroValue(t Type) any {
	switch t {
	case Boolean:
		return false
	case Number:
		return float64(0)
	case String:
		return ""
	case List:
		return []any{}
	case Map, Object:
		return map[string]any{}
	}
	return nil
}

// normalize 把 YAML 解码出的值转为 JSON 一致的表示（整数→float64，有序映射→map）。
func normalize(v any) any {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	case yaml.MapSlice:
		m := make(map[string]any, len(x))
		for _, it := range x {
			m[fmt.Sprint(it.Key)] = normalize(it.Value)
		}
		return m
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = normalize(val)
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalize(val)
		}
		return out
	}
	return v
}

func deepCopy(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = deepCopy(val)
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = deepCopy(val)
		}
		return out
	}
	return v
}
