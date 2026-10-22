package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/vfs"
	"github.com/wahhheee/yanyi/internal/work"
)

// fakeEnv 是测试用的工具环境。
type fakeEnv struct {
	w     *work.Model
	st    map[string]any
	cards []save.DisplayItem
}

func (e *fakeEnv) Features() prompts.FeatureSet {
	return prompts.FeatureSet{prompts.FState: true, prompts.FDice: true}
}
func (e *fakeEnv) Work() *work.Model              { return e.w }
func (e *fakeEnv) OfferChoices(save.DisplayItem)  {}
func (e *fakeEnv) ShowCard(item save.DisplayItem) { e.cards = append(e.cards, item) }
func (e *fakeEnv) VFS() vfs.FS                    { return nil }
func (e *fakeEnv) State() map[string]any          { return e.st }
func (e *fakeEnv) ModelImage(_ context.Context, blob string) (llm.ImageRef, error) {
	return llm.ImageRef{Blob: blob, MIME: "image/png"}, nil
}
func (e *fakeEnv) UpdateState(ops []state.Op) ([]state.Change, error) {
	next, changes, err := state.Apply(e.w.State, e.st, ops)
	if err == nil {
		e.st = next
	}
	return changes, err
}

func run(t *testing.T, h func(context.Context, Env, jsonx.Value) Result, env Env, args string) string {
	t.Helper()
	res := h(context.Background(), env, jsonx.Value(args))
	return resultText(&res)
}

func TestParseDice(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2d6+1", "2d6+1"}, {"d100", "1d100"}, {"D%", "1d100"}, {" 1d20 - 2 ", "1d20-2"}, {"-1+3d4", "-1+3d4"}, {"2d6+1d4+2", "2d6+1d4+2"},
	}
	for _, c := range cases {
		_, canon, err := parseDice(c.in)
		if err != nil || canon != c.want {
			t.Errorf("parseDice(%q) = %q, %v；期望 %q", c.in, canon, err, c.want)
		}
	}
	for _, bad := range []string{"", "3", "2d", "d1", "51d6", "2d6+", "2x6", "1d6+1d6+1d6+1d6+1d6+1d6+1d6+1d6+1"} {
		if _, _, err := parseDice(bad); err == nil {
			t.Errorf("parseDice(%q) 应报错", bad)
		}
	}
}

func TestRollDice(t *testing.T) {
	seq := []int{3, 5, 2}
	orig := rollDie
	rollDie = func(int) int { r := seq[0]; seq = seq[1:]; return r }
	defer func() { rollDie = orig }()

	env := &fakeEnv{}
	got := run(t, handleRollDice, env, `{"expr":"2d6+1d4-1","reason":"在雾里找路"}`)
	want := "# roll_dice 2d6+1d4-1\n2d6：3、5；1d4：2；加值 -1；合计 9"
	if got != want {
		t.Fatalf("结果：%q\n期望：%q", got, want)
	}
	if len(env.cards) != 1 {
		t.Fatal("应追加一张骰子卡片")
	}
	c := env.cards[0]
	if c.T != "dice" || c.Total != 9 || len(c.Rolls) != 3 || c.Text != "在雾里找路" || c.Expr != "2d6+1d4-1" {
		t.Fatalf("骰子卡片：%+v", c)
	}
	if got := run(t, handleRollDice, env, `{"expr":"100d6"}`); !strings.HasPrefix(got, "未执行：一次最多掷 50 颗骰子") {
		t.Fatalf("超限应说明可以拆开掷：%q", got)
	}
}

func TestStateHandlers(t *testing.T) {
	s, err := state.ParseSchema([]byte("fields:\n  day: {type: number, default: 1}\n  weather: {type: string, default: 雾}\n  trust: {type: map, values: number}\n"))
	if err != nil {
		t.Fatal(err)
	}
	env := &fakeEnv{w: &work.Model{State: s}, st: s.Defaults()}
	if got := run(t, handleGetState, env, `{}`); got != "# get_state\nday: 1\nweather: 雾\ntrust: {}" {
		t.Fatalf("get_state：%q", got)
	}
	for args, want := range map[string]string{
		`{"op":"set","path":"weather","value":"暴雨","reason":"入夜"}`: "# update_state\n已更新：weather 「雾」 → 「暴雨」",
		`{"op":"set","path":"trust.林舟","value":2}`:                 "# update_state\n已更新：trust.林舟 设为 2",
		`{"op":"inc","path":"day","value":1}`:                      "# update_state\n已更新：day 1 → 2",
		`{"op":"delete","path":"day"}`:                             "# update_state\n已更新：day 恢复默认值 1",
		`{"op":"append","path":"day","value":"x"}`:                 "未执行：day 是数字字段，append 只用于列表字段；可以改用 set。",
	} {
		if got := run(t, handleUpdateState, env, args); got != want {
			t.Errorf("update_state %s：%q", args, got)
		}
	}
	if got := run(t, handleGetState, env, `{"paths":["nope"]}`); !strings.HasPrefix(got, "未执行：没有状态字段 nope，可用的字段有：day、weather、trust") {
		t.Fatalf("未知路径：%q", got)
	}
	if got := run(t, handleUpdateState, env, `{"path":"day"}`); !strings.HasPrefix(got, "未执行：缺少 op") {
		t.Fatalf("缺少 op：%q", got)
	}
}

// 部分模型把数组参数写成 JSON 字符串：能还原的照常执行。
func TestDecodeStringifiedArgs(t *testing.T) {
	var a struct {
		Paths []string `json:"paths"`
		N     int      `json:"n"`
	}
	if err := decodeArgs(jsonx.Value(`{"paths":"[\"day\", \"weather\"]","n":2}`), &a); err != nil || len(a.Paths) != 2 || a.N != 2 {
		t.Fatalf("应还原字符串化的数组：%+v %v", a, err)
	}
	if err := decodeArgs(jsonx.Value(`{"paths":"[\"a\", 1"}`), &a); err == nil {
		t.Fatal("无法还原时应报告原来的错误")
	}

	// 实测样本（Claude Sonnet 5.5）：文字里的 ASCII 引号没有转义
	var c offerArgs
	raw := `{"choices":"[{\"id\": \"a\", \"label\": \"先把信给他看\", \"detail\": \"把手里那封写给\"明天的你\"的信递过去\"}, {\"id\": \"b\", \"label\": \"离开\"}]"}`
	if err := decodeArgs(jsonx.Value(raw), &c); err != nil || len(c.Choices) != 2 || c.Choices[0].Detail != `把手里那封写给"明天的你"的信递过去` {
		t.Fatalf("应修复未转义的引号：%+v %v", c, err)
	}
}
