package runtime_test

// 测试作品的评测脚本（实施计划第 4 节）：用真实模型按 testdata/eval/*.yaml 的行动序列游玩一遍，
// 检查结构性结果（每回合都提交、工具调用成功、交还选项、状态有维护、面板正常渲染），
// 并在结束后以场外提问检查事实召回——答案写入报告，由人核对。会产生费用，默认跳过。
//
// 运行：
//
//	YANYI_EVAL=.local/secrets/live.json YANYI_EVAL_MODEL=claude@gemai go test ./internal/runtime -run TestEval -v -timeout 60m
//
// 可选：YANYI_EVAL_SCRIPT（默认 testdata/eval/misty-harbor.yaml）、YANYI_EVAL_OUT（报告目录，默认 .local/eval/<时间>）、
// YANYI_EVAL_TRACE（协议原始报文目录）、YANYI_EVAL_REASONING（默认 low）。live.json 的格式见 provider/live_test.go。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/provider"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/work"
)

type evalScript struct {
	Work  string         `yaml:"work"`
	Form  map[string]any `yaml:"form"`
	Steps []struct {
		Choice  string `yaml:"choice"`
		Free    string `yaml:"free"`
		Command string `yaml:"command"`
		Mode    string `yaml:"mode"`
		Text    string `yaml:"text"`
		OOC     string `yaml:"ooc"`
	} `yaml:"steps"`
	Checks []struct {
		Q      string `yaml:"q"`
		Expect string `yaml:"expect"`
	} `yaml:"checks"`
}

// evalModels 让存档使用 live.json 中指定的真实模型。
type evalModels struct {
	p llm.Provider
	m *llm.Model
}

func (e evalModels) Resolve(context.Context, string) (llm.Provider, *llm.Model, error) {
	return e.p, e.m, nil
}

// lazyBlobs 在环境装配好之后才知道 blob 存储（适配器先于环境创建）。
type lazyBlobs struct{ e **env }

func (l lazyBlobs) ReadBlob(ctx context.Context, hash string) ([]byte, error) {
	return (*l.e).blobs.Get(ctx, (*l.e).db.Read(), hash)
}

