package runtime

import (
	"context"
	"errors"
	"io"
	"maps"
	"slices"
	"time"

	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/helper"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/usage"
)

func (m *Manager) delegate(ctx context.Context, d *draft, kind, task string) (string, error) {
	p, model, ok := m.lightModel(ctx)
	if !ok {
		return "", errs.Invalid("请先配置轻量模型")
	}
	if !d.features.Has(prompts.FHelpers) {
		return "", errs.Invalid("后台助手未开启")
	}
	m.publish(d.save.ID, event.HelperProgress, event.HelperProgressData{TurnID: d.id, Kind: kind, Phase: "start"})
	phase := "error"
	defer func() {
		m.publish(d.save.ID, event.HelperProgress, event.HelperProgressData{TurnID: d.id, Kind: kind, Phase: phase})
	}()
	allowed := map[string]bool{"read": true, "list": true, "search": true}
	if kind == "curate" {
		allowed["write"], allowed["edit"], allowed["remove"] = true, true, true
	}
	features := maps.Clone(d.features)
	features[prompts.FVision] = model.Caps.Vision
	env := &compactEnv{features: features, work: d.work, fs: d.vfs, state: d.state, images: d.images, model: model}
	specs := slices.DeleteFunc(m.d.Tools.Specs(features, d.work), func(s llm.ToolSpec) bool { return !allowed[s.Name] })
	system := prompts.HelperResearch
	if kind == "curate" {
		system = prompts.HelperCurate
	}
	system, err := contextx.RenderBackgroundPrompt(system, features, nil)
	if err != nil {
		return "", err
	}
	req := &llm.Request{CacheKey: "aint:helper:" + d.save.ID + ":" + d.id, System: []llm.SystemBlock{{Text: system + "\n" + contextx.RenderToolGuide(specs)}}, Tools: specs, Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(task)}}}, MaxOutput: min(4000, model.MaxOutput), Reasoning: llm.ReasoningLow, Purpose: "helper", CacheTTL: d.save.Settings.CacheTTL}
	var inspection *debugRecorder
	call := func(ctx context.Context, req *llm.Request) (llm.Message, llm.StopReason, error) {
		if d.helperCalls >= 10 {
			return llm.Message{}, "", errs.Invalid("本回合助手已达调用上限")
		}
		d.helperCalls++
		if contextx.EstimateRequest(req)+req.MaxOutput > model.ContextWindow {
			return llm.Message{}, "", errs.Invalid("助手上下文已达上限")
		}
		inspection = m.debugStart(ctx, d.save.ID, d.id, d.turnNo, p, model, req, d.vfs, d.state)
		start := time.Now()
		msg, stop, u, err := backgroundCall(ctx, p, model, req, inspection)
		rec := usage.Record{CallKind: "helper", ModelID: model.ID, Usage: u, LatencyMS: time.Since(start).Milliseconds(), Stop: string(stop)}
		if cost, ok := model.Price.CostMicros(u, req.CacheTTL); ok {
			rec.CostMicros, rec.Currency = &cost, model.Price.CurrencyOrDefault()
		}
		d.records = append(d.records, rec)
		d.usage = d.usage.Add(u)
		stats := usage.Total(d.records)
		m.publish(d.save.ID, event.TurnUsage, event.TurnUsageData{TurnID: d.id, Usage: d.usage, Costs: stats.Costs, Unpriced: stats.Unpriced, Context: event.ContextUsage{Used: d.contextTokens(m), Threshold: d.threshold}})
		return msg, stop, err
	}
	execute := func(ctx context.Context, calls []llm.ToolCall) llm.Message {
		valid := slices.DeleteFunc(slices.Clone(calls), func(c llm.ToolCall) bool { return !allowed[c.Name] })
		out := m.d.Tools.Execute(ctx, env, valid)
		result := llm.Message{Role: llm.RoleUser}
		for _, c := range calls {
			r := llm.ToolResult{CallID: c.ID, Parts: []llm.Part{llm.Text("未执行：助手没有这个工具的权限")}, IsError: true}
			for _, candidate := range out.Results {
				if candidate.CallID == c.ID {
					r = candidate
					break
				}
			}
			result.Parts = append(result.Parts, llm.Part{Kind: llm.PartToolResult, ToolResult: &r})
		}
		inspection.toolsDone(ctx, out, d.vfs, d.state)
		return result
	}
	result, err := helper.Run(ctx, req, call, execute)
	if err == nil {
		phase = "done"
	}
	return result, err
}

// backgroundCall 收集不进入正文的单次调用，缺少终态时不自动重试。
func backgroundCall(ctx context.Context, p llm.Provider, model *llm.Model, req *llm.Request, inspection *debugRecorder) (llm.Message, llm.StopReason, llm.Usage, error) {
	stream, err := debugStream(ctx, p, model, req, inspection)
	if err != nil {
		return llm.Message{}, "", llm.UsageOf(err), err
	}
	defer func() { _ = stream.Close() }()
	for {
		ev, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = errs.Invalid("模型未返回完整结果")
			}
			return llm.Message{}, "", llm.UsageOf(err), err
		}
		if done, ok := ev.(llm.Done); ok {
			return done.Message, done.Stop, done.Usage, nil
		}
	}
}
