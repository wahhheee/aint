package runtime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/provider"
	"github.com/wahhheee/yanyi/internal/runtime"
	"github.com/wahhheee/yanyi/internal/save"
)

// TestM4Live 验证 T1/T3 短流程及循环换协议：每个起始协议两部作品，各两回合。
// 默认跳过；配置只选三个供应商。每回合最多四次调用、每次 2048 输出 token，不自动重试。
// YANYI_M4_LIVE=配置，YANYI_M4_OUT=报告目录；YANYI_M4_ONLY 可只跑一个起始协议。
func TestM4Live(t *testing.T) {
	path := os.Getenv("YANYI_M4_LIVE")
	if path == "" {
		t.Skip("设置 YANYI_M4_LIVE 后执行精简真实评测（会产生费用）")
	}
	var cfg struct {
		Providers []struct {
			Protocol llm.Protocol `json:"protocol"`
			BaseURL  string       `json:"base_url"`
			Key      string       `json:"key"`
			Model    string       `json:"model"`
		} `json:"providers"`
	}
	b, err := os.ReadFile(path) //nolint:gosec // G703：真实测试配置由开发者指定
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonx.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 3 {
		t.Fatal("精简评测配置须各协议一个供应商，共三个")
	}
	for i, source := range cfg.Providers {
		if only := os.Getenv("YANYI_M4_ONLY"); only != "" && only != string(source.Protocol) {
			continue
		}
		for _, workID := range []string{"yanyi.first-story", "yanyi.misty-harbor"} {
			if !t.Run(string(source.Protocol)+"/"+workID, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
				defer cancel()
				out := filepath.Join(envOr("YANYI_M4_OUT", "../../.local/eval/m4"), string(source.Protocol), workID)
				if err := os.MkdirAll(out, 0o750); err != nil {
					t.Fatal(err)
				}
				var e *env
				models := &evalModels{}
				selectModel := func(index int) {
					lp := cfg.Providers[index]
					p, err := provider.NewAdapter(llm.ProviderConfig{Protocol: lp.Protocol, BaseURL: lp.BaseURL, APIKey: lp.Key, Blobs: lazyBlobs{&e}, TraceDir: filepath.Join(out, "raw")})
					if err != nil {
						t.Fatal(err)
					}
					pre, ok := provider.PresetFor(lp.Protocol, lp.Model)
					if !ok {
						t.Fatal("缺少模型预设")
					}
					*models = evalModels{p: p, m: &llm.Model{ID: string(lp.Protocol), Name: lp.Model, ContextWindow: pre.ContextWindow, MaxOutput: 2048, Caps: pre.Caps, Price: pre.Price}}
				}
				selectModel(i)
				var err error
				e, err = openEnvWith(ctx, t.TempDir(), models, func(d *runtime.Deps) {
					d.Budget = runtime.Budget{MaxCalls: 4, MaxToolCalls: 15, MaxContinuations: 1, MaxOutputCap: 2048, WallClock: 2 * time.Minute}
				})
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = e.close() }()
				sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: workID, Settings: save.Settings{NarratorModel: "m4", Pace: "100-180", Reasoning: "off", CacheTTL: llm.CacheShort}})
				if err != nil {
					t.Fatal(err)
				}
				sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
				defer sub.Close()
				var report strings.Builder
				toolSuccess := map[string]int{}
				defer func() {
					if err := os.WriteFile(filepath.Join(out, "story.md"), []byte(report.String()), 0o600); err != nil {
						t.Error(err)
					}
				}()
				for turn := range 2 {
					input := save.TurnInput{Kind: save.InputFree, Text: "我检查眼前最重要的线索。请简短推进故事，然后用 offer_choices 给两个选项。"}
					if workID == "yanyi.misty-harbor" && turn == 0 {
						input.Text = "我检查邮局的信件柜。请先用 read 查看 /work/lore/characters/zhou.md，再用 update_state 把 situation 设为“正在检查信件柜”，简短叙述后用 offer_choices 给两个选项。"
					}
					if turn == 1 {
						selectModel((i + 1) % len(cfg.Providers))
						id := pickChoice(t, e, sv.ID, 0)
						input = save.TurnInput{Kind: save.InputContinue}
						if id != "" {
							input = save.TurnInput{Kind: save.InputChoice, ChoiceID: id}
						}
					}
					if _, err := e.mgr.Submit(ctx, sv.ID, input); err != nil {
						t.Fatal(err)
					}
					waiting := true
					for waiting {
						select {
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						case ev := <-sub.C:
							switch ev.Type {
							case event.TurnTool:
								d := ev.Data.(event.TurnToolData)
								fmt.Fprintf(&report, "工具 %s: %s\n\n", d.Name, d.Phase)
								if d.Phase == "done" {
									toolSuccess[d.Name]++
								}
								if d.Phase == "error" {
									t.Errorf("工具未执行：%s", d.Name)
								}
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
					fmt.Fprintf(&report, "## 回合 %d · %s · %s\n\n", turn+1, models.m.Name, n.Stop)
					for _, item := range n.Display {
						fmt.Fprintf(&report, "%s\n\n", jsonx.MustMarshal(item))
					}
					if n.Status == save.StatusInterrupted || (n.Stop != "end" && n.Stop != "choices") {
						t.Fatalf("回合未正常结束：%s %s", n.Status, n.Stop)
					}
				}
				panels, err := e.mgr.Panels(ctx, sv.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range panels {
					if strings.Contains(p.HTML, "面板渲染失败") {
						t.Fatal("面板渲染失败")
					}
				}
				if workID == "yanyi.misty-harbor" && (toolSuccess["read"] == 0 || toolSuccess["update_state"] == 0) {
					t.Error("T3 未完成资料读取或状态更新")
				}
				var rows []map[string]any
				q, err := e.db.Read().QueryContext(ctx, `SELECT model_id,input_uncached,cache_read,cache_write,output,reasoning,cost_micros FROM usage_log WHERE save_id=? ORDER BY id`, sv.ID)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = q.Close() }()
				for q.Next() {
					var model string
					var u llm.Usage
					var cost *int64
					if err := q.Scan(&model, &u.InputUncached, &u.CacheRead, &u.CacheWrite, &u.Output, &u.Reasoning, &cost); err != nil {
						t.Fatal(err)
					}
					rows = append(rows, map[string]any{"model": model, "usage": u, "cost_micros": cost})
				}
				if err := q.Err(); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, "usage.json"), jsonx.MustMarshal(rows), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Logf("%s → %s：%d 次调用，报告 %s", source.Protocol, models.p.Protocol(), len(rows), out)
			}) {
				return
			}
		}
	}
}
