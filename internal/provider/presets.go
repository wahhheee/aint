package provider

import (
	"maps"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
)

// Preset 是内置的模型预设：添加模型时按名字匹配，预填上下文窗口、输出上限、能力与官方价格（用户可修改）。
//
// 能力与推理映射的依据见 docs/notes/api-verification.md；中转站的价格因站点而异，预设只给官方价格，
// 中转站请在模型设置中填写站点标价，并在供应商上设置分组倍率（D-48）。
type Preset struct {
	// Match 是模型名前缀（如 "claude-sonnet-5-5"）；按最长前缀匹配。
	Match         string
	DisplayName   string
	ContextWindow int
	MaxOutput     int
	Caps          llm.Caps
	// ReasoningBy 按接入协议覆盖 Caps.ReasoningMap（同一模型经不同兼容接口接入时推理参数写法不同）。
	ReasoningBy map[llm.Protocol]map[string]string
	Price       *llm.Price
}

func usd(v float64) *float64 { return &v }

func price(cur string, input, cacheRead, cacheWrite, cacheWriteLong, output float64) *llm.Price {
	p := &llm.Price{Input: usd(input), Output: usd(output), CacheRead: usd(cacheRead), Currency: cur}
	if cacheWrite > 0 {
		p.CacheWrite = usd(cacheWrite)
	}
	if cacheWriteLong > 0 {
		p.CacheWriteLong = usd(cacheWriteLong)
	}
	return p
}

// claudeCaps 是 Claude 4.6 起的通用能力（adaptive thinking + effort）。
func claudeCaps(off string) llm.Caps {
	return llm.Caps{
		Vision: true, Tools: true, ParallelTools: true, ImageInToolResult: true, Reasoning: "effort", Cache: "breakpoints",
		ImageMaxEdge: 1568,
		ReasoningMap: map[string]string{"off": off, "low": "adaptive:low", "medium": "adaptive:medium", "high": "adaptive:high"},
	}
}

// Presets 是内置模型预设（2026-10 核对）。
var Presets = []Preset{
	// Anthropic：Sonnet 5.5 不能 disabled，关闭思考用 between_tools；Opus 5.5 / Fable 5.1 思考不能关闭，退到 low。
	{Match: "claude-sonnet-5-5", DisplayName: "Claude Sonnet 5.5", ContextWindow: 1_000_000, MaxOutput: 128_000,
		Caps: claudeCaps("between_tools"), Price: price("USD", 2, 0.2, 2.5, 4, 10)},
	{Match: "claude-opus-5-5", DisplayName: "Claude Opus 5.5", ContextWindow: 1_000_000, MaxOutput: 128_000,
		Caps: claudeCaps("adaptive:low"), Price: price("USD", 4, 0.2, 5, 8, 20)},
	{Match: "claude-haiku-5-5", DisplayName: "Claude Haiku 5.5", ContextWindow: 1_000_000, MaxOutput: 128_000,
		Caps: claudeCaps("disabled"), Price: price("USD", 0.1, 0.01, 0.125, 0.2, 0.5)},
	{Match: "claude-fable-5-1", DisplayName: "Claude Fable 5.1", ContextWindow: 1_000_000, MaxOutput: 128_000,
		Caps: claudeCaps("adaptive:low"), Price: price("USD", 10, 0.25, 12.5, 20, 50)},

	// OpenAI：GPT-5.6 起支持显式缓存断点与 30m / 12h 缓存保留；推理 effort 支持 none。
	{Match: "gpt-6", DisplayName: "", ContextWindow: 400_000, MaxOutput: 128_000,
		Caps: llm.Caps{Vision: true, Tools: true, ParallelTools: true, ImageInToolResult: true, Reasoning: "effort", Cache: "breakpoints",
			ImageMaxEdge: 2048, ReasoningMap: map[string]string{"off": "none", "low": "low", "medium": "medium", "high": "high"}}},
	{Match: "gpt-5.6", DisplayName: "", ContextWindow: 400_000, MaxOutput: 128_000,
		Caps: llm.Caps{Vision: true, Tools: true, ParallelTools: true, ImageInToolResult: true, Reasoning: "effort", Cache: "breakpoints",
			ImageMaxEdge: 2048, ReasoningMap: map[string]string{"off": "none", "low": "low", "medium": "medium", "high": "high"}}},

	// Gemini：3.8 / 4 系列思考档位只有 low / medium / high，关闭时退到 low；只有隐式缓存。
	{Match: "gemini-3.8-flash", DisplayName: "Gemini 3.8 Flash", ContextWindow: 1_048_576, MaxOutput: 65_536,
		Caps: llm.Caps{Vision: true, Tools: true, ParallelTools: true, ImageInToolResult: true, Reasoning: "level", Cache: "implicit",
			ImageMaxEdge: 1568, ReasoningMap: map[string]string{"off": "low", "low": "low", "medium": "medium", "high": "high"}}},
	{Match: "gemini-4-argon", DisplayName: "Gemini 4 Argon", ContextWindow: 1_048_576, MaxOutput: 65_536,
		Caps: llm.Caps{Vision: true, Tools: true, ParallelTools: true, ImageInToolResult: true, Reasoning: "level", Cache: "implicit",
			ImageMaxEdge: 1568, ReasoningMap: map[string]string{"off": "low", "low": "low", "medium": "medium", "high": "high"}}},

	// DeepSeek（官方价格为高峰时段人民币价；项目不区分峰谷，按高峰计）：effort 只有 low / high / max，缓存为自动的硬盘缓存。
	// Anthropic 兼容接口的 thinking 可以关闭；Responses 兼容接口只认 effort。
	{Match: "deepseek-flash", DisplayName: "DeepSeek V4.1 Flash", ContextWindow: 1_048_576, MaxOutput: 393_216,
		Caps: llm.Caps{Vision: true, Tools: true, ParallelTools: true, ImageInToolResult: true, Reasoning: "effort", Cache: "implicit",
			ImageMaxEdge: 1568},
		ReasoningBy: deepseekReasoning, Price: price("CNY", 2, 0.04, 0, 0, 8)},
	{Match: "deepseek-v4-pro", DisplayName: "DeepSeek V4 Pro", ContextWindow: 1_048_576, MaxOutput: 393_216,
		Caps:        llm.Caps{Tools: true, ParallelTools: true, Reasoning: "effort", Cache: "implicit"},
		ReasoningBy: deepseekReasoning, Price: price("CNY", 9, 0.3, 0, 0, 27)},
}

var deepseekReasoning = map[llm.Protocol]map[string]string{
	llm.ProtocolAnthropic:       {"off": "disabled", "low": "effort:low", "medium": "effort:high", "high": "effort:high"},
	llm.ProtocolOpenAIResponses: {"off": "low", "low": "low", "medium": "high", "high": "high"},
}

// PresetFor 返回与模型名最匹配的预设（最长前缀）；没有匹配时 ok=false。返回的是副本，可以修改。
func PresetFor(protocol llm.Protocol, model string) (Preset, bool) {
	best, found := Preset{}, false
	for _, p := range Presets {
		if strings.HasPrefix(model, p.Match) && len(p.Match) > len(best.Match) {
			best, found = p, true
		}
	}
	if !found {
		return Preset{}, false
	}
	src := best.Caps.ReasoningMap
	if m, ok := best.ReasoningBy[protocol]; ok {
		src = m
	}
	best.Caps.ReasoningMap = maps.Clone(src)
	best.ReasoningBy = nil
	return best, true
}
