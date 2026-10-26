package usage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/store"
)

func openTest(t *testing.T) *store.DB {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(dir, "aint.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	return db
}

func exec(t *testing.T, db *store.DB, fn func(tx *sql.Tx) error) {
	t.Helper()
	if err := db.Tx(t.Context(), fn); err != nil {
		t.Fatal(err)
	}
}

// seed 建两部作品、三个存档，写入各类调用：
//
//	s1（作品 w1）：主笔 narrate 两次（CNY）+ continue 一次（CNY）+ compact 一次（轻量模型，USD）
//	s2（作品 w1）：narrate 一次（模型未配置价格）
//	s3（作品 w2）：narrate 一次（CNY）
//	连接测试 probe 一次（不属于任何存档）
func seed(t *testing.T, db *store.DB) {
	t.Helper()
	exec(t, db, func(tx *sql.Tx) error {
		ctx := t.Context()
		for _, q := range []string{
			`INSERT INTO works (id, title, created_at, updated_at) VALUES ('w1', '作品一', 0, 0), ('w2', '作品二', 0, 0)`,
			`INSERT INTO work_versions (id, work_id, version, tree_hash, source, manifest, imported_at)
			 VALUES ('v1', 'w1', '1.0.0', 'h1', '{}', '{}', 0), ('v2', 'w2', '1.0.0', 'h2', '{}', '{}', 0)`,
			`INSERT INTO saves (id, work_id, work_version_id, settings, opening_id, initial_form, created_at, updated_at)
			 VALUES ('s1', 'w1', 'v1', '{}', 'default', '{}', 0, 0), ('s2', 'w1', 'v1', '{}', 'default', '{}', 0, 0), ('s3', 'w2', 'v2', '{}', 'default', '{}', 0, 0)`,
			`INSERT INTO providers (id, name, protocol, base_url, secret_ref, created_at) VALUES ('p', '供应商', 'anthropic', 'https://x', 'k', 0)`,
			`INSERT INTO models (id, provider_id, model, display_name, context_window, max_output, caps)
			 VALUES ('main', 'p', 'main-model', '主笔模型', 1000, 100, '{}'), ('light', 'p', 'light-model', NULL, 1000, 100, '{}')`,
		} {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		cost := func(v int64) *int64 { return &v }
		for _, r := range []Record{
			{SaveID: "s1", NodeID: "t1", CallKind: "narrate", ModelID: "main", Usage: llm.Usage{InputUncached: 100, CacheRead: 900, Output: 50}, CostMicros: cost(1000), Currency: "CNY"},
			{SaveID: "s1", NodeID: "t2", CallKind: "narrate", ModelID: "main", Usage: llm.Usage{InputUncached: 10, CacheRead: 1000, CacheWrite: 20, Output: 60, Reasoning: 5}, CostMicros: cost(2000), Currency: "CNY"},
			{SaveID: "s1", NodeID: "t2", CallKind: "continue", ModelID: "main", Usage: llm.Usage{CacheRead: 1100, Output: 40}, CostMicros: cost(500), Currency: "CNY"},
			{SaveID: "s1", NodeID: "c1", CallKind: "compact", ModelID: "light", Usage: llm.Usage{InputUncached: 3000, Output: 300}, CostMicros: cost(700), Currency: "USD"},
			{SaveID: "s2", NodeID: "t3", CallKind: "narrate", ModelID: "gone", Usage: llm.Usage{InputUncached: 400, Output: 80}},
			{SaveID: "s3", NodeID: "t4", CallKind: "narrate", ModelID: "main", Usage: llm.Usage{InputUncached: 200, Output: 20}, CostMicros: cost(300), Currency: "CNY"},
			{CallKind: "probe", ModelID: "main", Usage: llm.Usage{InputUncached: 30, Output: 3}, CostMicros: cost(10), Currency: "CNY"},
		} {
			if err := Insert(ctx, tx, r); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestSummarizeSave(t *testing.T) {
	db := openTest(t)
	seed(t, db)
	sum, err := NewService(db).Summarize(t.Context(), Filter{SaveID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	want := Stats{
		Usage: llm.Usage{InputUncached: 3110, CacheRead: 3000, CacheWrite: 20, Output: 450, Reasoning: 5},
		Costs: map[string]int64{"CNY": 3500, "USD": 700}, Calls: 4,
	}
	assertStats(t, "合计", sum.Total, want)
	assertStats(t, "narrate", sum.ByKind["narrate"], Stats{
		Usage: llm.Usage{InputUncached: 110, CacheRead: 1900, CacheWrite: 20, Output: 110, Reasoning: 5},
		Costs: map[string]int64{"CNY": 3000}, Calls: 2,
	})
	assertStats(t, "continue", sum.ByKind["continue"], Stats{Usage: llm.Usage{CacheRead: 1100, Output: 40}, Costs: map[string]int64{"CNY": 500}, Calls: 1})
	assertStats(t, "compact", sum.ByKind["compact"], Stats{Usage: llm.Usage{InputUncached: 3000, Output: 300}, Costs: map[string]int64{"USD": 700}, Calls: 1})
	if len(sum.ByKind) != 3 {
		t.Errorf("by_kind 应只含出现过的用途，得到 %v", sum.ByKind)
	}

	if len(sum.ByModel) != 2 {
		t.Fatalf("by_model 应有两个模型，得到 %+v", sum.ByModel)
	}
	if m := sum.ByModel[0]; m.ModelID != "main" || m.Name != "主笔模型" || m.Stats.Calls != 3 {
		t.Errorf("调用最多的模型排在前面并带显示名，得到 %+v", m)
	}
	if m := sum.ByModel[1]; m.ModelID != "light" || m.Name != "light-model" {
		t.Errorf("没有显示名时用供应商侧模型 ID，得到 %+v", m)
	}
}

func TestSummarizeFilters(t *testing.T) {
	db := openTest(t)
	seed(t, db)
	svc := NewService(db)

	work, err := svc.Summarize(t.Context(), Filter{WorkID: "w1"})
	if err != nil {
		t.Fatal(err)
	}
	// w1 = s1 + s2；s2 的调用未配置价格：计入用量与调用次数，不计入费用
	assertStats(t, "作品 w1", work.Total, Stats{
		Usage: llm.Usage{InputUncached: 3510, CacheRead: 3000, CacheWrite: 20, Output: 530, Reasoning: 5},
		Costs: map[string]int64{"CNY": 3500, "USD": 700}, Unpriced: 1, Calls: 5,
	})
	// 调用次数相同的按模型 ID 排列
	if ids := modelIDs(work.ByModel); ids != "main,gone,light" {
		t.Errorf("by_model 顺序为 %s", ids)
	}
	if m := work.ByModel[1]; m.Name != "" || m.Stats.Unpriced != 1 {
		t.Errorf("模型表中没有的模型名称为空，得到 %+v", m)
	}

	all, err := svc.Summarize(t.Context(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total.Calls != 7 || all.Total.Costs["CNY"] != 3810 || all.ByKind["probe"].Calls != 1 {
		t.Errorf("全局累计应包含全部调用（含连接测试），得到 %+v", all.Total)
	}

	node, err := svc.Summarize(t.Context(), Filter{SaveID: "s1", NodeID: "t2"})
	if err != nil {
		t.Fatal(err)
	}
	assertStats(t, "回合 t2", node.Total, Stats{
		Usage: llm.Usage{InputUncached: 10, CacheRead: 2100, CacheWrite: 20, Output: 100, Reasoning: 5},
		Costs: map[string]int64{"CNY": 2500}, Calls: 2,
	})

	// 条件同时满足：存档不属于该作品时没有调用
	none, err := svc.Summarize(t.Context(), Filter{SaveID: "s3", WorkID: "w1"})
	if err != nil {
		t.Fatal(err)
	}
	assertStats(t, "不匹配", none.Total, Stats{Costs: map[string]int64{}})
	if none.ByKind == nil || none.ByModel == nil {
		t.Error("没有调用时分组也应为空集合（JSON 中是 {} 与 []），不是 nil")
	}
}

func TestTotal(t *testing.T) {
	cost := int64(1200)
	got := Total([]Record{
		{Usage: llm.Usage{InputUncached: 10, Output: 5}, CostMicros: &cost}, // 未写货币按默认货币
		{Usage: llm.Usage{CacheRead: 100, Reasoning: 7}},
	})
	assertStats(t, "回合", got, Stats{
		Usage: llm.Usage{InputUncached: 10, CacheRead: 100, Output: 5, Reasoning: 7},
		Costs: map[string]int64{llm.DefaultCurrency: 1200}, Unpriced: 1, Calls: 2,
	})
	if empty := Total(nil); empty.Costs == nil {
		t.Error("没有调用时 Costs 也应非 nil")
	}
}

func modelIDs(ms []ModelStats) string {
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.ModelID
	}
	return strings.Join(ids, ",")
}

func assertStats(t *testing.T, name string, got, want Stats) {
	t.Helper()
	if got.Usage != want.Usage || got.Calls != want.Calls || got.Unpriced != want.Unpriced || len(got.Costs) != len(want.Costs) {
		t.Errorf("%s：得到 %+v，期望 %+v", name, got, want)
		return
	}
	for cur, v := range want.Costs {
		if got.Costs[cur] != v {
			t.Errorf("%s：%s 费用 %d，期望 %d", name, cur, got.Costs[cur], v)
		}
	}
}
