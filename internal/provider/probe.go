package provider

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/store"
	"github.com/wahhheee/aint/internal/usage"
)

// TestStep 是连接测试中的一项。
type TestStep struct {
	Name      string `json:"name"` // list_models | text | tool | image
	OK        bool   `json:"ok"`
	Skipped   bool   `json:"skipped,omitzero"`
	Message   string `json:"message"`         // 面向玩家的结论
	Detail    string `json:"detail,omitzero"` // 原始错误或返回内容（排查用，不含密钥）
	LatencyMS int64  `json:"latency_ms,omitzero"`
}

// TestReport 是连接测试的结果。OK 表示必需的项（文本、工具往返）都通过。
type TestReport struct {
	OK    bool       `json:"ok"`
	Model string     `json:"model"`
	Steps []TestStep `json:"steps"`
}

// TestInput 指定用哪个模型测试：已添加的模型 ID，或尚未添加的模型名（按预设推断能力）；都为空时用该供应商的第一个模型。
type TestInput struct {
	ModelID string `json:"model_id,omitzero"`
	Model   string `json:"model,omitzero"`
}

// probeTimeout 是单项测试的超时。
const probeTimeout = 90 * time.Second

// Test 依次执行连接测试（《02-技术方案》7.5）：拉取模型列表 → 文本请求 → 工具调用往返 → 看图（模型能看图时）。
// 每项给出通过 / 失败与原始错误；结果记入供应商（设置页显示"测试通过 / 未测试"），用量记入 usage_log（call_kind = probe）。
func (s *Service) Test(ctx context.Context, providerID string, in TestInput) (*TestReport, error) {
	cfg, multiplier, err := s.loadProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	m, err := s.probeModel(ctx, cfg, in)
	if err != nil {
		return nil, err
	}
	m.Price = m.Price.Scaled(multiplier)

	// 测试使用独立的适配器实例：图片来自内存，不经过缓存的连接。
	pc := cfg
	pc.Blobs = probeBlobs
	adapter, err := NewAdapter(pc)
	if err != nil {
		return nil, err
	}
	p := llm.WithRetry(adapter, llm.RetryPolicy{MaxRetries: 1, BaseDelay: 2 * time.Second, MaxDelay: 10 * time.Second})
	if s.opts.TraceDir != "" {
		p = llm.WithTrace(p, s.opts.TraceDir)
	}
	r := &probeRun{s: s, p: p, m: m}
	report := &TestReport{Model: m.Name}

	report.Steps = append(report.Steps, r.listModels(ctx, adapter))
	text := r.text(ctx)
	report.Steps = append(report.Steps, text)
	tool := TestStep{Name: "tool", Skipped: true, Message: "文本请求未通过，跳过"}
	if text.OK {
		tool = r.tool(ctx)
	}
	report.Steps = append(report.Steps, tool)
	img := TestStep{Name: "image", Skipped: true, Message: "模型未标记为能看图，跳过"}
	if m.Caps.Vision && text.OK {
		img = r.image(ctx)
	}
	report.Steps = append(report.Steps, img)
	report.OK = text.OK && tool.OK

	if err := s.exec(ctx, `UPDATE providers SET test_ok = ?, tested_at = ? WHERE id = ?`, report.OK, store.NowMillis(), providerID); err != nil {
		slog.WarnContext(ctx, "记录连接测试结果失败", "err", err)
	}
	return report, nil
}

// probeModel 确定测试用的模型配置。
func (s *Service) probeModel(ctx context.Context, cfg llm.ProviderConfig, in TestInput) (*llm.Model, error) {
	if in.ModelID != "" {
		m, err := s.loadModel(ctx, s.db.Read(), in.ModelID)
		if err != nil {
			return nil, err
		}
		if m.ProviderID != cfg.ID {
			return nil, errs.Invalid("模型不属于这个供应商")
		}
		return m, nil
	}
	name := strings.TrimSpace(in.Model)
	if name == "" {
		var id string
		err := s.db.Read().QueryRowContext(ctx, `SELECT id FROM models WHERE provider_id = ? ORDER BY COALESCE(display_name, model) LIMIT 1`, cfg.ID).Scan(&id)
		if err != nil {
			return nil, errs.Invalid("请先添加一个模型，或指定要测试的模型 ID")
		}
		return s.loadModel(ctx, s.db.Read(), id)
	}
	m := &llm.Model{ID: "probe:" + name, ProviderID: cfg.ID, Name: name, ContextWindow: 128_000, MaxOutput: 8192, Caps: fallbackCaps}
	if pre, ok := PresetFor(cfg.Protocol, name); ok {
		m.ContextWindow, m.MaxOutput, m.Caps, m.Price = pre.ContextWindow, pre.MaxOutput, pre.Caps, pre.Price
	}
	return m, nil
}

