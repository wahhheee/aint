package runtime

import (
	"context"
	"log/slog"

	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/panel"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/work"
)

// Presentation 是阅读页打开存档时所需的全部"呈现信息"（GET /saves/{id}）。
// 它是一个读模型：把存档、作品版本、模型能力合成为前端直接可用的结构，前端不需要理解作品格式。
type Presentation struct {
	Save          *save.Save `json:"save"`
	RunningTurnID string     `json:"running_turn_id,omitzero"`
	NeedsOpening  bool       `json:"needs_opening"` // 还没有任何回合：前端应提交 kind=opening 的输入
	Model         ModelBrief `json:"model"`
	UI            UI         `json:"ui"`
}

// ModelBrief 是主笔模型的摘要。
type ModelBrief struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Vision      bool   `json:"vision"`
	Context     int    `json:"context_window"`
}

// UI 是作品界面配置。
type UI struct {
	Features    UIFeatures     `json:"features"`
	BuiltinTags []string       `json:"builtin_tags"` // 已启用的内置标签（渲染白名单）
	Tags        []work.TagDef  `json:"tags"`         // 作品样式标签（前端据此生成 .mk-<tag> 样式）
	Panels      []PanelMeta    `json:"panels"`
	CommandBar  []string       `json:"command_bar"`
	Commands    []work.Command `json:"commands"`
	Characters  []Character    `json:"characters"` // <say who> 的说话人样式
	Pace        string         `json:"pace"`
}

// UIFeatures 是与界面交互相关的作品开关。
type UIFeatures struct {
	FreeInput    bool `json:"free_input"`
	OOC          bool `json:"ooc"`
	Dice         bool `json:"dice"`
	TimedChoices bool `json:"timed_choices"`
}

// PanelMeta 是面板的静态布局信息；内容（HTML）另经 /panels 与 SSE panels 事件下发。
type PanelMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitzero"`
	Place     string `json:"place"`
	Order     int    `json:"order"`
	Collapsed bool   `json:"collapsed,omitzero"`
}

// Character 是人物的显示属性（来自设定文档的 display 字段）。
type Character struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitzero"`
	Color   string   `json:"color,omitzero"`
	Avatar  string   `json:"avatar,omitzero"`
}

// Presentation 返回存档的呈现信息。
func (m *Manager) Presentation(ctx context.Context, saveID string) (*Presentation, error) {
	sv, line, err := m.d.Saves.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, err
	}
	head := line[len(line)-1]
	w, err := m.d.Works.Model(ctx, head.WorkVersionID)
	if err != nil {
		return nil, err
	}
	p := &Presentation{Save: sv, RunningTurnID: m.Running(saveID), NeedsOpening: head.Kind == save.KindRoot}

	features := prompts.FeatureSet{}
	if _, model, err := m.d.Models.Resolve(ctx, sv.Settings.NarratorModel); err == nil {
		p.Model = ModelBrief{ID: model.ID, Name: model.Name, DisplayName: model.DisplayName, Vision: model.Caps.Vision, Context: model.ContextWindow}
		features = contextx.Profile(w, sv.Settings, model.Caps, false)
	} else {
		// 模型不可用时仍允许浏览存档；提交回合时会给出明确错误。
		slog.WarnContext(ctx, "存档的主笔模型不可用", "save_id", saveID, "err", err)
	}

	mf := w.Manifest
	ui := UI{
		Features:    UIFeatures{FreeInput: mf.Features.FreeInput, OOC: mf.Features.OOC, Dice: mf.Features.Dice, TimedChoices: mf.Features.TimedChoices},
		BuiltinTags: nonNil(contextx.EnabledBuiltinTags(features)),
		Tags:        nonNil(w.Tags),
		Panels:      []PanelMeta{},
		CommandBar:  nonNil(w.Layout.CommandBar),
		Commands:    nonNil(w.Commands),
		Characters:  []Character{},
		Pace:        firstNonEmpty(sv.Settings.Pace, mf.Narrator.Length),
	}
	for _, pd := range w.Layout.Panels {
		ui.Panels = append(ui.Panels, PanelMeta{ID: pd.ID, Title: pd.Title, Place: pd.Place, Order: pd.Order, Collapsed: pd.Collapsed})
	}
	for _, d := range w.Docs {
		if d.Kind == "character" || d.Display.Color != "" || d.Display.Avatar != "" {
			ui.Characters = append(ui.Characters, Character{ID: d.ID, Name: d.Title, Aliases: d.Aliases, Color: d.Display.Color, Avatar: d.Display.Avatar})
		}
	}
	p.UI = ui
	return p, nil
}

// Panels 渲染存档当前 head 的面板。
func (m *Manager) Panels(ctx context.Context, saveID string) ([]event.PanelRender, error) {
	sv, line, err := m.d.Saves.ProgressLine(ctx, saveID)
	if err != nil {
		return nil, err
	}
	head := line[len(line)-1]
	w, err := m.d.Works.Model(ctx, head.WorkVersionID)
	if err != nil {
		return nil, err
	}
	st, err := m.d.Saves.LoadState(ctx, head.StateHash)
	if err != nil {
		return nil, err
	}
	return panel.Render(m.d.Tmpl, w, panel.Vars{State: st, Turn: head.TurnNo, WorkTitle: w.Manifest.Title, SaveTitle: sv.Title}), nil
}

func (m *Manager) publishPanels(ctx context.Context, saveID string) {
	panels, err := m.Panels(ctx, saveID)
	if err != nil {
		slog.WarnContext(ctx, "面板渲染失败", "err", err)
		return
	}
	m.publish(saveID, event.Panels, event.PanelsData{Panels: panels})
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
