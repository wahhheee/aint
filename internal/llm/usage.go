package llm

import "math"

// Usage 是归一化后的用量，五项互斥（《02-技术方案》7.3）。
type Usage struct {
	InputUncached int `json:"input_uncached"`
	CacheRead     int `json:"cache_read"`
	CacheWrite    int `json:"cache_write"`
	Output        int `json:"output"`
	Reasoning     int `json:"reasoning"`
}

// Add 累加用量（一个回合可能包含多次调用）。
func (u Usage) Add(o Usage) Usage {
	return Usage{
		InputUncached: u.InputUncached + o.InputUncached,
		CacheRead:     u.CacheRead + o.CacheRead,
		CacheWrite:    u.CacheWrite + o.CacheWrite,
		Output:        u.Output + o.Output,
		Reasoning:     u.Reasoning + o.Reasoning,
	}
}

// InputTotal 是全部输入 token（缓存命中率 = CacheRead / InputTotal）。
func (u Usage) InputTotal() int { return u.InputUncached + u.CacheRead + u.CacheWrite }

// Price 是每百万 token 的价格，单位为 Currency（D-34：费用按货币分别汇总，不做汇率换算）。
// 任一需要的单价为 nil 时，费用无法估算（界面显示"无法估算"，不显示 0）。
type Price struct {
	Input        *float64 `json:"input,omitzero"`
	CacheRead    *float64 `json:"cache_read,omitzero"`
	CacheWrite5m *float64 `json:"cache_write_5m,omitzero"`
	CacheWrite1h *float64 `json:"cache_write_1h,omitzero"`
	Output       *float64 `json:"output,omitzero"`
	Currency     string   `json:"currency,omitzero"` // ISO 4217，如 "CNY"、"USD"；为空按 DefaultCurrency
}

// DefaultCurrency 是价格未写货币时的默认值（官方 API 多以美元计价）。
const DefaultCurrency = "USD"

// CurrencyOrDefault 返回计费货币。
func (p *Price) CurrencyOrDefault() string {
	if p == nil || p.Currency == "" {
		return DefaultCurrency
	}
	return p.Currency
}

// CostMicros 估算费用（百万分之一货币单位）。ok=false 表示单价缺失，无法估算。
// cacheTTL 决定缓存写入使用哪档单价（"1h" 或其他）。
func (p *Price) CostMicros(u Usage, cacheTTL string) (micros int64, ok bool) {
	if p == nil {
		return 0, false
	}
	write := p.CacheWrite5m
	if cacheTTL == "1h" {
		write = p.CacheWrite1h
	}
	items := []struct {
		n     int
		price *float64
	}{
		{u.InputUncached, p.Input},
		{u.CacheRead, p.CacheRead},
		{u.CacheWrite, write},
		{u.Output + u.Reasoning, p.Output},
	}
	var total float64
	for _, it := range items {
		if it.n == 0 {
			continue
		}
		if it.price == nil {
			return 0, false
		}
		total += float64(it.n) * *it.price // 每百万 token 价格 × token 数 = 微单位
	}
	return int64(math.Round(total)), true
}
