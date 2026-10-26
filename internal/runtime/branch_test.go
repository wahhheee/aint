package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/save"
)

// branchScript：每个回合两次调用——先写一行正文、把玩家输入记进 /memory/log.md 并检索"乙"，再写"（完）"。
func branchScript() *fake.Script {
	return &fake.Script{Responses: []fake.Response{
		{Text: "回应：{{input}}", ChunkRunes: 50, DelayMS: 1, ToolCalls: []fake.ToolCall{
			{Name: "write", Args: map[string]any{"path": "/memory/log.md", "content": "{{input}}"}},
			{Name: "search", Args: map[string]any{"query": "乙"}},
		}},
		{Text: "（完）", ChunkRunes: 50, DelayMS: 1},
	}}
}

// regenerate 重新生成（in 非空时为修改玩家输入）并等待提交。
func regenerate(t *testing.T, e *env, saveID, nodeID string, in *save.TurnInput) string {
	t.Helper()
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(saveID), 0)
	defer sub.Close()
	if _, err := e.mgr.Regenerate(context.Background(), saveID, nodeID, in); err != nil {
		t.Fatal(err)
	}
	return waitFor(t, sub, event.TurnCommitted).Data.(event.TurnCommittedData).NodeID
}

func memoryText(t *testing.T, e *env, saveID, nodeID, path string) string {
	t.Helper()
	ctx := context.Background()
	n, err := e.saves.Node(ctx, saveID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	mf, err := e.saves.LoadMemory(ctx, n.MemoryHash)
	if err != nil {
		t.Fatal(err)
	}
	f, ok := mf.Files[path]
	if !ok {
		return ""
	}
	data, _ := e.saves.Blob(ctx, f.Blob)
	return string(data)
}

// searchResult 返回某回合第二次调用前、检索"乙"的工具结果。
func searchResult(reqs []*llm.Request, i int) string {
	rs := toolResults(reqs[i].Messages[len(reqs[i].Messages)-1])
	return rs[1].Parts[0].Text
}

func timelineTurns(t *testing.T, e *env, saveID string) string {
	t.Helper()
	tl, err := e.saves.Timeline(context.Background(), saveID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, n := range tl.Nodes {
		if n.Kind == save.KindTurn {
			var text strings.Builder
			for _, it := range n.Display {
				text.WriteString(it.Markup)
			}
			parts = append(parts, text.String())
		}
	}
	return strings.Join(parts, "|")
}

// 分支：重新生成、修改玩家输入、回到此处、切换分支、修改正文、丢弃、删除分支、复制为新存档。
// 每个分支的记忆、原文检索只含该分支的内容（不变量 3：分支隔离）。
func TestBranches(t *testing.T) {
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(branchScript()), reqs: &reqs})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.first-story", Settings: save.Settings{Pace: "short"}})
	if err != nil {
		t.Fatal(err)
	}
	free := func(text string) save.TurnInput { return save.TurnInput{Kind: save.InputFree, Text: text} }

	t1 := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})
	t2 := play(t, e, sv.ID, free("甲"))
	t3 := play(t, e, sv.ID, free("乙"))
	t4 := play(t, e, sv.ID, free("丁"))
	if r := searchResult(reqs, 7); !strings.Contains(r, "/transcript/0003:2: > 玩家：【乙】") {
		t.Fatalf("原分支第 4 回合应检索到第 3 回合：\n%s", r)
	}

	// 修改第 3 回合的玩家输入：以同一父节点生成新版本
	t3b := regenerate(t, e, sv.ID, t3, &save.TurnInput{Kind: save.InputFree, Text: "丙"})
	t4b := play(t, e, sv.ID, free("丁"))
	if r := searchResult(reqs, 11); !strings.Contains(r, "（没有找到）") {
		t.Fatalf("新分支检索不到另一分支的原文：\n%s", r)
	}
	if got := memoryText(t, e, sv.ID, t3b, "/memory/log.md"); got != "丙" {
		t.Fatalf("新分支第 3 回合的记忆 = %q", got)
	}
	if got := memoryText(t, e, sv.ID, t3, "/memory/log.md"); got != "乙" {
		t.Fatalf("原分支的记忆不受影响 = %q", got)
	}
	if got := timelineTurns(t, e, sv.ID); got != "回应：请按开场指引开始故事。（完）|回应：甲（完）|回应：丙（完）|回应：丁（完）" {
		t.Fatalf("时间线 = %s", got)
	}
	tl, _ := e.saves.Timeline(ctx, sv.ID, "", 50)
	if v := tl.Nodes[2].Versions; v.Index != 2 || v.Count != 2 {
		t.Fatalf("第 3 回合的版本 = %+v", v)
	}

	// 回到此处：head 停在第 2 回合，后续保留；切换到最新进度
	if _, err := e.saves.SetHead(ctx, sv.ID, t2, save.HeadHere); err != nil {
		t.Fatal(err)
	}
	if tl, _ := e.saves.Timeline(ctx, sv.ID, "", 50); !tl.HasLater || len(tl.Nodes) != 2 {
		t.Fatalf("回到第 2 回合：%+v", tl)
	}
	if head, _ := e.saves.SetHead(ctx, sv.ID, t2, save.HeadLatestLeaf); head != t4b {
		t.Fatalf("最新进度应是新分支的第 4 回合")
	}
	if head, _ := e.saves.SetHead(ctx, sv.ID, t3, save.HeadLatestLeaf); head != t4 {
		t.Fatalf("切换版本：第 3 回合原版本所在分支的最新进度应是原来的第 4 回合")
	}
	if got := timelineTurns(t, e, sv.ID); !strings.HasSuffix(got, "回应：乙（完）|回应：丁（完）") {
		t.Fatalf("切换回原分支后的时间线 = %s", got)
	}

	// 分支结构
	tree, err := e.saves.Tree(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Root.Turns != 2 || len(tree.Root.Children) != 2 || !tree.Root.Children[0].Current || tree.Root.Children[1].Current ||
		tree.Root.Children[1].Summary != "玩家：丙" || tree.Root.Children[1].LeafID != t4b {
		t.Fatalf("分支结构：%+v / %+v / %+v", tree.Root, tree.Root.Children[0], tree.Root.Children[1])
	}
	if sv2, _ := e.saves.Get(ctx, sv.ID); sv2.BranchCount != 2 || sv2.TurnCount != 4 {
		t.Fatalf("分支数 %d、回合数 %d", sv2.BranchCount, sv2.TurnCount)
	}

	// 修改 AI 正文：新版本成为 head，原文索引随之更新
	t4c, err := e.saves.Rewrite(ctx, sv.ID, t4, "改写后的正文。")
	if err != nil {
		t.Fatal(err)
	}
	if got := timelineTurns(t, e, sv.ID); !strings.HasSuffix(got, "|改写后的正文。") {
		t.Fatalf("修改后的时间线 = %s", got)
	}
	texts, _ := e.saves.TranscriptTexts(ctx, sv.ID)
	if texts[t4c.ID] != "> 玩家：丁\n\n改写后的正文。" {
		t.Fatalf("修改后的原文 = %q", texts[t4c.ID])
	}
	msgs, _ := e.saves.LoadMessages(ctx, t4c.MessagesHash)
	var assistant []string
	for _, m := range msgs {
		if m.Role == llm.RoleAssistant {
			assistant = append(assistant, m.TextOf())
		}
	}
	if strings.Join(assistant, "|") != "改写后的正文。" || len(msgs[1].ToolCalls()) != 2 {
		t.Fatalf("助手文本应合并为一段，工具调用保留：%q", assistant)
	}

	// 删除分支：不能删除 head 所在的分支
	if _, err := e.saves.DeleteBranch(ctx, sv.ID, t3); !errs.Is(err, errs.CodeConflict) {
		t.Fatalf("删除 head 所在的分支应拒绝：%v", err)
	}
	if _, err := e.saves.SetHead(ctx, sv.ID, t3b, save.HeadLatestLeaf); err != nil {
		t.Fatal(err)
	}
	n, err := e.saves.DeleteBranch(ctx, sv.ID, t3)
	if err != nil || n != 3 { // t3、t4、改写的 t4
		t.Fatalf("删除分支：%d %v", n, err)
	}
	texts, _ = e.saves.TranscriptTexts(ctx, sv.ID)
	if _, ok := texts[t3]; ok || len(texts) != 4 {
		t.Fatalf("删除分支后原文索引应一并删除：%d", len(texts))
	}

	// 复制为新存档
	cp, err := e.saves.CopyAsNew(ctx, sv.ID, t2)
	if err != nil {
		t.Fatal(err)
	}
	if cp.ForkedFrom == nil || cp.ForkedFrom.TurnNo != 2 || cp.TurnCount != 2 {
		t.Fatalf("复制的存档：%+v", cp)
	}
	if got := timelineTurns(t, e, cp.ID); got != "回应：请按开场指引开始故事。（完）|回应：甲（完）" {
		t.Fatalf("复制的存档时间线 = %s", got)
	}
	if texts, _ := e.saves.TranscriptTexts(ctx, cp.ID); len(texts) != 2 {
		t.Fatalf("复制的存档原文索引 = %d", len(texts))
	}
	_ = t1
}

