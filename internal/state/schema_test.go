package state

import (
	"reflect"
	"testing"
)

const exampleYAML = `
fields:
  player:
    type: object
    fields:
      name:  {type: string, default: 沈遥, desc: 玩家角色名}
      trait: {type: string, default: 观察}
  day:    {type: number, default: 1, desc: 故事内第几天}
  clues:  {type: list, desc: 线索}
  trust:  {type: map, values: number}
`

func TestParseSchemaAndDefaults(t *testing.T) {
	s, err := ParseSchema([]byte(exampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Order; len(got) != 4 || got[0] != "player" || got[3] != "trust" {
		t.Fatalf("字段应保持声明顺序，得到 %v", got)
	}
	d := s.Defaults()
	if d["day"] != float64(1) {
		t.Fatalf("day 默认值应为 1（float64），得到 %#v", d["day"])
	}
	if name := d["player"].(map[string]any)["name"]; name != "沈遥" {
		t.Fatalf("嵌套对象默认值不对：%#v", name)
	}
	if err := Check(s.Fields["trust"], map[string]any{"林舟": "很高"}); err == nil {
		t.Fatal("map 的值类型为 number，应拒绝字符串")
	}
}

func TestParseSchemaUnknownKeys(t *testing.T) {
	s, err := ParseSchema([]byte(`version: 1
fields:
  day: {type: number, defaults: 3, desc: 天数}
  player:
    type: object
    fields:
      name: {type: string, label: 名字}
  clues: {type: list, items: {type: string, max: 5}}
  trust: {type: map, values: number}
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []UnknownKey{{Key: "version"}, {Field: "day", Key: "defaults"}, {Field: "player.name", Key: "label"}, {Field: "clues[]", Key: "max"}}
	if !reflect.DeepEqual(s.Unknown, want) {
		t.Fatalf("未知键应按出现顺序记录路径，得到 %+v", s.Unknown)
	}
	if s.Defaults()["day"] != float64(0) {
		t.Fatal("拼错的 default 不生效，取类型的零值")
	}
	if s, _ := ParseSchema([]byte(exampleYAML)); len(s.Unknown) != 0 {
		t.Fatalf("合法定义不应有未知键：%+v", s.Unknown)
	}
}

func TestParseSchemaRejectsBadDefault(t *testing.T) {
	_, err := ParseSchema([]byte("fields:\n  day: {type: number, default: 第一天}\n"))
	if err == nil {
		t.Fatal("默认值类型不符时应报错")
	}
}
