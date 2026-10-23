package contextx

import (
	"testing"

	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/work"
)

// 状态说明：object 子字段、list / map 中 object 元素的字段与元素本身的说明都列出来；简单类型的元素不多占一行。
func TestRenderStateDesc(t *testing.T) {
	s, err := state.ParseSchema([]byte(`
fields:
  player:
    type: object
    fields:
      name: {type: string, desc: 玩家名}
  clues: {type: list, desc: 线索}
  trust: {type: map, values: number, desc: 信任}
  items:
    type: list
    desc: 随身物品
    items:
      type: object
      fields:
        name: {type: string, desc: 物品名}
        meta:
          type: object
          fields:
            worn: {type: boolean, desc: 是否穿戴}
  npcs:
    type: map
    desc: 人物近况
    values:
      type: object
      desc: 一个人物
      fields:
        mood: {type: string, desc: 心情}
  notes: {type: list, items: {type: string, desc: 一句话}, desc: 笔记}
`))
	if err != nil {
		t.Fatal(err)
	}
	got := renderStateDesc(&work.Model{State: s}, nil)
	want := `- player（对象）
  - player.name（文本）：玩家名
- clues（列表：文本）：线索
- trust（映射 → 数字）：信任
- items（列表：对象）：随身物品
  - items 中每项的 name（文本）：物品名
  - items 中每项的 meta（对象）
    - items 中每项的 meta.worn（布尔）：是否穿戴
- npcs（映射 → 对象）：人物近况
  - npcs.<键>（对象）：一个人物
    - npcs.<键>.mood（文本）：心情
- notes（列表：文本）：笔记
  - notes 中每项（文本）：一句话`
	if got != want {
		t.Fatalf("状态说明不符：\n%s\n\n期望：\n%s", got, want)
	}
}
