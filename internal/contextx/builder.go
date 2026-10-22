package contextx

import (
	"fmt"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/pkg/lru"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/store"
	"github.com/wahhheee/yanyi/internal/tmpl"
	"github.com/wahhheee/yanyi/internal/work"
)

// Builder 构建模型请求。组装结果（L0、L1、L3）按稳定输入缓存。并发安全。
type Builder struct {
	tmpl *tmpl.Engine

	cache *lru.Cache[string, *prefix]
}

// prefixCacheSize 是前缀组装结果的缓存条数：每个（作品版本、能力画像、初始状态）一条，
// 也就是大致每个最近游玩的存档一条；淘汰后下次请求重新组装（结果相同，不影响前缀缓存）。
const prefixCacheSize = 64

// prefix 是稳定前缀的组装结果。
type prefix struct {
	system    []llm.SystemBlock
	l3        []llm.Part // 首条 user 消息开头的常驻资料；为空表示没有 L3
	fragments []FragmentReport
}

// NewBuilder 创建构建器。
func NewBuilder(tm *tmpl.Engine) *Builder {
	return &Builder{tmpl: tm, cache: lru.New[string, *prefix](prefixCacheSize)}
}

// BuildInput 是构建一次请求所需的全部输入。
type BuildInput struct {
	SaveID           string
	Work             *work.Model
	Features         prompts.FeatureSet
	Model            *llm.Model
	Settings         save.Settings
	InitialState     map[string]any // 存档创建时的状态（执笔指令中的 Liquid 以它渲染）
	InitialStateHash string
	Tools            []llm.ToolSpec // 已按能力画像裁切（tools.Registry.Specs）
	History          []llm.Message  // 本阶段已提交的消息（发送形态，只读）
	Epoch            string         // L4 阶段块；TODO(M3-6)
	Turn             llm.Message    // 本回合玩家消息（Envelope.Message）
}

// Built 是构建结果。
type Built struct {
	Request    *llm.Request
	Fragments  []FragmentReport
	PrefixHash string // L0–L3 + 工具；与上一请求不同即缓存重建
}

// Build 构建请求：system=[L0][L1]，tools=L2，messages=[L3+L4]…历史…本回合。
func (b *Builder) Build(in BuildInput) (*Built, error) {
	p, err := b.prefix(in)
	if err != nil {
		return nil, err
	}

	var msgs []llm.Message
	// L3（常驻资料）+ L4（阶段块）合为开头的一条 user 消息；均为空时不生成（也不放断点 ①②）。
	var head []llm.Part
	if len(p.l3) > 0 {
		head = append(head, p.l3...)
		head[len(head)-1].CacheMark = true // 断点 ①
	}
	if in.Epoch != "" {
		head = append(head, llm.Part{Kind: llm.PartText, Text: in.Epoch, CacheMark: true}) // 断点 ②
	}
	if len(head) > 0 {
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Parts: head})
	}
	// 历史与本回合消息不打标记：CacheMark 只标稳定层的边界（L1、L3、L4 末尾），
	// "最新消息末尾"的滚动断点由适配器按协议放置（Anthropic 显式、OpenAI 隐式断点）；相邻同角色消息由适配器合并。
	msgs = append(msgs, in.History...)
	msgs = append(msgs, in.Turn)

	maxOut := min(in.Model.MaxOutput, 32_000)
	if maxOut <= 0 {
		maxOut = 32_000
	}
	req := &llm.Request{
		System:    p.system,
		Tools:     in.Tools,
		Messages:  msgs,
		MaxOutput: maxOut,
		Reasoning: llm.ReasoningLevel(in.Settings.Reasoning),
		CacheKey:  "yanyi:" + in.SaveID,
		CacheTTL:  in.Settings.CacheTTL,
		Purpose:   "narrate",
	}
	hash := store.HashBytes(jsonx.MustMarshal(struct {
		System []llm.SystemBlock `json:"system"`
		Tools  []llm.ToolSpec    `json:"tools"`
		L3     []llm.Part        `json:"l3"`
	}{p.system, in.Tools, p.l3}))
	return &Built{Request: req, Fragments: p.fragments, PrefixHash: hash}, nil
}

func (b *Builder) prefix(in BuildInput) (*prefix, error) {
	key := strings.Join([]string{prompts.Version, in.Work.VersionID, in.Features.Signature(), in.InitialStateHash}, "|")
	if p, ok := b.cache.Get(key); ok {
		return p, nil
	}

	l0, reports, err := renderFragments("L0", prompts.L0, in.Features, "##")
	if err != nil {
		return nil, err
	}
	narrator, err := b.tmpl.RenderWith("prompts/narrator.md", in.Work.Narrator, map[string]any{"state": in.InitialState}, in.Work.Partials)
	if err != nil {
		return nil, fmt.Errorf("执笔指令模板渲染失败: %w", err)
	}
	l1, l1Reports := assembleL1(in.Work, in.Features, narrator)
	reports = append(reports, l1Reports...)

	p := &prefix{
		system:    []llm.SystemBlock{{Text: l0}, {Text: l1, CacheMark: true}},
		l3:        renderL3(in.Work),
		fragments: reports,
	}
	b.cache.Put(key, p)
	return p, nil
}

// renderL3 渲染常驻资料：load: always 的设定文档正文（按路径排序）。
// TODO(M2-10)：常驻图片（pin: true 的素材、pin_images 的文档图片）。
func renderL3(m *work.Model) []llm.Part {
	var b strings.Builder
	for _, d := range m.Docs {
		if d.Load != "always" || d.IsIndex {
			continue
		}
		fmt.Fprintf(&b, "<lore path=\"/work/%s\" title=\"%s\">\n%s\n</lore>\n", d.Path, d.Title, strings.TrimSpace(d.Body))
	}
	if b.Len() == 0 {
		return nil
	}
	return []llm.Part{llm.Text(strings.TrimSpace(b.String()))}
}