type probeRun struct {
	s *Service
	p llm.Provider
	m *llm.Model
}

func (r *probeRun) listModels(ctx context.Context, adapter llm.Provider) TestStep {
	st := TestStep{Name: "list_models"}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	start := time.Now()
	list, err := adapter.ListModels(ctx)
	st.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		// 不少兼容接口不提供模型列表，不影响使用：记为跳过而不是失败。
		st.Skipped, st.Message, st.Detail = true, "供应商没有提供模型列表（不影响使用，可以手动填写模型 ID）", err.Error()
		return st
	}
	st.OK = true
	st.Message = fmt.Sprintf("共 %d 个模型", len(list))
	if !slices.ContainsFunc(list, func(x llm.RemoteModel) bool { return x.ID == r.m.Name }) && len(list) > 0 {
		st.Message += fmt.Sprintf("，列表中没有 %s（部分供应商的列表不完整，以下面的实际调用为准）", r.m.Name)
	}
	return st
}

func (r *probeRun) text(ctx context.Context) TestStep {
	st := TestStep{Name: "text"}
	done, latency, err := r.call(ctx, &llm.Request{
		System:   []llm.SystemBlock{{Text: "你是连接测试助手。"}},
		Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("请只回复四个字：连接成功")}}},
	})
	st.LatencyMS = latency
	if err != nil {
		st.Message, st.Detail = describeError(err), err.Error()
		return st
	}
	text := strings.TrimSpace(done.Message.TextOf())
	st.Detail = clip(text, 200)
	if text == "" {
		st.Message = fmt.Sprintf("模型没有返回正文（停止原因：%s）", done.Raw)
		return st
	}
	st.OK, st.Message = true, "可以正常生成正文"
	return st
}

var probeEcho = llm.ToolSpec{
	Name:        "probe_echo",
	Description: "连接测试用的回声工具：返回一个校验码。",
	Schema:      jsonx.Value(`{"type":"object","properties":{"text":{"type":"string","description":"要回声的文字"}},"required":["text"]}`),
}

// tool 验证工具调用往返：模型调用 probe_echo → 回填结果（含随机校验码）→ 模型在最终回答中复述校验码。
// 不使用强制 tool_choice（部分模型不支持），靠指令引导。
func (r *probeRun) tool(ctx context.Context) TestStep {
	st := TestStep{Name: "tool"}
	code := fmt.Sprintf("%04d", rand.IntN(10000)) //nolint:gosec // G404：测试校验码不需要密码学随机数
	req := &llm.Request{
		System:   []llm.SystemBlock{{Text: "你是连接测试助手。用户要求调用工具时，必须调用工具。"}},
		Tools:    []llm.ToolSpec{probeEcho},
		Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("请调用 probe_echo 工具，参数 text 填「雾港」。拿到结果后，只回复结果中的校验码。")}}},
	}
	done, latency, err := r.call(ctx, req)
	st.LatencyMS = latency
	if err != nil {
		st.Message, st.Detail = describeError(err), err.Error()
		return st
	}
	calls := done.Message.ToolCalls()
	if len(calls) == 0 {
		st.Message, st.Detail = "模型没有调用工具（可能不支持工具调用）", clip(done.Message.TextOf(), 200)
		return st
	}
	req.Messages = append(req.Messages, done.Message)
	results := llm.Message{Role: llm.RoleUser}
	for _, c := range calls {
		results.Parts = append(results.Parts, llm.Part{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{
			CallID: c.ID, Parts: []llm.Part{llm.Text("回声：雾港。校验码：" + code)},
		}})
	}
	req.Messages = append(req.Messages, results)
	done, latency2, err := r.call(ctx, req)
	st.LatencyMS += latency2
	if err != nil {
		st.Message, st.Detail = "工具结果回填后出错："+describeError(err), err.Error()
		return st
	}
	text := done.Message.TextOf()
	st.Detail = clip(text, 200)
	if !strings.Contains(text, code) {
		st.Message = "工具调用成功，但模型没有用上工具结果"
		return st
	}
	st.OK, st.Message = true, "工具调用、结果回填与最终回答都正常"
	return st
}

