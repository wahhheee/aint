package runtime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/provider"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/usage"
)

// TestToolTextLive 用真实 DeepSeek 验证带叙事标签的开场、选项回填和状态维护。
// 默认跳过；AINT_TOOL_TEXT_LIVE 指向 live.json，AINT_TOOL_TEXT_OUT 指定报告目录。
// AINT_TOOL_TEXT_TURNS 控制回合数（默认六回合），AINT_TOOL_TEXT_PACE 控制篇幅。
// 每回合最多八次调用、单次 4000 token，不自动重试网络错误。
func TestToolTextLive(t *testing.T) {
	config := os.Getenv("AINT_TOOL_TEXT_LIVE")
	if config == "" {
		t.Skip("设置 AINT_TOOL_TEXT_LIVE 后执行真实工具纠正评测（会产生费用）")
	}
	turns, err := strconv.Atoi(envOr("AINT_TOOL_TEXT_TURNS", "6"))
	if err != nil || turns < 3 || turns > 12 {
		t.Fatal("AINT_TOOL_TEXT_TURNS 须为 3–12")
	}
	var cfg struct {
		Providers []struct {
			Protocol llm.Protocol `json:"protocol"`
			BaseURL  string       `json:"base_url"`
			Key      string       `json:"key"`
			Model    string       `json:"model"`
		} `json:"providers"`
	}
	b, err := os.ReadFile(config) //nolint:gosec // G703：评测配置由开发者通过环境变量指定
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonx.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, source := range cfg.Providers {
		if !strings.HasPrefix(source.Model, "deepseek-") {
			continue
		}
		for _, reasoning := range []string{"off", "low"} {
			t.Run(string(source.Protocol)+"/"+reasoning, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
				defer cancel()
				out := filepath.Join(envOr("AINT_TOOL_TEXT_OUT", "../../.local/eval/deepseek-tool-text/runtime"), string(source.Protocol), reasoning)
				var e *env
				p, err := provider.NewAdapter(llm.ProviderConfig{Protocol: source.Protocol, BaseURL: source.BaseURL, APIKey: source.Key, Blobs: lazyBlobs{&e}, TraceDir: filepath.Join(out, "raw")})
				if err != nil {
					t.Fatal(err)
				}
				pre, ok := provider.PresetFor(source.Protocol, source.Model)
				if !ok {
					t.Fatal("模型缺少预设")
				}
				var reqs []*llm.Request
				models := evalModels{p: recordingProvider{Provider: p, reqs: &reqs}, m: &llm.Model{ID: "tool-text", Name: source.Model, ContextWindow: pre.ContextWindow, MaxOutput: 4000, Caps: pre.Caps, Price: pre.Price}}
				e, err = openEnvWith(ctx, t.TempDir(), models, func(d *runtime.Deps) {
					d.Budget = runtime.Budget{MaxCalls: 8, MaxToolCalls: 30, MaxContinuations: 1, MaxOutputCap: 4000, WallClock: 90 * time.Second}
				})
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = e.close() }()
				sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor", OpeningID: "arriving", Settings: save.Settings{NarratorModel: "tool-text", Pace: envOr("AINT_TOOL_TEXT_PACE", "150-250"), Reasoning: reasoning}})
				if err != nil {
					t.Fatal(err)
				}
				sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
				defer sub.Close()
				var report strings.Builder
				stateUpdated := false
				defer func() {
					if err := os.WriteFile(filepath.Join(out, "story.md"), []byte(report.String()), 0o600); err != nil {
						t.Error(err)
					}
				}()
				var repairsByTurn []int
				for turn := range turns {
					requestStart := len(reqs)
					input := save.TurnInput{Kind: save.InputOpening}
					if turn > 0 {
						id := pickChoice(t, e, sv.ID, 0)
						input = save.TurnInput{Kind: save.InputChoice, ChoiceID: id}
						if id == "" {
							input = save.TurnInput{Kind: save.InputContinue}
						}
					}
					if turn == 2 {
						input = save.TurnInput{Kind: save.InputFree, Text: "我回邮局检查信件柜。请用 update_state 将 situation 设为“正在检查信件柜”，简短叙述后用 offer_choices 给两个选项。"}
					}
					id, err := e.mgr.Submit(ctx, sv.ID, input)
					if err != nil {
						t.Fatal(err)
					}
					var streamed strings.Builder
					waiting := true
					for waiting {
						select {
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						case ev := <-sub.C:
							switch ev.Type {
							case event.TurnTool:
								tool := ev.Data.(event.TurnToolData)
								if turn == 2 && tool.Name == "update_state" && tool.Phase == "done" {
									stateUpdated = true
								}
							case event.TurnText:
								streamed.WriteString(ev.Data.(event.TurnTextData).Delta)
							case event.TurnFailed:
								t.Fatalf("回合失败：%+v", ev.Data)
							case event.TurnCommitted:
								waiting = false
							}
						}
					}
					tl, err := e.saves.Timeline(ctx, sv.ID, "", 1)
					if err != nil {
						t.Fatal(err)
					}
					n := tl.Nodes[len(tl.Nodes)-1]
					fmt.Fprintf(&report, "## 回合 %d · %s\n\n%s\n\n", turn+1, n.Stop, streamed.String())
					if (n.Stop != "choices" && (turn == 2 || n.Stop != "end")) || n.Status != save.StatusCommitted {
						t.Fatalf("回合未成功交还选项：%s / %s", n.Stop, n.Status)
					}
					for _, name := range []string{"offer_choices", "update_state", "read", "write"} {
						if strings.Contains(streamed.String(), "<"+name) {
							t.Fatalf("玩家正文出现伪调用：%s", name)
						}
					}
					sum, err := usage.Summarize(ctx, e.db.Read(), usage.Filter{SaveID: sv.ID, NodeID: id})
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("回合 %d：%s，调用 %d 次，用量 %+v", turn+1, n.Stop, sum.Total.Calls, sum.Total.Usage)
					repairs := 0
					for _, req := range reqs[requestStart:] {
						if strings.Contains(req.Messages[len(req.Messages)-1].TextOf(), "上一条回复把工具") {
							repairs++
						}
					}
					repairsByTurn = append(repairsByTurn, repairs)
					for _, msg := range reqs[requestStart].Messages {
						if msg.Role == llm.RoleAssistant && (strings.Contains(msg.TextOf(), "<offer_choices") || strings.Contains(msg.TextOf(), "<update_state")) {
							t.Fatal("回合开始时仍包含历史伪调用")
						}
					}
					if turn == 2 {
						_, line, err := e.saves.ProgressLine(ctx, sv.ID)
						if err != nil {
							t.Fatal(err)
						}
						st, err := e.saves.LoadState(ctx, line[len(line)-1].StateHash)
						situation, _ := st["situation"].(string)
						if err != nil || !stateUpdated || !strings.Contains(situation, "信件柜") {
							t.Fatalf("状态未实际更新：%v / %v", st["situation"], err)
						}
					}
				}
				repairs := 0
				for _, req := range reqs {
					if strings.Contains(req.Messages[len(req.Messages)-1].TextOf(), "上一条回复把工具") {
						repairs++
					}
				}
				t.Logf("共 %d 次模型调用，%d 次纠正请求；逐回合纠正次数 %v", len(reqs), repairs, repairsByTurn)
			})
		}
	}
}
