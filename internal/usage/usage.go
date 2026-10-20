// Package usage 记录与统计模型用量（usage_log 表）。
//
// 每次模型调用一条记录，与回合提交在同一事务中写入（原子提交，不变量 2）。
// 费用在写入时按当时的单价计算并固化（cost_micros 为 NULL 表示无法估算，界面显示"无法估算"）。
package usage

import (
	"context"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/store"
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

// Stats 是累计用量（存档 / 作品 / 全局）。
type Stats struct {
	Usage llm.Usage `json:"usage"`
	// Costs 按货币分别累计（D-34），如 {"USD": 210000, "CNY": 100000}；不做汇率换算。
	Costs map[string]int64 `json:"costs"`
	// Unpriced 是无法估算费用的调用次数（界面提示"部分调用无法估算"）。
	Unpriced int `json:"unpriced"`
	Calls    int `json:"calls"`
}

// Summarize 统计累计用量。
//
// TODO(M4-4)：按 save_id / work_id / 全局聚合；缓存命中占比；用于顶栏用量浮层与统计页。
func Summarize(ctx context.Context, q store.Querier, saveID, workID string) (*Stats, error) {
	return nil, errs.NotImplemented("用量统计")
}