// image 发送程序生成的色块图，询问颜色。
func (r *probeRun) image(ctx context.Context) TestStep {
	st := TestStep{Name: "image"}
	done, latency, err := r.call(ctx, &llm.Request{
		System: []llm.SystemBlock{{Text: "你是连接测试助手。"}},
		Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{
			{Kind: llm.PartImage, Image: &llm.ImageRef{Blob: probeImageHash, MIME: "image/png", Width: 64, Height: 64}},
			llm.Text("图片是什么颜色？只回答颜色。"),
		}}},
	})
	st.LatencyMS = latency
	if err != nil {
		st.Message, st.Detail = "看图请求出错（模型可能不支持图片，可在模型设置中关闭「能看图」）："+describeError(err), err.Error()
		return st
	}
	text := done.Message.TextOf()
	st.Detail = clip(text, 200)
	if !strings.Contains(text, "红") && !strings.Contains(strings.ToLower(text), "red") {
		st.Message = "模型没有认出图片颜色"
		return st
	}
	st.OK, st.Message = true, "可以识别图片"
	return st
}

// call 发起一次测试调用（推理关闭、输出上限较小），并记录用量。
func (r *probeRun) call(ctx context.Context, req *llm.Request) (llm.Done, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req.Purpose = "probe"
	req.MaxOutput = min(r.m.MaxOutput, 2048)
	req.Reasoning = llm.ReasoningOff
	start := time.Now()
	s, err := r.p.Stream(ctx, r.m, req)
	if err != nil {
		return llm.Done{}, time.Since(start).Milliseconds(), err
	}
	defer func() { _ = s.Close() }()
	var done *llm.Done
	for {
		ev, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return llm.Done{}, time.Since(start).Milliseconds(), err
		}
		if d, ok := ev.(llm.Done); ok {
			done = &d
		}
	}
	latency := time.Since(start).Milliseconds()
	if done == nil {
		return llm.Done{}, latency, errors.New("流在没有结束事件的情况下中断")
	}
	r.record(ctx, *done, latency)
	return *done, latency, nil
}

func (r *probeRun) record(ctx context.Context, d llm.Done, latency int64) {
	rec := usage.Record{CallKind: "probe", ModelID: r.m.ID, Usage: d.Usage, LatencyMS: latency, Stop: string(d.Stop)}
	if cost, ok := r.m.Price.CostMicros(d.Usage, llm.CacheShort); ok {
		rec.CostMicros, rec.Currency = &cost, r.m.Price.CurrencyOrDefault()
	}
	err := r.s.db.Tx(context.WithoutCancel(ctx), func(tx *sql.Tx) error { return usage.Insert(ctx, tx, rec) })
	if err != nil {
		slog.WarnContext(ctx, "记录连接测试用量失败", "err", err)
	}
}

// describeError 把模型调用错误写成玩家能看懂的话。
func describeError(err error) string {
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		return "地址不存在，请检查 Base URL 与模型 ID"
	}
	switch llm.KindOf(err) {
	case llm.ErrAuth:
		return "鉴权失败或余额不足，请检查 API Key"
	case llm.ErrRateLimited:
		return "请求太频繁或额度用尽，请稍后再试"
	case llm.ErrOverloaded, llm.ErrServer:
		return "供应商暂时不可用"
	case llm.ErrNetwork:
		return "连接不上供应商，请检查 Base URL 与网络"
	case llm.ErrInvalidRequest:
		return "请求被拒绝，请检查模型 ID 与 Base URL（详情见下方）"
	case llm.ErrCanceled:
		return "已取消"
	}
	return "调用失败"
}

// describeLLMError 把模型调用错误包装为业务错误（API 返回给前端）。
func describeLLMError(err error, what string) error {
	return errs.Wrap(err, errs.CodeUnavailable, "%s：%s", what, describeError(err)).WithDetails(map[string]string{"error": err.Error()})
}

func clip(s string, n int) string {
	if r := []rune(strings.TrimSpace(s)); len(r) > n {
		return string(r[:n]) + "…"
	}
	return strings.TrimSpace(s)
}

// probeImageHash 是测试色块图的伪哈希（只在测试适配器的内存 BlobReader 中存在）。
const probeImageHash = "probe:red"

type memBlobReader map[string][]byte

func (m memBlobReader) ReadBlob(_ context.Context, hash string) ([]byte, error) {
	if b, ok := m[hash]; ok {
		return b, nil
	}
	return nil, errs.NotFound("图片不存在：%s", hash)
}

var probeBlobs = memBlobReader{probeImageHash: solidPNG(color.RGBA{220, 20, 20, 255})}

// solidPNG 生成 64×64 的纯色 PNG。
func solidPNG(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
