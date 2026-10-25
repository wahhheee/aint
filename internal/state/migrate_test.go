package state

import (
	"reflect"
	"testing"
)

func TestMigrate(t *testing.T) {
	schema, err := ParseSchema([]byte(`fields:
  hp: {type: number, default: 10}
  added: {type: string, default: 新字段}
  party:
    type: list
    items:
      type: object
      fields:
        name: string
        alive: {type: boolean, default: true}
  bag:
    type: map
    values: {type: number, default: 0}
`))
	if err != nil {
		t.Fatal(err)
	}
	old := map[string]any{"hp": "旧数值", "deleted": float64(2), "party": []any{map[string]any{"name": "小明", "old": "旧字段"}}, "bag": map[string]any{"apple": float64(3), "bad": "字符串"}}
	before := deepCopy(old)
	got, records := Migrate(schema, old)
	want := map[string]any{"hp": float64(10), "added": "新字段", "party": []any{map[string]any{"name": "小明", "alive": true}}, "bag": map[string]any{"apple": float64(3), "bad": float64(0)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("迁移状态：%#v", got)
	}
	if len(records) != 4 {
		t.Fatalf("迁移记录：%#v", records)
	}
	if !reflect.DeepEqual(old, before) {
		t.Fatal("迁移修改了旧分支状态")
	}
	empty, all := Migrate(nil, old)
	if len(empty) != 0 || len(all) != 4 {
		t.Fatal("移除状态定义也应归档全部旧值")
	}
}
