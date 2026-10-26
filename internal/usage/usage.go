// Package usage 记录与统计模型用量（usage_log 表）。
//
// 每次模型调用一条记录，与回合提交在同一事务中写入（原子提交，不变量 2）。
// 费用在写入时按当时的单价计算并固化（cost_micros 为 NULL 表示无法估算，界面显示"未配置价格"）。
package usage

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/store"
)

// Record 是一次模型调用的用量。
type Record struct {
	SaveID     string
	NodeID     string
	CallKind   string // narrate | continue | compact | helper | title | probe
	ModelID    string
	Usage      llm.Usage
	CostMicros *int64 // nil = 无法估算
	Currency   string // CostMicros 的货币（D-34）
	LatencyMS  int64
	Stop       string
}

// Insert 写入一条用量记录。q 通常是回合提交事务。
func Insert(ctx context.Context, q store.Querier, r Record) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO usage_log (save_id, node_id, call_kind, model_id, input_uncached, cache_read, cache_write,
		                       output, reasoning, cost_micros, currency, latency_ms, stop, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.SaveID, r.NodeID, r.CallKind, r.ModelID, r.Usage.InputUncached, r.Usage.CacheRead, r.Usage.CacheWrite,
		r.Usage.Output, r.Usage.Reasoning, r.CostMicros, store.NullString(r.Currency), r.LatencyMS, r.Stop, store.NowMillis())
	return err
}

// Stats 是一组调用的累计用量。
type Stats struct {
	Usage llm.Usage `json:"usage"`
	// Costs 按货币分别累计（D-34），单位为该货币的百万分之一，如 {"USD": 210000, "CNY": 100000}；不做汇率换算。
	Costs map[string]int64 `json:"costs"`
	// Unpriced 是无法估算费用的调用次数（模型未配置价格）。
	Unpriced int `json:"unpriced"`
	Calls    int `json:"calls"`
}

// Add 把一次调用计入统计。
func (s *Stats) Add(r Record) {
	s.Usage = s.Usage.Add(r.Usage)
	s.Calls++
	if r.CostMicros == nil {
		s.Unpriced++
		return
	}
	cur := cmp.Or(r.Currency, llm.DefaultCurrency)
	if s.Costs == nil {
		s.Costs = map[string]int64{}
	}
	s.Costs[cur] += *r.CostMicros
}

// merge 合并另一组统计。
func (s *Stats) merge(o Stats) {
	s.Usage = s.Usage.Add(o.Usage)
	s.Calls += o.Calls
	s.Unpriced += o.Unpriced
	if s.Costs == nil {
		s.Costs = map[string]int64{}
	}
	for cur, v := range o.Costs {
		s.Costs[cur] += v
	}
}

// Total 统计一组调用（如一个回合中的各次调用）。Costs 总是非 nil。
func Total(records []Record) Stats {
	s := Stats{Costs: map[string]int64{}}
	for _, r := range records {
		s.Add(r)
	}
	return s
}

// Filter 选择要统计的调用，各条件同时满足；都为空时统计全部调用（全局累计，含连接测试）。
type Filter struct {
	SaveID string
	// WorkID 统计该作品现存存档中的调用；已删除存档的调用只计入全局累计。
	WorkID string
	NodeID string // 某个节点（回合或整理）中的调用
}

// Summary 是累计用量：合计，以及按调用用途、按模型分组的小计。
type Summary struct {
	Total Stats `json:"total"`
	// ByKind 以调用用途（narrate / continue / compact / helper / title / probe）为键，只含出现过的用途。
	ByKind map[string]Stats `json:"by_kind"`
	// ByModel 按调用次数从多到少排列。
	ByModel []ModelStats `json:"by_model"`
}

// ModelStats 是一个模型的小计。
type ModelStats struct {
	ModelID string `json:"model_id"`
	// Name 是模型的显示名（未设置时为供应商侧模型 ID）；模型已删除或不在模型表中（如假模型）时为空。
	Name  string `json:"name,omitzero"`
	Stats Stats  `json:"stats"`
}

// Service 提供用量统计。
type Service struct {
	db *store.DB
}

// NewService 创建用量统计服务。
func NewService(db *store.DB) *Service { return &Service{db: db} }

// Summarize 统计累计用量（读连接）。
func (s *Service) Summarize(ctx context.Context, f Filter) (*Summary, error) {
	return Summarize(ctx, s.db.Read(), f)
}

// Summarize 统计 f 选中的调用：先在库中按（用途, 模型, 货币, 是否可估算）分组求和，再合并为合计与各分组小计。
func Summarize(ctx context.Context, q store.Querier, f Filter) (*Summary, error) {
	var where []string
	var args []any
	if f.SaveID != "" {
		where, args = append(where, "u.save_id = ?"), append(args, f.SaveID)
	}
	if f.WorkID != "" {
		where, args = append(where, "u.save_id IN (SELECT id FROM saves WHERE work_id = ?)"), append(args, f.WorkID)
	}
	if f.NodeID != "" {
		where, args = append(where, "u.node_id = ?"), append(args, f.NodeID)
	}
	cond := ""
	if len(where) > 0 {
		cond = "WHERE " + strings.Join(where, " AND ")
	}
	rows, err := q.QueryContext(ctx, `
		SELECT u.call_kind, u.model_id, COALESCE(MAX(NULLIF(m.display_name, '')), MAX(m.model), ''),
		       COALESCE(u.currency, ''), u.cost_micros IS NULL, COUNT(*),
		       COALESCE(SUM(u.input_uncached), 0), COALESCE(SUM(u.cache_read), 0), COALESCE(SUM(u.cache_write), 0),
		       COALESCE(SUM(u.output), 0), COALESCE(SUM(u.reasoning), 0), COALESCE(SUM(u.cost_micros), 0)
		FROM usage_log u LEFT JOIN models m ON m.id = u.model_id
		`+cond+`
		GROUP BY u.call_kind, u.model_id, u.currency, u.cost_micros IS NULL`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sum := &Summary{Total: Stats{Costs: map[string]int64{}}, ByKind: map[string]Stats{}, ByModel: []ModelStats{}}
	models := map[string]*ModelStats{}
	for rows.Next() {
		var (
			kind, model, name, currency string
			unpriced                    bool
			calls                       int
			cost                        int64
			st                          Stats
		)
		if err := rows.Scan(&kind, &model, &name, &currency, &unpriced, &calls, &st.Usage.InputUncached, &st.Usage.CacheRead,
			&st.Usage.CacheWrite, &st.Usage.Output, &st.Usage.Reasoning, &cost); err != nil {
			return nil, err
		}
		st.Calls = calls
		if unpriced {
			st.Unpriced = calls
		} else {
			st.Costs = map[string]int64{cmp.Or(currency, llm.DefaultCurrency): cost}
		}
		sum.Total.merge(st)
		k := sum.ByKind[kind]
		k.merge(st)
		sum.ByKind[kind] = k
		ms := models[model]
		if ms == nil {
			ms = &ModelStats{ModelID: model, Name: name}
			models[model] = ms
		}
		ms.Stats.merge(st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, ms := range models {
		sum.ByModel = append(sum.ByModel, *ms)
	}
	slices.SortFunc(sum.ByModel, func(a, b ModelStats) int {
		return cmp.Or(cmp.Compare(b.Stats.Calls, a.Stats.Calls), strings.Compare(a.ModelID, b.ModelID))
	})
	return sum, nil
}
