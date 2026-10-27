package runtime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// TestOperationLive 用 DeepSeek 检查连续重写、状态记忆、补选项及其后的续写。
// AINT_OPERATION_LIVE 指向私有配置，默认跳过；每个组合最多四步，每步八次调用、120 秒。
func TestOperationLive(t *testing.T) {
	config := os.Getenv("AINT_OPERATION_LIVE")
	if config == "" {
		t.Skip("设置 AINT_OPERATION_LIVE 后执行真实重写与选项评测（会产生费用）")
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
				ctx, cancel := context.WithTimeout(t.Context(), 9*time.Minute)
				defer cancel()
				out := filepath.Join(envOr("AINT_OPERATION_OUT", "../../.local/eval/turn-operations"), string(source.Protocol), reasoning)
				var e *env
				p, err := provider.NewAdapter(llm.ProviderConfig{Protocol: source.Protocol, BaseURL: source.BaseURL, APIKey: source.Key, Blobs: lazyBlobs{&e}, TraceDir: filepath.Join(out, "raw")})
				if err != nil {
					t.Fatal(err)
				}
				preset, ok := provider.PresetFor(source.Protocol, source.Model)
				if !ok {
					t.Fatal("缺少模型预设")
				}
				var reqs []*llm.Request
				var replies []operationAuditReply
				models := evalModels{p: recordingProvider{Provider: operationAuditProvider{Provider: p, replies: &replies}, reqs: &reqs}, m: &llm.Model{ID: "operation-live", Name: source.Model, ContextWindow: preset.ContextWindow, MaxOutput: 4000, Caps: preset.Caps, Price: preset.Price}}
				e, err = openEnvWith(ctx, t.TempDir(), models, func(d *runtime.Deps) {
					d.Budget = runtime.Budget{MaxCalls: 8, MaxToolCalls: 30, MaxContinuations: 1, MaxOutputCap: 4000, WallClock: 120 * time.Second}
				})
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = e.close() }()
				sv, root := operationSave(t, e)
				if err := e.saves.UpdateSettings(ctx, sv.ID, save.Settings{NarratorModel: "operation-live", Pace: "150-250", Reasoning: reasoning}); err != nil {
					t.Fatal(err)
				}
				original, err := e.saves.CommitTurn(ctx, save.TurnCommit{
					SaveID: sv.ID, Parent: root, TurnNo: 1, Input: save.TurnInput{Kind: save.InputFree, Text: "我看看摊位上的伞。"},
					Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("<input kind=\"free\">我看看摊位上的伞。</input>")}}, {Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text("你花两枚金币买下一把红伞。摊主收起钱，你握紧伞柄，站在雨夜的集市边。")}}},
					Display:  []save.DisplayItem{{T: "input", Text: "我看看摊位上的伞。", Kind: save.InputFree}, {T: "text", Markup: "你花两枚金币买下一把红伞。摊主收起钱，你握紧伞柄，站在雨夜的集市边。"}},
					State:    map[string]any{"gold": 8}, Memory: &save.MemoryWrite{Files: map[string]save.MemoryFile{"/memory/purchase.md": {Turn: 1}}, Data: map[string][]byte{"/memory/purchase.md": []byte("玩家花两枚金币买下红伞。")}},
					WorkVersionID: root.WorkVersionID, Status: save.StatusCommitted, Meta: save.NodeMeta{Stop: "end"},
				})
				if err != nil {
					t.Fatal(err)
				}
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
				sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
				defer sub.Close()
				target := original.ID
				for step := range 4 {
					start := len(reqs)
					replyStart := len(replies)
					current, _, _ := e.saves.ProgressLine(ctx, sv.ID)
					var id string
					switch step {
					case 0, 1:
						instructions := "把原文里的红伞改成蓝伞，价格改为三枚金币。保留雨夜集市的场景，写清购买过程，并同步金币和购买记忆。这轮只写正文，暂时不要选项。"
						if step == 1 {
							instructions = "保留买蓝伞和三枚金币的价格，让对话更含蓄，多写雨声。仍是重写同一次购买，并同步金币和购买记忆。这轮只写正文，暂时不要选项。"
						}
						id, err = e.mgr.Operate(ctx, sv.ID, save.TurnOperation{Kind: "rewrite", NodeID: target, ExpectedHead: current.HeadNodeID, Instructions: instructions})
					case 2:
						id, err = e.mgr.Operate(ctx, sv.ID, save.TurnOperation{Kind: "choices", NodeID: target, ExpectedHead: current.HeadNodeID})
					case 3:
						tl, xerr := e.saves.Timeline(ctx, sv.ID, "", 1)
						if xerr != nil {
							t.Fatal(xerr)
						}
						var choice string
						for _, it := range tl.Nodes[0].Display {
							if it.T == "choices" && len(it.Items) > 0 {
								choice = it.Items[0].ID
							}
						}
						if choice == "" {
							t.Fatal("没有可用的补充选项")
						}
						id, err = e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: choice})
					}
					if err != nil {
						t.Fatal(err)
					}
					waiting := true
					for waiting {
						select {
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						case ev := <-sub.C:
							switch ev.Type {
							case event.TurnFailed:
								t.Fatalf("第 %d 步失败：%+v", step+1, ev.Data)
							case event.TurnCommitted:
								waiting = false
							}
						}
					}
					n, err := e.saves.Node(ctx, sv.ID, id)
					if err != nil {
						t.Fatal(err)
					}
					items, _ := e.saves.LoadDisplay(ctx, n.DisplayHash)
					fmt.Fprintf(&report, "## 第 %d 步\n\n", step+1)
					var visible strings.Builder
					for _, it := range items {
						if it.T == "text" {
							visible.WriteString(it.Markup)
						}
						if it.T == "choices" {
							visible.WriteString(it.Prompt)
							for _, c := range it.Items {
								fmt.Fprintf(&visible, "\n- %s %s", c.Label, c.Detail)
							}
						}
					}
					fmt.Fprintf(&report, "%s\n\n", visible.String())
					if repeatedParagraph(visible.String()) {
						t.Errorf("第 %d 步重复输出正文", step+1)
					}
					if immersionLeak.MatchString(visible.String()) {
						t.Errorf("第 %d 步出现操作旁白：%s", step+1, visible.String())
					}
					if step < 2 {
						target = id
						st, _ := e.saves.LoadState(ctx, n.StateHash)
						manifest, merr := e.saves.LoadMemory(ctx, n.MemoryHash)
						if merr != nil {
							t.Fatal(merr)
						}
						var paths []string
						for path := range manifest.Files {
							paths = append(paths, path)
						}
						slices.Sort(paths)
						var memory string
						for _, path := range paths {
							memory += path + ": " + memoryText(t, e, sv.ID, id, path) + "\n"
						}
						fmt.Fprintf(&report, "金币：%v；记忆：%s\n\n", st["gold"], memory)
						if fmt.Sprint(st["gold"]) != "7" || (!strings.Contains(memory, "蓝") || !strings.Contains(memory, "伞")) || strings.Contains(memory, "买下红伞") || strings.Contains(memory, "买了红伞") {
							t.Errorf("重写状态或记忆不符：%v / %s", st, memory)
						}
					}
					if step == 2 {
						base, _ := e.saves.Node(ctx, sv.ID, target)
						if base.StateHash != n.StateHash || base.MemoryHash != n.MemoryHash || base.TurnNo != n.TurnNo {
							t.Error("补选项改变了状态")
						}
					}
					for _, req := range reqs[start:] {
						if step >= 2 && strings.Contains(string(jsonx.MustMarshal(req.Messages)), "rewrite_request") {
							t.Error("重写要求污染后续上下文")
						}
					}
					for _, reply := range replies[replyStart:] {
						fmt.Fprintf(&report, "原始普通回复（%s）：%s\n\n", reply.Purpose, reply.Message.TextOf())
						if reply.Purpose != "rewrite_memory" && immersionLeak.MatchString(reply.Message.TextOf()) {
							t.Errorf("第 %d 步原始回复出现操作旁白：%s", step+1, reply.Message.TextOf())
						}
					}
					fmt.Fprintf(&report, "模型调用：%d 次。\n\n", len(reqs)-start)
					t.Logf("第 %d 步完成，调用 %d 次", step+1, len(reqs)-start)
				}
			})
		}
	}
}

