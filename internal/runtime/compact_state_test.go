package runtime

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/state"
	"github.com/wahhheee/aint/internal/work"
)

func TestCompactionStateRebase(t *testing.T) {
	schema, err := state.ParseSchema([]byte(`fields:
  gold: {type: number, default: 10}
  tasks: {type: list, items: string, default: [找钥匙]}
  person:
    type: object
    fields:
      mood: {type: string, default: 警惕}
      goal: {type: string, default: 回家}
  trust: {type: map, values: number, default: {林舟: 2}}
`))
	if err != nil {
		t.Fatal(err)
	}
	base := schema.Defaults()
	draft, pending, err := state.Apply(schema, base, []state.Op{
		{Op: state.OpInc, Path: "gold", Value: -2},
		{Op: state.OpAppend, Path: "tasks", Value: "还信"},
		{Op: state.OpSet, Path: "person.mood", Value: "安心"},
		{Op: state.OpDelete, Path: "trust.林舟"},
	})
	if err != nil {
		t.Fatal(err)
	}
	env := &compactEnv{work: &work.Model{State: schema}, state: base, pending: pending}
	for _, op := range []state.Op{
		{Op: state.OpSet, Path: " gold ", Value: 0},
		{Op: state.OpRemove, Path: "tasks", Value: "找钥匙"},
		{Op: state.OpSet, Path: "person", Value: map[string]any{"mood": "生气", "goal": "离开"}},
		{Op: state.OpSet, Path: "trust.林舟", Value: 3},
	} {
		if _, err := env.UpdateState([]state.Op{op}); err == nil || !strings.Contains(err.Error(), "重叠") {
			t.Fatalf("整理覆盖了本回合的写入：%+v %v", op, err)
		}
	}
	if _, err := env.UpdateState([]state.Op{{Op: state.OpSet, Path: "person.goal", Value: "寻找妹妹"}, {Op: state.OpSet, Path: "trust.周看守", Value: 1}}); err != nil {
		t.Fatal(err)
	}
	merged, err := rebaseTurnState(schema, env.state, pending)
	if err != nil {
		t.Fatal(err)
	}
	want := state.Clone(draft)
	want["person"].(map[string]any)["goal"] = "寻找妹妹"
	want["trust"].(map[string]any)["周看守"] = float64(1)
	if !reflect.DeepEqual(merged, want) || !reflect.DeepEqual(base, schema.Defaults()) {
		t.Fatalf("合并应保留双方的独立修改，不重复增减或追加：%v", merged)
	}
	env.pending = []state.Change{{Path: "person"}}
	if _, err := env.UpdateState([]state.Op{{Op: state.OpSet, Path: "person.goal", Value: "离开"}}); err == nil {
		t.Fatal("本回合写入整个对象后，整理不应修改其子字段")
	}
}
