package contextx

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/tmpl"
	"github.com/wahhheee/aint/internal/work"
)

// token 估算（《02-技术方案》8.5）：CJK 字符 × 1.0 + 其他字符 × 0.3；图片按 Anthropic 的公式（宽 × 高 ÷ 750，
// 先缩放到长边 1568 以内）。估算只用于校验报告与界面提示，计费以 API 返回为准。
const (
	cjkTokenRatio   = 1.0
	otherTokenRatio = 0.3
	imageMaxEdge    = 1568
)

// EstimateTokens 估算一段文字的 token 数。
func EstimateTokens(s string) int {
	var cjk, other int
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul),
			r >= 0x3000 && r <= 0x303F, r >= 0xFF00 && r <= 0xFFEF:
			cjk++
		default:
			other++
		}
	}
	return int(math.Ceil(float64(cjk)*cjkTokenRatio + float64(other)*otherTokenRatio))
}

// EstimateImageTokens 估算一张图片发送给模型后的 token 数。
func EstimateImageTokens(w, h int) int {
	if w <= 0 || h <= 0 {
		return 0
	}
	if long := max(w, h); long > imageMaxEdge {
		w, h = w*imageMaxEdge/long, h*imageMaxEdge/long
	}
	return int(math.Ceil(float64(w*h) / 750))
}

// messageOverhead 是每条消息、每个内容块在协议中的额外开销（角色、分隔、工具调用的结构）的粗略估算。
const messageOverhead = 4

// EstimateMessages 估算一组消息的 token 数：文字、工具调用（名称与参数）、工具结果、图片。
func EstimateMessages(msgs []llm.Message) int {
	n := 0
	for _, m := range msgs {
		n += messageOverhead + EstimateParts(m.Parts)
	}
	return n
}

// EstimateParts 估算一组内容块的 token 数。
func EstimateParts(parts []llm.Part) int {
	n := 0
	for _, p := range parts {
		n += messageOverhead
		switch p.Kind {
		case llm.PartText, llm.PartReasoning:
			n += EstimateTokens(p.Text)
		case llm.PartImage:
			if p.Image != nil {
				n += EstimateImageTokens(p.Image.Width, p.Image.Height)
			}
		case llm.PartToolCall:
			if p.ToolCall != nil {
				n += EstimateTokens(p.ToolCall.Name) + EstimateTokens(string(p.ToolCall.Args))
			}
		case llm.PartToolResult:
			if p.ToolResult != nil {
				n += EstimateParts(p.ToolResult.Parts)
			}
		}
	}
	return n
}

// EstimateRequest 估算一次请求的输入 token 数：系统提示、工具定义与全部消息。
func EstimateRequest(req *llm.Request) int {
	n := 0
	for _, b := range req.System {
		n += EstimateTokens(b.Text)
	}
	if len(req.Tools) > 0 {
		n += EstimateTokens(string(jsonx.MustMarshal(req.Tools)))
	}
	return n + EstimateMessages(req.Messages)
}

// 常驻部分的建议上限（token，《02-技术方案》5.3）。
const (
	maxIndexTokens  = 6000
	maxIndexEntries = 150
	maxPrefixTokens = 20000
)

// ToolSpecs 按能力画像返回工具定义（tools.Registry.Specs）；由调用方提供，本包不依赖 tools。
type ToolSpecs func(fs prompts.FeatureSet, m *work.Model) []llm.ToolSpec

// Analyze 估算作品常驻部分（每回合都会发送、可被缓存的前缀）的 token，写入报告（I-TOKENS），
// 设定索引或常驻部分过大时给出 W-INDEX-LARGE / W-PREFIX-LARGE。
//
// 以作品"典型"的游玩条件组装：作品声明需要看图时按能看图的模型计算，不开后台助手；初始状态取默认值。
// 只在报告没有 error 时调用（有 error 的作品无法组装）。
func Analyze(m *work.Model, rep *work.Report, specs ToolSpecs) {
	caps := llm.Caps{Vision: m.Manifest.Requires.Vision != "none", Tools: true, ImageMaxEdge: imageMaxEdge}
	fs := Profile(m, save.Settings{}, caps, false)
	model := &llm.Model{Caps: caps, MaxOutput: 8000}
	st := m.State.Defaults()
	built, err := NewBuilder(tmpl.New(), nil).Build(context.Background(), BuildInput{
		SaveID: "estimate", Work: m, Features: fs, Model: model, InitialState: st, InitialStateHash: "estimate",
		Epoch: RenderEpoch0(fs, m.State, st, nil),
		Tools: specs(fs, m), Turn: llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("")}},
	})
	if err != nil {
		rep.Warnf(work.CodeTokens, "", 0, "", "无法估算常驻部分的 token：%v", err)
		return
	}
	t := &work.TokenEstimate{}
	t.Platform = EstimateTokens(built.Request.System[0].Text)
	t.Work = EstimateTokens(built.Request.System[1].Text)
	index := renderLoreIndex(m, fs)
	if fs.Has(prompts.FLoreIndex) {
		t.Index = EstimateTokens(index)
	}
	t.Tools = EstimateTokens(string(jsonx.MustMarshal(built.Request.Tools)))
	if len(built.Request.Messages) > 1 { // 第一条是 L3 + L4，最后一条是本回合消息
		for _, p := range built.Request.Messages[0].Parts {
			t.Resident += EstimateTokens(p.Text)
		}
	}
	if fs.Has(prompts.FVision) {
		for _, a := range m.PinnedImages() {
			t.Images += EstimateImageTokens(a.Width, a.Height)
		}
	}
	t.Total = t.Platform + t.Work + t.Tools + t.Resident + t.Images
	rep.Tokens = t

	msg := fmt.Sprintf("常驻部分约 %s token：平台提示词 %s、作品指令 %s", thousands(t.Total), thousands(t.Platform), thousands(t.Work))
	if t.Index > 0 {
		msg += fmt.Sprintf("（其中设定索引 %s）", thousands(t.Index))
	}
	msg += fmt.Sprintf("、工具定义 %s、常驻资料 %s", thousands(t.Tools), thousands(t.Resident))
	if t.Images > 0 {
		msg += fmt.Sprintf("、常驻图片 %s", thousands(t.Images))
	}
	rep.Infof(work.CodeTokens, "", 0, "每回合都会发送，可被缓存；按 CJK 字符 1 token、其他字符 0.3 token 估算", "%s", msg)

	entries := 0
	for _, line := range strings.Split(index, "\n") {
		if strings.HasPrefix(line, "- ") {
			entries++
		}
	}
	if t.Index > maxIndexTokens || entries > maxIndexEntries {
		rep.Warnf(work.CodeIndexLarge, "lore", 0, "大量细碎条目用目录摘要（_index.md）配合 load: none，设定索引只显示目录级摘要",
			"设定索引较大（约 %s token、%d 条）", thousands(t.Index), entries)
	}
	if t.Total > maxPrefixTokens {
		rep.Warnf(work.CodePrefixLarge, "", 0, "把偶尔才需要的设定改为 load: index，减少常驻图片，精简执笔指令",
			"常驻部分较大（约 %s token，建议不超过 %s）", thousands(t.Total), thousands(maxPrefixTokens))
	}
}

// thousands 给整数加千位分隔符。
func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
