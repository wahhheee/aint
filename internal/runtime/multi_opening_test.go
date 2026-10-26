package runtime_test

import (
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/state"
	"github.com/wahhheee/aint/internal/work"
)

func openingSave(t *testing.T, e *env, opening string) *save.Save {
	t.Helper()
	sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "aint.misty-harbor", OpeningID: opening,
		Title: "我的旅程", Form: map[string]any{"player.name": "阿宁"}, Settings: save.Settings{Pace: "short"}})
	if err != nil {
		t.Fatal(err)
	}
	return sv
}

func openingTurn(t *testing.T, e *env, id string, input save.TurnInput) *save.Node {
	t.Helper()
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(id), 0)
	defer sub.Close()
	turn, err := e.mgr.Submit(t.Context(), id, input)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	n, err := e.saves.Node(t.Context(), id, turn)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOpeningSwitchResetsStateAndHistory(t *testing.T) {
	e := setup(t, quickScript())
	sv := openingSave(t, e, "")
	ctx := t.Context()
	original := sv.HeadNodeID
	if err := e.mgr.SwitchOpening(ctx, sv.ID, sv.OpeningID, original); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := e.saves.Get(ctx, sv.ID)
	if unchanged.HeadNodeID != original {
		t.Fatal("选择当前开场不能重建")
	}
	if err := e.mgr.SwitchOpening(ctx, sv.ID, "missing", original); err == nil {
		t.Fatal("未知开场应被拒绝")
	}
	if _, _, err := e.saves.EditState(ctx, sv.ID, []state.Op{{Op: state.OpSet, Path: "trust.林舟", Value: 99}}); err != nil {
		t.Fatal(err)
	}
	text := "旧开场的记忆"
	if _, err := e.saves.EditMemory(ctx, sv.ID, "/memory/temporary.md", &text); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.SwitchOpening(ctx, sv.ID, "arriving", original); !errs.Is(err, errs.CodeConflict) {
		t.Fatalf("旧页面应拒绝：%v", err)
	}
	current, _ := e.saves.Get(ctx, sv.ID)
	if err := e.mgr.SwitchOpening(ctx, sv.ID, "arriving", current.HeadNodeID); err != nil {
		t.Fatal(err)
	}
	sv, line, err := e.saves.ProgressLine(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(line) != 1 || sv.TurnCount != 0 || sv.OpeningID != "arriving" || sv.Title != "我的旅程" || sv.Settings.Pace != "short" {
		t.Fatalf("切换结果不符：%+v", sv)
	}
	st, err := e.saves.LoadState(ctx, line[0].StateHash)
	if err != nil {
		t.Fatal(err)
	}
	if st["trust"].(map[string]any)["林舟"] != float64(0) || st["player"].(map[string]any)["name"] != "阿宁" {
		t.Fatalf("初始状态不符：%+v", st)
	}
	if got := memoryText(t, e, sv.ID, sv.HeadNodeID, "/memory/temporary.md"); got != "" {
		t.Fatal("旧记忆残留", got)
	}
	if _, err := e.saves.Node(ctx, sv.ID, original); !errs.Is(err, errs.CodeNotFound) {
		t.Fatal("旧开场不应保留为分支", err)
	}
	if err := e.mgr.SwitchOpening(ctx, sv.ID, "post-office", sv.HeadNodeID); err != nil {
		t.Fatal(err)
	}
	sv, line, err = e.saves.ProgressLine(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	st, _ = e.saves.LoadState(ctx, line[0].StateHash)
	if st["trust"].(map[string]any)["林舟"] != float64(2) || sv.TurnCount != 1 {
		t.Fatal("开场覆盖不能累积", st)
	}
	if got := timelineTurns(t, e, sv.ID); !strings.Contains(got, "阿宁") {
		t.Fatal("静态开场应保留玩家表单", got)
	}
}

func TestOpeningLocksAfterPlayerInteraction(t *testing.T) {
	for _, input := range []save.TurnInput{
		{Kind: save.InputFree, Text: "看看窗外"},
		{Kind: save.InputChoice, ChoiceID: "a"},
		{Kind: save.InputOOC, Text: "先介绍一下规则"},
		{Kind: save.InputContinue},
		{Kind: save.InputCommand, Command: &save.CommandCall{Name: "follow", Args: map[string]string{"who": "林舟"}}},
	} {
		t.Run(input.Kind, func(t *testing.T) {
			e := setup(t, quickScript())
			sv := openingSave(t, e, "post-office")
			opening := sv.HeadNodeID
			n := openingTurn(t, e, sv.ID, input)
			p, err := e.mgr.Presentation(t.Context(), sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !p.Save.OpeningLocked || p.Opening.CanSwitch {
				t.Fatal("成功回合必须锁定开场")
			}
			if _, err := e.saves.SetHead(t.Context(), sv.ID, opening, save.HeadHere); err != nil {
				t.Fatal(err)
			}
			if _, err := e.saves.DeleteBranch(t.Context(), sv.ID, n.ID); err != nil {
				t.Fatal(err)
			}
			if err := e.mgr.SwitchOpening(t.Context(), sv.ID, "arriving", opening); !errs.Is(err, errs.CodeConflict) {
				t.Fatal("回退与删除不能解锁", err)
			}
			cp, err := e.saves.CopyAsNew(t.Context(), sv.ID, opening)
			if err != nil {
				t.Fatal(err)
			}
			if !cp.OpeningLocked || cp.OpeningID != sv.OpeningID {
				t.Fatal("复制必须继承开场与锁定状态")
			}
			if err := e.mgr.SwitchOpening(t.Context(), cp.ID, "arriving", cp.HeadNodeID); !errs.Is(err, errs.CodeConflict) {
				t.Fatal("复制不能解锁", err)
			}
		})
	}
}

func TestGeneratedOpeningCanSwitchAndKeepsUsage(t *testing.T) {
	var requests []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(quickScript()), reqs: &requests})
	sv := openingSave(t, e, "arriving")
	n := openingTurn(t, e, sv.ID, save.TurnInput{Kind: save.InputOpening, ExpectedHeadID: sv.HeadNodeID})
	p, err := e.mgr.Presentation(t.Context(), sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !n.Meta.Opening || p.Save.OpeningLocked || !p.Opening.CanSwitch || p.NeedsOpening {
		t.Fatal("生成开场不是玩家首轮")
	}
	if !strings.Contains(requests[0].Messages[len(requests[0].Messages)-1].TextOf(), "阿宁第一次来到这里") {
		t.Fatal("应使用所选开场指引及表单")
	}
	for _, msg := range requests[0].Messages {
		if strings.Contains(msg.TextOf(), "柜台上的煤油灯还亮着") {
			t.Fatal("其他开场不应进入上下文")
		}
	}
	var before, after int
	if err := e.db.Read().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM usage_log WHERE save_id = ?", sv.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.SwitchOpening(t.Context(), sv.ID, "post-office", n.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.db.Read().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM usage_log WHERE save_id = ?", sv.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before == 0 || before != after {
		t.Fatal("切换必须保留用量")
	}
}

func TestInterruptedOpeningAndInteraction(t *testing.T) {
	for _, opening := range []string{"post-office", "arriving"} {
		t.Run(opening, func(t *testing.T) {
			e := setupWith(t, &stalledProvider{Provider: fake.New(quickScript()), text: "尚未写完的正文"})
			sv := openingSave(t, e, opening)
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			kind, target := save.InputFree, "arriving"
			if opening == "arriving" {
				kind, target = save.InputOpening, "post-office"
			}
			id, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: kind, Text: "推门"})
			if err != nil {
				t.Fatal(err)
			}
			waitFor(t, sub, event.TurnText)
			if err := e.mgr.SwitchOpening(t.Context(), sv.ID, target, sv.HeadNodeID); !errs.Is(err, errs.CodeConflict) {
				t.Fatal("生成时不能切换", err)
			}
			if err := e.mgr.Cancel(sv.ID, id); err != nil {
				t.Fatal(err)
			}
			waitFor(t, sub, event.TurnCommitted)
			p, err := e.mgr.Presentation(t.Context(), sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			if p.Save.OpeningLocked || !p.Opening.CanSwitch || p.Opening.HasUnfinishedTurn != (kind == save.InputFree) {
				t.Fatal("中断不能锁定，玩家首轮应提示清除")
			}
			if kind == save.InputOpening {
				n := openingTurn(t, e, sv.ID, save.TurnInput{Kind: save.InputContinue})
				if !n.Meta.Opening {
					t.Fatal("中断开场的续写仍是开场")
				}
				id = n.ID
			}
			if err := e.mgr.SwitchOpening(t.Context(), sv.ID, target, id); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpeningFormPrecedenceAndFailedSwitch(t *testing.T) {
	e := setup(t, quickScript())
	set := work.NewFileSet()
	files := map[string]string{
		"work.yaml": `format: 1
id: test.initial-state
version: 1.0.0
title: 初始状态
openings:
  - {id: a, title: 甲, mode: static, file: a.md, initial_state: {player.name: 开场姓名, day: 2}}
  - {id: b, title: 乙, mode: static, file: a.md, initial_state: {player.name: 另一姓名, day: 3}}
  - {id: bad, title: 超限模板, mode: static, file: bad.md}
new_game:
  fields:
    - {key: player.name, label: 姓名, type: text, default: 表单默认}
`,
		"state.yaml": "fields:\n  player:\n    type: object\n    fields:\n      name: {type: string, default: 状态默认}\n  day: {type: number, default: 1}\n",
		"a.md":       "{{ state.player.name }}，第 {{ state.day }} 天。",
		"bad.md":     "{% for i in (1..1000000) %}超过输出上限{% endfor %}",
	}
	for path, content := range files {
		if err := set.Add(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := work.NewService(e.db, e.blobs).ImportFileSet(t.Context(), set, work.Source{Type: "dir"}); err != nil {
		t.Fatal(err)
	}
	sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "test.initial-state", Form: map[string]any{"player.name": "玩家姓名"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := timelineTurns(t, e, sv.ID); got != "玩家姓名，第 2 天。" {
		t.Fatal("表单必须优先于开场与状态默认值", got)
	}
	if err := e.mgr.SwitchOpening(t.Context(), sv.ID, "b", sv.HeadNodeID); err != nil {
		t.Fatal(err)
	}
	if got := timelineTurns(t, e, sv.ID); got != "玩家姓名，第 3 天。" {
		t.Fatal("切换后保留表单并应用新覆盖", got)
	}
	before, _ := e.saves.Get(t.Context(), sv.ID)
	if err := e.mgr.SwitchOpening(t.Context(), sv.ID, "bad", before.HeadNodeID); !errs.Is(err, errs.CodeWorkInvalid) {
		t.Fatal("模板超限应拒绝切换", err)
	}
	after, _ := e.saves.Get(t.Context(), sv.ID)
	if before.HeadNodeID != after.HeadNodeID || after.OpeningID != "b" {
		t.Fatal("失败不能改变开场或历史")
	}
}

func TestOpeningLockSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	e, err := openEnv(t.Context(), dir, fake.New(quickScript()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })
	sv := openingSave(t, e, "post-office")
	openingTurn(t, e, sv.ID, save.TurnInput{Kind: save.InputOOC, Text: "了解了"})
	if err := e.close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openEnv(t.Context(), dir, fake.New(quickScript()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.close() })
	current, err := reopened.saves.Get(t.Context(), sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.OpeningLocked {
		t.Fatal("重启后必须保留锁定状态")
	}
	if err := reopened.mgr.SwitchOpening(t.Context(), sv.ID, "arriving", current.HeadNodeID); !errs.Is(err, errs.CodeConflict) {
		t.Fatal("重启不能解锁", err)
	}
}
