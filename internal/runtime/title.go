package runtime

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/markup"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/usage"
)

// GenerateTitle 启动至多一个后台标题任务；自动请求尊重关闭、手动命名与进度去重规则。
func (m *Manager) GenerateTitle(ctx context.Context, id string, automatic bool) error {
	if automatic {
		if m.d.Settings == nil {
			return nil
		}
		g, err := m.d.Settings.Get(ctx)
		if err != nil {
			return err
		}
		if g.Play.TitleOnExit != "auto" {
			return nil
		}
	}
	sv, err := m.d.Saves.Get(ctx, id)
	if err != nil {
		return err
	}
	if automatic && sv.TitleSource == "manual" {
		return nil
	}
	p, model, ok := m.lightModel(ctx)
	if !ok {
		if automatic {
			return nil
		}
		return errs.Invalid("请先配置轻量模型")
	}
	m.titleMu.Lock()
	if m.titles == nil {
		m.titles = map[string]bool{}
	}
	if m.titles[id] {
		m.titleMu.Unlock()
		return nil
	}
	if automatic {
		current, err := m.d.Saves.TitleCurrent(ctx, id, sv.HeadNodeID)
		if err != nil || current {
			m.titleMu.Unlock()
			return err
		}
	}
	m.titles[id] = true
	m.titleMu.Unlock()
	m.wg.Go(func() {
		defer func() { m.titleMu.Lock(); delete(m.titles, id); m.titleMu.Unlock() }()
		completed := false
		defer func() {
			if !completed {
				m.d.Bus.Publish(event.GlobalTopic, event.SaveTitleFailed, event.SaveTitleFailedData{SaveID: id})
			}
		}()
		ctx, cancel := context.WithTimeout(m.base, 45*time.Second)
		defer cancel()
		tl, err := m.d.Saves.Timeline(ctx, id, "", 3)
		if err != nil {
			slog.WarnContext(ctx, "读取标题素材失败", "err", err)
			return
		}
		var text strings.Builder
		for _, n := range tl.Nodes {
			for _, it := range n.Display {
				if it.T == "text" {
					text.WriteString(markup.PlainText(it.Markup))
					text.WriteString("\n")
				}
			}
		}
		recent := []rune(text.String())
		if len(recent) == 0 {
			completed = true
			return
		}
		if len(recent) > 3000 {
			recent = recent[len(recent)-3000:]
		}
		system, err := contextx.RenderBackgroundPrompt(prompts.SaveTitle, nil, map[string]string{"WorkTitle": sv.WorkTitle, "RecentTurns": string(recent)})
		if err != nil {
			slog.WarnContext(ctx, "生成标题提示词失败", "err", err)
			return
		}
		req := &llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(system)}}}, MaxOutput: min(512, model.MaxOutput), Reasoning: llm.ReasoningOff, Purpose: "title"}
		start := time.Now()
		msg, stop, u, callErr := backgroundCall(ctx, p, model, req, nil)
		title := ""
		if callErr == nil && stop != llm.StopMaxTokens {
			for _, part := range msg.Parts {
				if part.Kind == llm.PartText {
					title += part.Text
				}
			}
			title = strings.Trim(strings.TrimSpace(title), "\"“”《》")
			if runes := []rune(title); len(runes) > 16 {
				title = string(runes[:16])
			}
			title = strings.Split(title, "\n")[0]
		}
		rec := usage.Record{SaveID: id, NodeID: sv.HeadNodeID, CallKind: "title", ModelID: model.ID, Usage: u, LatencyMS: time.Since(start).Milliseconds(), Stop: "error"}
		if title != "" {
			rec.Stop = "title"
		}
		if cost, ok := model.Price.CostMicros(u, ""); ok {
			rec.CostMicros, rec.Currency = &cost, model.Price.CurrencyOrDefault()
		}
		changed, err := m.d.Saves.CommitTitle(context.WithoutCancel(ctx), sv, title, rec)
		if err != nil || callErr != nil {
			slog.WarnContext(ctx, "生成标题失败", "err", err, "model_error", callErr)
		}
		completed = err == nil && callErr == nil && title != ""
		if changed {
			m.d.Bus.Publish(event.GlobalTopic, event.SaveTitled, event.SaveTitledData{SaveID: id, Title: title})
		}
	})
	return nil
}