func TestEval(t *testing.T) {
	cfgPath, modelName := os.Getenv("YANYI_EVAL"), os.Getenv("YANYI_EVAL_MODEL")
	if cfgPath == "" || modelName == "" {
		t.Skip("评测：设置 YANYI_EVAL=<live.json> 与 YANYI_EVAL_MODEL=<名称> 后运行（会产生费用）")
	}
	scriptPath := envOr("YANYI_EVAL_SCRIPT", "../../testdata/eval/misty-harbor.yaml")
	var script evalScript
	readYAML(t, scriptPath, &script)

	var cfg struct {
		Providers []struct {
			Name     string       `json:"name"`
			Protocol llm.Protocol `json:"protocol"`
			BaseURL  string       `json:"base_url"`
			Key      string       `json:"key"`
			Model    string       `json:"model"`
		} `json:"providers"`
	}
	b, err := os.ReadFile(cfgPath) //nolint:gosec // G703：配置文件路径由开发者通过环境变量指定
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonx.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	var e *env
	var models evalModels
	for _, lp := range cfg.Providers {
		if lp.Name != modelName {
			continue
		}
		adapter, err := provider.NewAdapter(llm.ProviderConfig{Protocol: lp.Protocol, BaseURL: lp.BaseURL, APIKey: lp.Key,
			Blobs: lazyBlobs{&e}, TraceDir: os.Getenv("YANYI_EVAL_TRACE")})
		if err != nil {
			t.Fatal(err)
		}
		pre, ok := provider.PresetFor(lp.Protocol, lp.Model)
		if !ok {
			t.Fatalf("模型 %s 没有预设", lp.Model)
		}
		models = evalModels{
			p: llm.WithRetry(adapter, llm.RetryPolicy{MaxRetries: 3, BaseDelay: 3 * time.Second, MaxDelay: 30 * time.Second}),
			m: &llm.Model{ID: "eval", Name: lp.Model, DisplayName: lp.Name, ContextWindow: pre.ContextWindow, MaxOutput: pre.MaxOutput,
				Caps: pre.Caps, Price: pre.Price},
		}
	}
	if models.p == nil {
		t.Fatalf("live.json 中没有 %s", modelName)
	}

	e, err = openEnvWith(context.Background(), t.TempDir(), models)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })

	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: script.Work, Form: script.Form,
		Settings: save.Settings{NarratorModel: "eval", Pace: "medium", Reasoning: envOr("YANYI_EVAL_REASONING", "low"), CacheTTL: "short"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()

	out := envOr("YANYI_EVAL_OUT", filepath.Join("..", "..", ".local", "eval", time.Now().Format("20060102-150405")))
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	var story strings.Builder
	fmt.Fprintf(&story, "# 评测：%s（%s，推理 %s）\n\n", script.Work, models.m.Name, sv.Settings.Reasoning)

	w, _ := e.mgr.Presentation(ctx, sv.ID)
	stats := evalStats{tools: map[string]int{}}
	auto := 0
	play := func(label string, in save.TurnInput) string {
		t.Helper()
		if _, err := e.mgr.Submit(ctx, sv.ID, in); err != nil {
			t.Fatalf("%s：提交失败：%v", label, err)
		}
		toolErrors := 0
		for {
			ev := <-sub.C
			switch ev.Type {
			case event.TurnTool:
				d := ev.Data.(event.TurnToolData)
				switch d.Phase {
				case "error":
					toolErrors++
					stats.toolErrors = append(stats.toolErrors, fmt.Sprintf("%s：%s", label, d.Name))
				case "done":
					stats.tools[d.Name]++
				}
			case event.TurnFailed:
				d := ev.Data.(event.TurnFailedData)
				stats.failed = append(stats.failed, fmt.Sprintf("%s：%s", label, d.Message))
				return ""
			case event.TurnCommitted:
				tl, _ := e.saves.Timeline(ctx, sv.ID, "", 1)
				n := tl.Nodes[len(tl.Nodes)-1]
				stats.turns++
				var text strings.Builder
				for _, it := range n.Display {
					switch it.T {
					case "text":
						text.WriteString(it.Markup + "\n\n")
					case "choices":
						stats.withChoices++
						for _, c := range it.Items {
							fmt.Fprintf(&text, "- [%s] %s\n", c.ID, c.Label)
						}
					case "dice":
						fmt.Fprintf(&text, "（掷骰 %s = %d：%s）\n\n", it.Expr, it.Total, it.Text)
					case "notice":
						fmt.Fprintf(&text, "（提示：%s）\n\n", it.Text)
					}
				}
				fmt.Fprintf(&story, "## %s\n\n%s\n", label, text.String())
				t.Logf("%s → %s，工具错误 %d", label, n.Stop, toolErrors)
				return text.String()
			}
		}
	}

	for i, s := range script.Steps {
		label := fmt.Sprintf("第 %d 步", i+1)
		var in save.TurnInput
		switch {
		case s.Choice != "":
			id := s.Choice
			if id == "auto" {
				id = pickChoice(t, e, sv.ID, auto)
				auto++
			}
			in = save.TurnInput{Kind: save.InputChoice, ChoiceID: id}
			label += "（选择 " + id + "）"
		case s.Command != "":
			in = commandInput(t, w.UI.Commands, s.Command)
			label += "（" + s.Command + "）"
		case s.Mode != "":
			in = save.TurnInput{Kind: save.InputFree, Mode: modeCommand(w.UI.Commands, s.Mode), Text: s.Text}
			label += "（" + s.Mode + "：" + s.Text + "）"
		case s.OOC != "":
			in = save.TurnInput{Kind: save.InputOOC, Text: s.OOC}
			label += "（场外：" + s.OOC + "）"
		default:
			in = save.TurnInput{Kind: save.InputFree, Text: s.Free}
			label += "（" + s.Free + "）"
		}
		play(label, in)
	}

	// 事实召回：场外提问
	fmt.Fprintf(&story, "# 检查问题\n\n")
	for _, c := range script.Checks {
		answer := play("检查："+c.Q, save.TurnInput{Kind: save.InputOOC, Text: c.Q + "（请根据目前的故事简短回答）"})
		answer = strings.NewReplacer("<ooc>", "", "</ooc>", "", "\n", " ").Replace(answer)
		fmt.Fprintf(&story, "- 期望：%s\n- 回答：%s\n\n", c.Expect, strings.TrimSpace(answer))
	}

	st, _ := e.mgr.State(ctx, sv.ID)
	panels, _ := e.mgr.Panels(ctx, sv.ID)
	usage := usageSummary(t, e, sv.ID)
	fmt.Fprintf(&story, "# 结果\n\n- 回合：%d，交还选项 %d，失败 %v\n- 工具：%v，工具错误 %v\n- 用量：%s\n\n```json\n%s\n```\n",
		stats.turns, stats.withChoices, stats.failed, stats.tools, stats.toolErrors, usage, jsonx.MustMarshal(st.State))
	for _, p := range panels {
		if strings.Contains(p.HTML, "面板渲染失败") {
			stats.failed = append(stats.failed, "面板 "+p.ID+" 渲染失败")
		}
	}
	if err := os.WriteFile(filepath.Join(out, "story.md"), []byte(story.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("报告：%s；用量：%s", filepath.Join(out, "story.md"), usage)

	// 结构性检查
	if len(stats.failed) > 0 {
		t.Errorf("失败：%v", stats.failed)
	}
	if stats.tools["update_state"] == 0 {
		t.Error("整局都没有更新状态")
	}
	if steps := len(script.Steps); stats.withChoices*10 < steps*7 {
		t.Errorf("交还选项的回合偏少：%d / %d", stats.withChoices, steps)
	}
	if len(stats.toolErrors) > 3 {
		t.Errorf("工具错误偏多：%v", stats.toolErrors)
	}
}

type evalStats struct {
	turns, withChoices int
	tools              map[string]int
	toolErrors, failed []string
}

func readYAML(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// pickChoice 依次选择最新一组选项中的第 1、2、3 个（选项不足时取最后一个）。
func pickChoice(t *testing.T, e *env, saveID string, n int) string {
	t.Helper()
	tl, err := e.saves.Timeline(context.Background(), saveID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	disp := tl.Nodes[len(tl.Nodes)-1].Display
	for i := len(disp) - 1; i >= 0; i-- {
		if disp[i].T == "choices" && len(disp[i].Items) > 0 {
			return disp[i].Items[min(n%3, len(disp[i].Items)-1)].ID
		}
	}
	t.Fatal("上一回合没有给出选项，脚本中的 choice: auto 无法执行")
	return ""
}

// commandInput 把 "/follow 林舟" 解析为命令输入：参数按位置对应命令声明的参数，多余的文字作为附带输入。
func commandInput(t *testing.T, cmds []work.Command, text string) save.TurnInput {
	t.Helper()
	fields := strings.Fields(strings.TrimPrefix(text, "/"))
	for _, c := range cmds {
		if c.Name != fields[0] {
			continue
		}
		args := map[string]string{}
		rest := fields[1:]
		for _, a := range c.Args {
			if len(rest) == 0 {
				break
			}
			args[a.Name], rest = rest[0], rest[1:]
		}
		return save.TurnInput{Kind: save.InputCommand, Command: &save.CommandCall{Name: c.Name, Args: args}, Text: strings.Join(rest, " ")}
	}
	t.Fatalf("作品没有命令 %s", text)
	return save.TurnInput{}
}

// modeCommand 按输入模式的显示名找到命令名。
func modeCommand(cmds []work.Command, label string) string {
	for _, c := range cmds {
		if c.HasPresent("mode") && (c.ModeLabel == label || c.Title == label || c.Name == label) {
			return c.Name
		}
	}
	return label
}

// usageSummary 汇总存档的模型调用用量（usage_log）。
func usageSummary(t *testing.T, e *env, saveID string) string {
	t.Helper()
	var calls, uncached, read, write, output int64
	var cost *int64
	err := e.db.Read().QueryRowContext(context.Background(),
		`SELECT COUNT(*), COALESCE(SUM(input_uncached),0), COALESCE(SUM(cache_read),0), COALESCE(SUM(cache_write),0),
		        COALESCE(SUM(output),0), SUM(cost_micros) FROM usage_log WHERE save_id = ?`, saveID).
		Scan(&calls, &uncached, &read, &write, &output, &cost)
	if err != nil {
		t.Fatal(err)
	}
	s := fmt.Sprintf("%d 次调用；输入 未缓存 %d / 缓存读取 %d / 缓存写入 %d；输出 %d", calls, uncached, read, write, output)
	if cost != nil {
		s += fmt.Sprintf("；按官方价约 %.3f", float64(*cost)/1e6)
	}
	return s
}
