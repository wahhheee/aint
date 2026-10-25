package state

import (
	"fmt"
	"maps"
	"slices"
)

// MigrationValue 保存已删除或类型不兼容的旧值，仅用于迁移记录，不参与主笔状态。
type MigrationValue struct {
	Path   string `json:"path"`
	Value  any    `json:"value"`
	Reason string `json:"reason"`
}

// Migrate 按目标定义递归补默认值、保留兼容值，并归档无法沿用的值；不修改输入。
func Migrate(schema *Schema, old map[string]any) (map[string]any, []MigrationValue) {
	var records []MigrationValue
	var fields map[string]*Field
	if schema != nil {
		fields = schema.Fields
	}
	return migrateObject(fields, old, "", &records), records
}

func migrateObject(fields map[string]*Field, old map[string]any, prefix string, records *[]MigrationValue) map[string]any {
	out := map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		f := fields[name]
		p := prefix + name
		if value, ok := old[name]; ok {
			out[name] = migrateValue(f, value, p, records)
		} else {
			out[name] = f.zeroOrDefault()
		}
	}
	for _, name := range slices.Sorted(maps.Keys(old)) {
		if _, ok := fields[name]; !ok {
			*records = append(*records, MigrationValue{Path: prefix + name, Value: deepCopy(old[name]), Reason: "字段已删除"})
		}
	}
	return out
}

func migrateValue(f *Field, value any, p string, records *[]MigrationValue) any {
	switch f.Type {
	case Object:
		if values, ok := value.(map[string]any); ok {
			return migrateObject(f.Fields, values, p+".", records)
		}
	case List:
		if values, ok := value.([]any); ok {
			out := make([]any, len(values))
			for i, v := range values {
				out[i] = migrateValue(f.Items, v, fmt.Sprintf("%s[%d]", p, i), records)
			}
			return out
		}
	case Map:
		if values, ok := value.(map[string]any); ok {
			out := map[string]any{}
			for _, key := range slices.Sorted(maps.Keys(values)) {
				out[key] = migrateValue(f.Values, values[key], p+"."+key, records)
			}
			return out
		}
	default:
		if Check(f, value) == nil {
			return deepCopy(value)
		}
	}
	*records = append(*records, MigrationValue{Path: p, Value: deepCopy(value), Reason: "类型不兼容"})
	return f.zeroOrDefault()
}