var immersionLeak = regexp.MustCompile(`(?i)let me|alright|好的[，,！!]|按.{0,12}要求.{0,8}(重写|改写)|以下是.{0,12}(版本|选项)|接下来.{0,12}(调用|工具)|我将.{0,12}(重写|生成|调用)|正文已完整|购买经过已记下|金币已扣至|状态已更新|记忆已更新`)

// operationAuditProvider 记录原始助手消息，独立核对界面未展示的普通回复。
type operationAuditProvider struct {
	llm.Provider
	replies *[]operationAuditReply
}

func (p operationAuditProvider) Stream(ctx context.Context, model *llm.Model, req *llm.Request) (llm.Stream, error) {
	s, err := p.Provider.Stream(ctx, model, req)
	if err != nil {
		return nil, err
	}
	return operationAuditStream{Stream: s, replies: p.replies, purpose: req.Purpose}, nil
}

type operationAuditReply struct {
	Purpose string
	Message llm.Message
}

type operationAuditStream struct {
	purpose string
	llm.Stream
	replies *[]operationAuditReply
}

func (s operationAuditStream) Next() (llm.Event, error) {
	ev, err := s.Stream.Next()
	if done, ok := ev.(llm.Done); ok {
		*s.replies = append(*s.replies, operationAuditReply{Purpose: s.purpose, Message: done.Message})
	}
	return ev, err
}

func repeatedParagraph(text string) bool {
	seen := map[string]bool{}
	for _, p := range strings.Split(text, "\n\n") {
		p = strings.TrimSpace(p)
		if len([]rune(p)) < 30 {
			continue
		}
		if seen[p] {
			return true
		}
		seen[p] = true
	}
	return false
}
