package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/settings"
	"github.com/wahhheee/aint/internal/work"
)

const initialMemoryIndex = "# 记忆索引\n- tasks.md — 当前目标与线索（钥匙已找到，信件仍未交还）。\n"
const compactedMemoryIndex = "# 记忆索引\n- tasks.md — 当前目标与线索（钥匙已找到并存入行囊；信件尚未交还，林舟已动身前往码头，需在他离港前送达）。\n"

func maintenanceSave(t *testing.T, e *env) *save.Save {
	t.Helper()
	files := work.NewFileSet()
	for name, body := range map[string]string{
		"work.yaml": "format: 1\nid: test.maintenance\nversion: 1.0.0\ntitle: 雾港记忆\nopenings:\n  - {id: default, title: 清晨, mode: generate}\n",
		"state.yaml": `fields:
  gold: {type: number, default: 10, desc: 玩家持有的金币}
  tasks: {type: list, items: string, default: [找钥匙, 还信], desc: 尚未完成的当前目标}
  situation: {type: string, default: 找到钥匙, desc: 当前场景与处境}
`,
		"memory/index.md": initialMemoryIndex,
		"memory/tasks.md": "# 当前目标\n只供主动查阅的详细记录。",
	} {
		if err := files.Add(name, []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := work.NewService(e.db, e.blobs).ImportFileSet(t.Context(), files, work.Source{Type: "dir"}); err != nil {
		t.Fatal(err)
	}
	sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "test.maintenance", Settings: save.Settings{KeepRecent: 100}})
	if err != nil {
		t.Fatal(err)
	}
	return sv
}

func maintenanceScript() *fake.Script {
	return &fake.Script{
		Responses: []fake.Response{{Text: "你找到钥匙，准备去码头还信。", ChunkRunes: 100, DelayMS: 1}},
		Compact: []fake.Response{
			{DelayMS: 1, ToolCalls: []fake.ToolCall{
				{Name: "update_state", Args: map[string]any{"op": "remove", "path": "tasks", "value": "找钥匙"}},
				{Name: "update_state", Args: map[string]any{"op": "set", "path": "situation", "value": "携带钥匙前往码头还信"}},
				{Name: "write", Args: map[string]any{"path": "/memory/index.md", "content": compactedMemoryIndex}},
				{Name: "get_state", Args: map[string]any{}},
			}},
			{Text: "## 当前场景\n钥匙已找到，准备去码头还信。", ChunkRunes: 100, DelayMS: 1},
		},
	}
}

func TestCompactionMaintenance(t *testing.T) {
	for _, mode := range []string{"inline", "separate"} {
		for _, fail := range []bool{false, true} {
			t.Run(mode+map[bool]string{true: "失败回滚", false: "提交"}[fail], func(t *testing.T) {
				var reqs []*llm.Request
				script := maintenanceScript()
				if fail {
					script.Compact[1] = fake.Response{Error: "network", DelayMS: 1}
				}
				g := settings.Defaults()
				g.Models.Light, g.Context.Mode = "light", mode
				e, err := openEnvWith(t.Context(), t.TempDir(), fakeModels{recordingProvider{Provider: fake.New(script), reqs: &reqs}},
					func(d *runtime.Deps) { d.Settings = staticSettings{g} })
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = e.close() })
				sv := maintenanceSave(t, e)
				play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
				if !strings.Contains(reqs[0].Messages[0].TextOf(), initialMemoryIndex) {
					t.Fatal("开局未注入完整索引")
				}
				for range 4 {
					play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "继续"})
				}
				before, line, _ := e.saves.ProgressLine(t.Context(), sv.ID)
				head := line[len(line)-1]
				sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
				defer sub.Close()
				if err := e.mgr.Compact(t.Context(), sv.ID); err != nil {
					t.Fatal(err)
				}
				ev := waitAny(t, sub, event.CompactionDone, event.CompactionFailed)
				after, line, _ := e.saves.ProgressLine(t.Context(), sv.ID)
				if fail {
					if ev.Type != event.CompactionFailed || after.HeadNodeID != before.HeadNodeID || memoryText(t, e, sv.ID, head.ID, "/memory/index.md") != initialMemoryIndex {
						t.Fatal("整理失败仍采用了状态或记忆")
					}
					return
				}
				if ev.Type != event.CompactionDone {
					t.Fatalf("整理失败：%+v", ev)
				}
				node := line[len(line)-1]
				st, _ := e.saves.LoadState(t.Context(), node.StateHash)
				if len(st["tasks"].([]any)) != 1 || st["situation"] != "携带钥匙前往码头还信" {
					t.Fatalf("整理后的状态未提交：%v", st)
				}
				old, _ := e.saves.LoadState(t.Context(), head.StateHash)
				if len(old["tasks"].([]any)) != 2 {
					t.Fatal("整理改写了历史快照")
				}
				epoch, _ := e.saves.Blob(t.Context(), node.EpochHash)
				for _, want := range []string{compactedMemoryIndex, "携带钥匙前往码头还信", "不覆盖此处快照"} {
					if !strings.Contains(string(epoch), want) {
						t.Fatalf("阶段块缺少 %q：%s", want, epoch)
					}
				}
				var compactReq *llm.Request
				for _, req := range reqs {
					if req.Purpose == "compact" {
						compactReq = req
					}
				}
				results := toolResults(compactReq.Messages[len(compactReq.Messages)-1])
				if len(results) != 4 || results[0].IsError || !strings.Contains(results[3].Parts[0].Text, "携带钥匙前往码头还信") {
					t.Fatalf("整理工具写入后未即时可读：%+v", results)
				}
				if mode == "separate" {
					body := compactReq.Messages[0].TextOf()
					if !strings.Contains(body, "尚未完成的当前目标") || !strings.Contains(body, initialMemoryIndex) || strings.Contains(body, "只供主动查阅") {
						t.Fatal("独立整理缺少字段含义或完整索引，或泄漏普通记忆全文")
					}
				}
				play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "前往码头"})
				a := reqs[len(reqs)-1].Messages[0].TextOf()
				if !strings.Contains(a, compactedMemoryIndex) || strings.Contains(a, initialMemoryIndex) {
					t.Fatal("下一阶段仍使用旧索引")
				}
				play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "寻找林舟"})
				if reqs[len(reqs)-1].Messages[0].TextOf() != a {
					t.Fatal("阶段内索引快照不稳定")
				}
			})
		}
	}
}

