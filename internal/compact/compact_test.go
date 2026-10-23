package compact

import "testing"

func TestThreshold(t *testing.T) {
	p := DefaultParams
	// 128k 窗口、32k 输出：reserve = 32k + 8k，阈值 = 窗口 − reserve
	if got := p.Threshold(128_000, 32_000); got != 88_000 {
		t.Fatalf("128k 窗口的阈值 = %d", got)
	}
	// 大窗口：受预算限制
	if got := p.Threshold(1_000_000, 32_000); got != 128_000 {
		t.Fatalf("1M 窗口的阈值 = %d", got)
	}
	if got := p.With(20_000, 0, "").Threshold(200_000, 32_000); got != 20_000 {
		t.Fatalf("存档预算 20k 的阈值 = %d", got)
	}
	if got := p.HardLimit(200_000, 32_000); got != 158_000 {
		t.Fatalf("硬上限 = %d", got)
	}
	if got := p.KeepTokens(20_000); got != 6_000 {
		t.Fatalf("保留原文不超过阈值的 30%% = %d", got)
	}
	if got := p.SummaryChars(20_000); got != 2_000 {
		t.Fatalf("摘要目标不超过阈值的 10%% = %d", got)
	}
	if p.With(0, 0, "bogus").Mode != ModeInline {
		t.Fatal("未知的整理方式沿用原值")
	}
}

func TestKeepFrom(t *testing.T) {
	cases := []struct {
		sizes []int
		keep  int
		want  int
	}{
		{[]int{100, 100, 100, 100, 100}, 250, 2}, // 从末尾累计到 ≥ 250：保留最后 3 个
		{[]int{100, 100, 100, 100, 100}, 50, 3},  // 至少保留最后 2 个
		{[]int{100, 100}, 50, 0},                 // 只有 2 个回合：没有可以摘要掉的
		{[]int{5000, 100, 100}, 50, 1},           // 以回合为界，不切分回合
		{[]int{100, 100, 100}, 10_000, 0},        // 全部加起来也不到保留量
	}
	for _, c := range cases {
		if got := KeepFrom(c.sizes, c.keep); got != c.want {
			t.Errorf("KeepFrom(%v, %d) = %d，期望 %d", c.sizes, c.keep, got, c.want)
		}
	}
}
