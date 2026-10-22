package state

import (
	"reflect"
	"strings"
	"testing"
)

const opsYAML = `
fields:
  player:
    type: object
    fields:
      name:  {type: string, default: 沈遥}
      trait: {type: string, default: 观察}
  day:    {type: number, default: 1}
  alive:  {type: boolean, default: true}
  clues:  {type: list}
  scores: {type: list, items: number}
  trust:  {type: map, values: number, default: {林舟: 2}}
  people:
    type: map
    values:
      type: object
      fields:
        trust: {type: number}
        seen:  {type: boolean}
`

func opsSchema(t *testing.T) *Schema {
	t.Helper()
	s, err := ParseSchema([]byte(opsYAML))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestApplyOps(t *testing.T) {
	s := opsSchema(t)
	st := s.Defaults()
	next, changes, err := Apply(s, st, []Op{
		{Op: OpInc, Path: "day", Value: 1},
		{Op: OpSet, Path: "player.name", Value: "林远"},
		{Op: OpAppend, Path: "clues", Value: "柜门上的新划痕"},
		{Op: OpAppend, Path: "clues", Value: []any{"潮湿的脚印", "熄灭的灯"}},
		{Op: OpRemove, Path: "clues", Value: "潮湿的脚印"},
		{Op: OpSet, Path: "trust.老陈", Value: -1},
		{Op: OpInc, Path: "trust.林舟", Value: 3},
		{Op: OpInc, Path: "trust.新人", Value: 2},
		{Op: OpSet, Path: "people.林舟.trust", Value: 4},
		{Op: OpDelete, Path: "alive"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if st["day"] != float64(1) || len(st["clues"].([]any)) != 0 {
		t.Fatal("Apply 不应修改传入的状态")
	}
	want := map[string]any{
		"day":    float64(2),
		"alive":  true,
		"clues":  []any{"柜门上的新划痕", "熄灭的灯"},
		"scores": []any{},
		"player": map[string]any{"name": "林远", "trait": "观察"},
		"trust":  map[string]any{"林舟": float64(5), "老陈": float64(-1), "新人": float64(2)},
		"people": map[string]any{"林舟": map[string]any{"trust": float64(4), "seen": false}},
	}
	if !reflect.DeepEqual(next, want) {
		t.Fatalf("结果不对：\n得到 %#v\n期望 %#v", next, want)
	}
	if len(changes) != 10 || changes[0].Before != float64(1) || changes[0].After != float64(2) {
		t.Fatalf("变化记录不对：%#v", changes[0])
	}
	if changes[2].Value != "柜门上的新划痕" {
		t.Fatalf("append 应记录追加的元素：%#v", changes[2])
	}
}

func TestApplyIsAtomic(t *testing.T) {
	s := opsSchema(t)
	st := s.Defaults()
	_, _, err := Apply(s, st, []Op{
		{Op: OpInc, Path: "day", Value: 1},
		{Op: OpSet, Path: "day", Value: "第三天"},
	})
	if err == nil || !strings.Contains(err.Error(), "第 2 个操作") || !strings.Contains(err.Error(), "数字字段") {
		t.Fatalf("应整体失败并指出第 2 个操作的类型问题，得到 %v", err)
	}
	if st["day"] != float64(1) {
		t.Fatal("失败时不应修改状态")
	}
}

func TestApplyErrors(t *testing.T) {
	s := opsSchema(t)
	st := s.Defaults()
	cases := []struct {
		op   Op
		want string
	}{
		{Op{Op: OpSet, Path: "weather", Value: "雨"}, "没有状态字段 weather"},
		{Op{Op: OpSet, Path: "player.age", Value: 3}, "player 没有字段 age"},
		{Op{Op: OpInc, Path: "clues", Value: 1}, "inc 只用于数字字段"},
		{Op{Op: OpInc, Path: "day", Value: "1"}, "增量"},
		{Op{Op: OpAppend, Path: "day", Value: 1}, "append 只用于列表字段"},
		{Op{Op: OpAppend, Path: "scores", Value: "高"}, "数字字段"},
		{Op{Op: OpRemove, Path: "clues", Value: "不存在"}, "clues 中没有「不存在」"},
		{Op{Op: OpDelete, Path: "trust.无名"}, "没有键 无名"},
		{Op{Op: OpSet, Path: "day"}, "set 需要 value"},
		{Op{Op: "add", Path: "day", Value: 1}, "可用的操作有"},
		{Op{Op: OpSet, Path: "day.x", Value: 1}, "没有下级字段"},
		{Op{Op: OpSet, Path: "trust.a..b", Value: 1}, "空的一段"},
	}
	for _, c := range cases {
		_, _, err := Apply(s, st, []Op{c.op})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v：期望错误含 %q，得到 %v", c.op, c.want, err)
		}
	}
}

func TestApplyDeleteRestoresDefault(t *testing.T) {
	s := opsSchema(t)
	st := s.Defaults()
	st["trust"] = map[string]any{"林舟": float64(9), "老陈": float64(1)}
	next, _, err := Apply(s, st, []Op{{Op: OpDelete, Path: "trust.老陈"}, {Op: OpDelete, Path: "player"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next["trust"], map[string]any{"林舟": float64(9)}) {
		t.Fatalf("delete 映射键：%#v", next["trust"])
	}
	next, _, err = Apply(s, next, []Op{{Op: OpDelete, Path: "trust"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next["trust"], map[string]any{"林舟": float64(2)}) {
		t.Fatalf("delete 整个映射应恢复默认值：%#v", next["trust"])
	}
}

func TestSetObjectFillsDefaults(t *testing.T) {
	s := opsSchema(t)
	next, _, err := Apply(s, s.Defaults(), []Op{{Op: OpSet, Path: "player", Value: map[string]any{"name": "阿远"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next["player"], map[string]any{"name": "阿远", "trait": "观察"}) {
		t.Fatalf("整体 set 对象时应补齐未写出的字段：%#v", next["player"])
	}
}

func TestYAMLAndFormat(t *testing.T) {
	s := opsSchema(t)
	st := s.Defaults()
	st["trust"] = map[string]any{"老陈": float64(-1), "林舟": float64(2.5)}
	out, err := YAML(s, st, "day", "trust", "player.name")
	if err != nil {
		t.Fatal(err)
	}
	want := "day: 1\ntrust:\n  林舟: 2.5\n  老陈: -1\nplayer.name: 沈遥\n"
	if out != want {
		t.Fatalf("YAML：\n%s\n期望：\n%s", out, want)
	}
	if got := Format([]any{"a", float64(3)}); got != "[「a」、3]" {
		t.Fatalf("Format：%s", got)
	}
	if _, err := YAML(s, st, "nope"); err == nil {
		t.Fatal("未知路径应报错")
	}
}