// 玩家修改记忆：产生编辑节点，下一回合在 <notes> 中告知主笔；丢弃中断的回合后 head 回到父节点。
func TestEditNotesAndDiscard(t *testing.T) {
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(branchScript()), reqs: &reqs})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.first-story", Settings: save.Settings{Pace: "short"}})
	if err != nil {
		t.Fatal(err)
	}
	t1 := play(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening})

	content := "# 玩家改过的\n"
	if err := e.mgr.Exclusive(ctx, sv.ID, "edit_memory", func() error {
		_, err := e.saves.EditMemory(ctx, sv.ID, "/memory/player.md", &content)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if v, _ := e.saves.Memory(ctx, sv.ID); len(v.Files) != 2 || v.Files[1].Path != "/memory/player.md" || v.Files[1].Turn != 1 {
		t.Fatalf("记忆列表：%+v", v.Files)
	}
	if sv2, _ := e.saves.Get(ctx, sv.ID); sv2.TurnCount != 1 {
		t.Fatalf("head 停在编辑节点时回合数仍为 1：%d", sv2.TurnCount)
	}
	play(t, e, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "继续"})
	first := reqs[2].Messages[len(reqs[2].Messages)-1].TextOf()
	if !strings.Contains(first, "<notes>\n- 玩家新建了记忆：/memory/player.md\n</notes>") {
		t.Fatalf("下一回合应附注记：\n%s", first)
	}
	tl, _ := e.saves.Timeline(ctx, sv.ID, "", 50)
	if len(tl.Nodes) != 3 || tl.Nodes[1].Kind != save.KindEdit || tl.Nodes[1].Notes[0] != "玩家新建了记忆：/memory/player.md" {
		t.Fatalf("时间线应包含编辑节点：%+v", tl.Nodes)
	}

	// 中断的回合可以丢弃
	_, line, _ := e.saves.ProgressLine(ctx, sv.ID)
	head := line[len(line)-1]
	n, err := e.saves.CommitTurn(ctx, save.TurnCommit{
		SaveID: sv.ID, Parent: head, TurnNo: 3, Input: save.TurnInput{Kind: save.InputFree, Text: "停"},
		Display: []save.DisplayItem{{T: "text", Markup: "写到一半"}}, StateHash: head.StateHash, MemoryHash: head.MemoryHash,
		WorkVersionID: head.WorkVersionID, Status: save.StatusInterrupted, PlainText: "写到一半",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.saves.Discard(ctx, sv.ID, t1); err == nil {
		t.Fatal("已提交的回合不能丢弃")
	}
	newHead, err := e.saves.Discard(ctx, sv.ID, n.ID)
	if err != nil || newHead != head.ID {
		t.Fatalf("丢弃后 head 回到父节点：%s %v", newHead, err)
	}
}
