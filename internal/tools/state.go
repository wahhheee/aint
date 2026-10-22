package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/state"
)

// get_state / update_state 的结果格式（《02-技术方案》10.2）：
//
//	# get_state
//	day: 3
//	clues:
//	  - 柜门上的新划痕
//
//	# update_state
//	已更新：day 2 → 3；clues 追加「柜门上的新划痕」

type getStateArgs struct {
	Paths []string `json:"paths"`
}

func handleGetState(_ context.Context, env Env, raw jsonx.Value) Result {
	var a getStateArgs
	if err := jsonx.Unmarshal(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {} 读取全部字段，或 {\"paths\": [\"day\"]}。", err)
	}
	var paths []string
	for _, p := range a.Paths {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	out, err := state.YAML(env.Work().State, env.State(), paths...)
	if err != nil {
		return NotExecuted("%v。", err)
	}
	return Text("# get_state\n%s", strings.TrimRight(out, "\n"))
}

type updateStateArgs struct {
	Ops    []state.Op `json:"ops"`
	Reason string     `json:"reason"`
}

func handleUpdateState(_ context.Context, env Env, raw jsonx.Value) Result {
	var a updateStateArgs
	if err := jsonx.Unmarshal(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {\"ops\": [{\"op\": \"set\", \"path\": \"day\", \"value\": 2}]}。", err)
	}
	changes, err := env.UpdateState(a.Ops)
	if err != nil {
		return NotExecuted("%v。", err)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		parts[i] = describeChange(c)
	}
	return Text("# update_state\n已更新：%s", strings.Join(parts, "；"))
}

// describeChange 把一次变化写成一句话："day 2 → 3"、"clues 追加「……」"。
func describeChange(c state.Change) string {
	switch c.Op {
	case state.OpAppend:
		return fmt.Sprintf("%s 追加%s", c.Path, listElems(c.Value))
	case state.OpRemove:
		return fmt.Sprintf("%s 移除%s", c.Path, listElems(c.Value))
	case state.OpDelete:
		if c.After == nil {
			return fmt.Sprintf("删除 %s（原为 %s）", c.Path, state.Format(c.Before))
		}
		return fmt.Sprintf("%s 恢复默认值 %s", c.Path, state.Format(c.After))
	}
	if c.Before == nil {
		return fmt.Sprintf("%s 设为 %s", c.Path, state.Format(c.After))
	}
	return fmt.Sprintf("%s %s → %s", c.Path, state.Format(c.Before), state.Format(c.After))
}

// listElems 写出 append / remove 的元素：单个元素直接写，数组逐个列出。
func listElems(v any) string {
	if arr, ok := v.([]any); ok {
		parts := make([]string, len(arr))
		for i, e := range arr {
			parts[i] = state.Format(e)
		}
		return strings.Join(parts, "、")
	}
	return state.Format(v)
}
