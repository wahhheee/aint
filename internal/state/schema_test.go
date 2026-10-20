package state

import (
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

func TestParseSchemaRejectsBadDefault(t *testing.T) {
	_, err := ParseSchema([]byte("fields:\n  day: {type: number, default: 第一天}\n"))
	if err == nil {
		t.Fatal("默认值类型不符时应报错")
	}
}
