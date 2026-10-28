package runtime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/provider"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
)

// TestGeminiOpeningLive 验证静态开场作答、工具往返与下一轮选项作答。
// AINT_GEMINI_LIVE 指向仅含一个 Gemini 供应商的私有配置，默认跳过；两回合各至多六次调用，不自动重试。
func TestGeminiOpeningLive(t *testing.T) {
	config := os.Getenv("AINT_GEMINI_LIVE")
	if config == "" {
		t.Skip("设置 AINT_GEMINI_LIVE 后执行开场选项真实验证（会产生费用）")
	}
	var cfg struct {
		Providers []struct {
			Protocol llm.Protocol `json:"protocol"`
			BaseURL  string       `json:"base_url"`
			Key      string       `json:"key"`
			Model    string       `json:"model"`
		} `json:"providers"`
	}
	b, err := os.ReadFile(config) //nolint:gosec // G703：真实验证配置由开发者通过环境变量指定
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonx.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Protocol != llm.ProtocolGemini {
		t.Fatal("配置必须恰好包含一个 Gemini 供应商")
	}
	source := cfg.Providers[0]
	out := envOr("AINT_GEMINI_OUT", "../../.local/eval/gemini-opening")
	var e *env
	p, err := provider.NewAdapter(llm.ProviderConfig{Protocol: source.Protocol, BaseURL: source.BaseURL, APIKey: source.Key,
		Blobs: lazyBlobs{&e}, TraceDir: filepath.Join(out, "raw")})
	if err != nil {
		t.Fatal(err)
	}
	pre, ok := provider.PresetFor(source.Protocol, source.Model)
	if !ok {
		t.Fatal("缺少模型预设")
	}
	models := evalModels{p: p, m: &llm.Model{ID: "gemini-opening", Name: source.Model, ContextWindow: pre.ContextWindow, MaxOutput: 2048, Caps: pre.Caps}}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	e, err = openEnvWith(ctx, t.TempDir(), models, func(d *runtime.Deps) {
		d.Budget = runtime.Budget{MaxCalls: 6, MaxToolCalls: 20, MaxContinuations: 1, MaxOutputCap: 2048, WallClock: 2 * time.Minute}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.close() }()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor", Settings: save.Settings{Pace: "100-180", Reasoning: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	var report strings.Builder
	defer func() {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(filepath.Join(out, "story.md"), []byte(report.String()), 0600); err != nil {
			t.Error(err)
		}
	}()
	for step := range 2 {
		choice := pickChoice(t, e, sv.ID, 0)
		if choice == "" {
			t.Fatal("缺少可作答选项")
		}
		if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: choice}); err != nil {
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
					fmt.Fprintf(&report, "工具 %s：%s\n\n", d.Name, d.Phase)
					if d.Phase == "error" {
						t.Errorf("工具执行失败：%s", d.Name)
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
		fmt.Fprintf(&report, "## 第 %d 次选项作答\n\n%s\n\n", step+1, jsonx.MustMarshal(n))
		if n.Status != save.StatusCommitted || n.Stop != "choices" {
			t.Fatalf("回合未正常交还选项：%s %s", n.Status, n.Stop)
		}
		t.Logf("第 %d 次选项作答：%s；累计用量 %s", step+1, n.Stop, usageSummary(t, e, sv.ID))
	}
}