type stateOverflowProvider struct {
	llm.Provider
	hit bool
}

func (p *stateOverflowProvider) Stream(ctx context.Context, model *llm.Model, req *llm.Request) (llm.Stream, error) {
	if req.Purpose == "narrate" && !p.hit {
		for _, result := range toolResults(req.Messages[len(req.Messages)-1]) {
			if strings.Contains(result.Parts[0].Text, "gold 10 → 8") {
				p.hit = true
				return nil, &llm.APIError{Kind: llm.ErrContextOverflow, Message: "超限"}
			}
		}
	}
	return p.Provider.Stream(ctx, model, req)
}

func TestCompactionDuringStateWrites(t *testing.T) {
	script := maintenanceScript()
	script.Responses = append(script.Responses, script.Responses[0], script.Responses[0], script.Responses[0], script.Responses[0],
		fake.Response{DelayMS: 1, ToolCalls: []fake.ToolCall{{Name: "update_state", Args: map[string]any{"op": "inc", "path": "gold", "value": -2}}}},
		fake.Response{DelayMS: 1, ToolCalls: []fake.ToolCall{{Name: "get_state", Args: map[string]any{}}}},
		fake.Response{Text: "你付了船钱，继续还信。", ChunkRunes: 100, DelayMS: 1})
	script.Compact[0].ToolCalls = append(script.Compact[0].ToolCalls, fake.ToolCall{Name: "update_state", Args: map[string]any{"op": "set", "path": "gold", "value": 0}})
	var reqs []*llm.Request
	p := &stateOverflowProvider{Provider: fake.New(script)}
	e := setupWith(t, recordingProvider{Provider: p, reqs: &reqs})
	sv := maintenanceSave(t, e)
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	for range 4 {
		play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "继续"})
	}
	id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "付船钱"})
	node, _ := e.saves.Node(t.Context(), sv.ID, id)
	parent, _ := e.saves.Node(t.Context(), sv.ID, node.ParentID)
	st, _ := e.saves.LoadState(t.Context(), node.StateHash)
	base, _ := e.saves.LoadState(t.Context(), parent.StateHash)
	if !p.hit || parent.Kind != save.KindCompaction || st["gold"] != float64(8) || base["gold"] != float64(10) || len(st["tasks"].([]any)) != 1 {
		t.Fatalf("紧急整理未隔离草稿并合并独立修改：%v / %v", st, base)
	}
	last := reqs[len(reqs)-1]
	var resumed bool
	for _, msg := range last.Messages {
		if strings.Contains(msg.TextOf(), prompts.CompactionResume) {
			resumed = true
		}
	}
	if !resumed {
		t.Fatal("紧急整理未标记旧读取结果的时间边界")
	}
	result := toolResults(last.Messages[len(last.Messages)-1])[0].Parts[0].Text
	if !strings.Contains(result, "gold: 8") || !strings.Contains(result, "携带钥匙前往码头还信") {
		t.Fatalf("紧急整理后的工具未读取合并状态：%s", result)
	}
}
