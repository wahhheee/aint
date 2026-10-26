package settings

import (
	"math"
	"net"
	"slices"

	"github.com/wahhheee/aint/internal/pkg/errs"
)

// Validate 检查完整设置；拒绝非法值，避免界面或模型请求落入无法操作的状态。
func (s Settings) Validate() error {
	if net.ParseIP(s.Listen) == nil {
		return errs.Invalid("监听地址应为 IP 地址，例如 127.0.0.1 或 0.0.0.0")
	}
	choices := []struct {
		name, value string
		allowed     []string
	}{
		{"外观", s.Theme, []string{"system", "light", "dark"}},
		{"字体", s.Reading.FontFamily, []string{"work", "serif", "sans"}},
		{"缩进", s.Reading.Indent, []string{"work", "on", "off"}},
		{"打字机", s.Reading.Typewriter, []string{"instant", "normal", "slow"}},
		{"减少动画", s.Reading.ReduceMotion, []string{"system", "on", "off"}},
		{"篇幅", s.Play.Pace, []string{"short", "medium", "long"}},
		{"推理", s.Play.Reasoning, []string{"off", "low", "medium", "high"}},
		{"缓存", s.Play.CacheTTL, []string{"short", "long"}},
		{"退出标题", s.Play.TitleOnExit, []string{"auto", "ask", "off"}},
		{"整理方式", s.Context.Mode, []string{"inline", "separate"}},
	}
	for _, c := range choices {
		if !slices.Contains(c.allowed, c.value) {
			return errs.Invalid("%s设置不正确", c.name)
		}
	}
	ranges := []struct {
		name            string
		value, min, max float64
	}{
		{"字号", float64(s.Reading.FontSize), 14, 24}, {"行距", s.Reading.LineHeight, 1.5, 2.4},
		{"段距", s.Reading.ParagraphSpacing, 0, 3}, {"版心", s.Reading.MaxWidth, 32, 52},
		{"背景强度", s.Reading.StageIntensity, 0, 1}, {"总音量", s.Audio.Master, 0, 1}, {"音乐音量", s.Audio.BGM, 0, 1}, {"音效音量", s.Audio.SFX, 0, 1},
		{"模型调用上限", float64(s.Context.MaxCalls), 0, 50}, {"工具调用上限", float64(s.Context.MaxToolCalls), 0, 200}, {"单次输出上限", float64(s.Context.MaxOutput), 0, 128_000},
		{"上下文预算", float64(s.Context.Budget), 1000, 2_000_000}, {"保留原文", float64(s.Context.KeepRecent), 0, 2_000_000},
	}
	for _, r := range ranges {
		if math.IsNaN(r.value) || math.IsInf(r.value, 0) || r.value < r.min || r.value > r.max {
			return errs.Invalid("%s超出范围（%g–%g）", r.name, r.min, r.max)
		}
	}
	if (s.Play.Helpers || s.Play.TitleOnExit != "off") && s.Models.Light == "" {
		return errs.Invalid("后台任务需要先配置轻量模型")
	}
	return nil
}
