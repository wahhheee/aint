package runtime_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/work"
)

func upgradeVersion(t *testing.T, e *env, version, stateDef string, opening bool) *work.ImportResult {
	t.Helper()
	files := fstest.MapFS{
		"work.yaml":           {Data: []byte("format: 1\nopenings:\n  - {id: default, title: 故事开场, mode: generate}\nid: test.upgrade\nversion: " + version + "\ntitle: 升级故事\nchangelog: 更新说明\n")},
		"prompts/narrator.md": {Data: []byte("指令版本：" + version)},
		"state.yaml":          {Data: []byte(stateDef)},
	}
	if opening {
		files["work.yaml"].Data = []byte(strings.Replace(string(files["work.yaml"].Data), "mode: generate}", "mode: static, file: prompts/opening.md}", 1))
		files["prompts/opening.md"] = &fstest.MapFile{Data: []byte("故事开始。")}
	}
	set, err := work.FromFS(files, work.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	res, err := work.NewService(e.db, e.blobs).ImportFileSet(context.Background(), set, work.Source{Type: "zip"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

const oldUpgradeState = "fields:\n  keep: {type: number, default: 7}\n  gone: {type: string, default: 旧值}\n  change: {type: number, default: 3}\n"
const newUpgradeState = "fields:\n  keep: {type: number, default: 0}\n  added: {type: boolean, default: true}\n  change: {type: string, default: 新默认值}\n"

func TestUpgradeRetainsHistoryAndMigrates(t *testing.T) {
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(compactScript()), reqs: &reqs})
	ctx := context.Background()
	old := upgradeVersion(t, e, "1.0.0", oldUpgradeState, true)
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: old.WorkID, VersionID: old.VersionID})
	if err != nil {
		t.Fatal(err)
	}
	previous := sv.HeadNodeID
	target := upgradeVersion(t, e, "2.0.0", newUpgradeState, true)
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if err := e.mgr.Upgrade(ctx, sv.ID, target.VersionID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.CompactionDone)
	current, line, err := e.saves.ProgressLine(ctx, sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	head := line[len(line)-1]
	if current.WorkVersionID != target.VersionID || head.WorkVersionID != target.VersionID || len(head.Meta.Migration) != 2 {
		t.Fatal("版本迁移未提交", current, head)
	}
	st, err := e.saves.LoadState(ctx, head.StateHash)
	if err != nil {
		t.Fatal(err)
	}
	if st["keep"] != float64(7) || st["added"] != true || st["change"] != "新默认值" || st["gone"] != nil {
		t.Fatal("状态迁移错误", st)
	}
	if len(reqs) == 0 {
		t.Fatal("有故事的升级应整理旧版本")
	}
	for _, req := range reqs {
		for _, block := range req.System {
			if strings.Contains(block.Text, "指令版本：2.0.0") {
				t.Fatal("整理提前使用新作品")
			}
		}
	}
	epoch, err := e.saves.Blob(ctx, head.EpochHash)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(epoch), "旧值") || !strings.Contains(string(epoch), "新默认值") {
		t.Fatal("阶段块含有弃用的状态", string(epoch))
	}
	works := work.NewService(e.db, e.blobs)
	if err := works.Delete(ctx, old.WorkID, old.VersionID); err == nil {
		t.Fatal("不能删除历史分支引用的版本")
	}
	if _, err := e.saves.SetHead(ctx, sv.ID, previous, save.HeadHere); err != nil {
		t.Fatal(err)
	}
	rewound, err := e.saves.Get(ctx, sv.ID)
	if err != nil || rewound.WorkVersionID != old.VersionID {
		t.Fatal("回退没有恢复绑定版本", rewound, err)
	}
	if _, err := e.saves.SetHead(ctx, sv.ID, head.ID, save.HeadHere); err != nil {
		t.Fatal(err)
	}
	// 升级节点不保留任何回合时，再次切换版本仍保留已有交接摘要。
	if err := e.mgr.Upgrade(ctx, sv.ID, old.VersionID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.CompactionDone)
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputFree, Text: "继续"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
}

func TestUpgradeEmptyAndFailure(t *testing.T) {
	for _, empty := range []bool{true, false} {
		t.Run(map[bool]string{true: "无回合不调用模型", false: "整理失败保持旧进度"}[empty], func(t *testing.T) {
			var reqs []*llm.Request
			e := setupWith(t, recordingProvider{Provider: fake.New(&fake.Script{Compact: []fake.Response{{Error: "auth"}}}), reqs: &reqs})
			ctx := context.Background()
			old := upgradeVersion(t, e, "1.0.0", oldUpgradeState, !empty)
			sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: old.WorkID})
			if err != nil {
				t.Fatal(err)
			}
			target := upgradeVersion(t, e, "2.0.0", newUpgradeState, !empty)
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			if err := e.mgr.Upgrade(ctx, sv.ID, target.VersionID); err != nil {
				t.Fatal(err)
			}
			if empty {
				waitFor(t, sub, event.CompactionDone)
			} else {
				waitFor(t, sub, event.CompactionFailed)
			}
			current, err := e.saves.Get(ctx, sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			if empty {
				if len(reqs) != 0 || current.WorkVersionID != target.VersionID {
					t.Fatal("空存档升级调用了模型或没有提交")
				}
				p, err := e.mgr.Presentation(ctx, sv.ID)
				if err != nil || !p.NeedsOpening {
					t.Fatal("升级后仍须生成开场", err)
				}
			} else if current.HeadNodeID != sv.HeadNodeID || current.WorkVersionID != old.VersionID {
				t.Fatal("失败留下半升级状态")
			}
		})
	}
}
