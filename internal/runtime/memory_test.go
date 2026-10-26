package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/save"
)

// play 提交一个输入并等待回合提交，返回提交后的节点 ID。
func play(t *testing.T, e *env, saveID string, in save.TurnInput) string {
	t.Helper()
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(saveID), 0)
	defer sub.Close()
	if _, err := e.mgr.Submit(context.Background(), saveID, in); err != nil {
		t.Fatal(err)
	}
	return waitFor(t, sub, event.TurnCommitted).Data.(event.TurnCommittedData).NodeID
}

// 记忆写入：write / edit / remove 进入回合覆盖层，随回合提交生成新清单；没有改动的回合沿用父节点的清单。
func TestMemoryWrites(t *testing.T) {
	script := &fake.Script{Responses: []fake.Response{
		{DelayMS: 1, ToolCalls: []fake.ToolCall{
			{Name: "write", Args: map[string]any{"path": "/memory/characters/lin.md", "content": "# 林舟\n第 2 回合：收下了信\n"}},
			{Name: "edit", Args: map[string]any{"path": "/memory/characters/lin.md", "old_text": "收下了信", "new_text": "收下了信，没有拆开"}},
			{Name: "read", Args: map[string]any{"path": "/memory/characters/lin.md"}},
		}},
		{Text: "林舟把信收进口袋。", ChunkRunes: 50, DelayMS: 1},
		{Text: "雾散了一些。", ChunkRunes: 50, DelayMS: 1},
		{DelayMS: 1, ToolCalls: []fake.ToolCall{{Name: "remove", Args: map[string]any{"path": "/memory/characters/lin.md"}}}},
		{Text: "什么也没留下。", ChunkRunes: 50, DelayMS: 1},
	}}
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(script), reqs: &reqs})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.first-story", Settings: save.Settings{Pace: "short"}})
	if err != nil {
		t.Fatal(err)
	}
	t1 := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	t2 := play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "把信交给林舟"})
	t3 := play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "等一等"})

	// 同一回合内写入后立即可读
	var results []string
	for _, r := range toolResults(reqs[1].Messages[len(reqs[1].Messages)-1]) {
		results = append(results, r.Parts[0].Text)
	}
	if len(results) != 3 || results[0] != "# write /memory/characters/lin.md\n已新建（2 行）" ||
		!strings.Contains(results[2], "收下了信，没有拆开") {
		t.Fatalf("工具结果 = %q", results)
	}

	_, line, _ := e.saves.ProgressLine(ctx, sv.ID)
	nodes := map[string]*save.Node{}
	for _, n := range line {
		nodes[n.ID] = n
	}
	root := line[0]
	if nodes[t1].MemoryHash == root.MemoryHash {
		t.Fatal("第 1 回合写了记忆，应生成新清单")
	}
	if nodes[t2].MemoryHash != nodes[t1].MemoryHash {
		t.Fatal("第 2 回合没有改记忆，应沿用父节点的清单")
	}
	mf, err := e.saves.LoadMemory(ctx, nodes[t1].MemoryHash)
	if err != nil {
		t.Fatal(err)
	}
	f, ok := mf.Files["/memory/characters/lin.md"]
	if !ok || f.Turn != 1 {
		t.Fatalf("写入的记忆应记录回合号：%+v", mf.Files)
	}
	data, _ := e.saves.Blob(ctx, f.Blob)
	if string(data) != "# 林舟\n第 2 回合：收下了信，没有拆开\n" {
		t.Fatalf("记忆内容 = %q", data)
	}
	if mf, _ := e.saves.LoadMemory(ctx, nodes[t3].MemoryHash); len(mf.Files) != 0 {
		t.Fatalf("第 3 回合删除后清单应为空：%+v", mf.Files)
	}
}

// 原文：/transcript/NNNN 是进度线上已提交回合的原文（玩家输入行 + 正文纯文本）；search 同时检索记忆、作品资料与原文。
func TestTranscriptAndSearch(t *testing.T) {
	script := &fake.Script{Responses: []fake.Response{
		{Text: "雾是从傍晚开始漫上来的。", ChunkRunes: 50, DelayMS: 1},
		{Text: "<say who=\"林舟\">我收下了。</say>\n\n他把银色钥匙放进口袋。", ChunkRunes: 50, DelayMS: 1},
		{DelayMS: 1, ToolCalls: []fake.ToolCall{
			{Name: "read", Args: map[string]any{"path": "/transcript/0002"}},
			{Name: "search", Args: map[string]any{"query": "银色钥匙"}},
			{Name: "search", Args: map[string]any{"query": "钥匙", "path": "/transcript"}},
			{Name: "list", Args: map[string]any{"path": "/transcript"}},
			{Name: "read", Args: map[string]any{"path": "/transcript/2"}},
		}},
		{Text: "你想起了那把钥匙。", ChunkRunes: 50, DelayMS: 1},
	}}
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(script), reqs: &reqs})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.first-story", Settings: save.Settings{Pace: "short"}})
	if err != nil {
		t.Fatal(err)
	}
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "把钥匙交给林舟"})
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "回想"})

	var results []string
	for _, r := range toolResults(reqs[3].Messages[len(reqs[3].Messages)-1]) {
		results = append(results, r.Parts[0].Text)
	}
	want := []string{
		"# read /transcript/0002（第 1–5 行，共 5 行）\n# 第 2 回合\n> 玩家：把钥匙交给林舟\n\n林舟：我收下了。\n他把银色钥匙放进口袋。",
		"# search \"银色钥匙\"（共 1 处）\n/transcript/0002:5: 他把【银色钥匙】放进口袋。",
		"# search \"钥匙\" 于 /transcript（共 2 处）\n/transcript/0002:2: > 玩家：把【钥匙】交给林舟\n/transcript/0002:5: 他把银色【钥匙】放进口袋。",
		"# list /transcript（共 2 项）\n/transcript/0001  开场\n/transcript/0002  玩家：把钥匙交给林舟",
	}
	for i, w := range want {
		if results[i] != w {
			t.Errorf("结果 %d：\n%s\n期望：\n%s", i, results[i], w)
		}
	}
	if !strings.HasPrefix(results[4], "未执行：/transcript/2 不存在。相近的路径有 /transcript/0002") {
		t.Errorf("回合号省略前导零时给出建议：%s", results[4])
	}
}
