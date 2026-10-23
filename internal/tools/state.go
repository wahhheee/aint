package tools

import (
	"context"
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
//	已更新：day 2 → 3

type getStateArgs struct {
	Paths []string `json:"paths"`
}

func handleGetState(_ context.Context, env Env, raw jsonx.Value) Result {
	var a getStateArgs
	if err := decodeArgs(raw, &a); err != nil {
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
	state.Op
	Reason string `json:"reason"`
}

func handleUpdateState(_ context.Context, env Env, raw jsonx.Value) Result {
	var a updateStateArgs
	if err := decodeArgs(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {\"op\": \"set\", \"path\": \"day\", \"value\": 2}。", err)
	}
	if a.Op.Op == "" {
		return NotExecuted("缺少 op，可以写成 {\"op\": \"set\", \"path\": \"day\", \"value\": 2}；有多项变化时多次调用。")
	}
	changes, err := env.UpdateState([]state.Op{a.Op})
	if err != nil {
		return NotExecuted("%v。", err)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		parts[i] = state.Describe(c)
	}
	return Text("# update_state\n已更新：%s", strings.Join(parts, "；"))
}
