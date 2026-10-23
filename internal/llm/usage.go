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
// 任一需要的单价为 nil 时，费用无法估算（界面显示"未配置价格"，不显示 0）。
//
// 缓存写入分两档，对应存档设置的缓存保留时长（Request.CacheTTL）：
// short = Anthropic 5 分钟 / OpenAI 30 分钟；long = Anthropic 1 小时 / OpenAI 12 小时。
type Price struct {
	Input          *float64 `json:"input,omitzero"`
	CacheRead      *float64 `json:"cache_read,omitzero"`
	CacheWrite     *float64 `json:"cache_write,omitzero"`
	CacheWriteLong *float64 `json:"cache_write_long,omitzero"`
	Output         *float64 `json:"output,omitzero"`
	Currency       string   `json:"currency,omitzero"` // ISO 4217，如 "CNY"、"USD"；为空按 DefaultCurrency
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

// Scaled 返回乘以倍率后的价格（中转站的分组倍率，D-48）。倍率 ≤ 0 或为 1 时原样返回。
func (p *Price) Scaled(multiplier float64) *Price {
	if p == nil || multiplier <= 0 || multiplier == 1 {
		return p
	}
	scale := func(v *float64) *float64 {
		if v == nil {
			return nil
		}
		x := *v * multiplier
		return &x
	}
	return &Price{
		Input: scale(p.Input), CacheRead: scale(p.CacheRead), CacheWrite: scale(p.CacheWrite),
		CacheWriteLong: scale(p.CacheWriteLong), Output: scale(p.Output), Currency: p.Currency,
	}
}

// CostMicros 估算费用（百万分之一货币单位）。ok=false 表示单价缺失，无法估算。
// cacheTTL 决定缓存写入使用哪档单价（CacheLong 用 CacheWriteLong，其余用 CacheWrite）。
func (p *Price) CostMicros(u Usage, cacheTTL string) (micros int64, ok bool) {
	if p == nil {
		return 0, false
	}
	write := p.CacheWrite
	if cacheTTL == CacheLong {
		write = p.CacheWriteLong
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
