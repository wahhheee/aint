package runtime

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/panel"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/work"
)

// Presentation 是阅读页打开存档时所需的全部"呈现信息"（GET /saves/{id}）。
// 它是一个读模型：把存档、作品版本、模型能力合成为前端直接可用的结构，前端不需要理解作品格式。
type Presentation struct {
	Save          *save.Save `json:"save"`
	RunningTurnID string     `json:"running_turn_id,omitzero"`
	NeedsOpening  bool       `json:"needs_opening"` // 还没有任何回合：前端应提交 kind=opening 的输入
	DevWork       bool       `json:"dev_work"`      // 作品来自开发目录：默认显示开发者工具（D-31）
	Compacting    bool       `json:"compacting"`    // 正在整理上下文
	// Context 是当前上下文规模与整理阈值（顶栏用量环）；Compactions 是当前进度线上的整理次数。
	Context     event.ContextUsage `json:"context"`
	Compactions int                `json:"compactions"`
	Model       ModelBrief         `json:"model"`
	UI          UI                 `json:"ui"`
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
	Assets      []AssetRef     `json:"assets"`     // 素材（舞台背景、插图、头像）
	Theme       work.Theme     `json:"theme"`      // 作品主题：CSS（作用域限于 .work-root）与排版数值
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

// AssetRef 是前端可直接使用的素材引用。作品中的素材引用写 ID 或路径（含 / 的视为路径），两者都能找到它。
type AssetRef struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Role        string `json:"role"`
	URL         string `json:"url"`
	Description string `json:"description,omitzero"`
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
	p := &Presentation{Save: sv, RunningTurnID: m.Running(saveID), NeedsOpening: lastTurn(line) == nil, DevWork: w.IsDev,
		Compacting: m.Compacting(saveID)}
	for _, n := range line {
		if n.Kind == save.KindCompaction {
			p.Compactions++
		}
	}

	features := prompts.FeatureSet{}
	if _, model, err := m.d.Models.Resolve(ctx, sv.Settings.NarratorModel); err == nil {
		p.Model = ModelBrief{ID: model.ID, Name: model.Name, DisplayName: model.DisplayName, Vision: model.Caps.Vision, Context: model.ContextWindow}
		features = contextx.Profile(w, sv.Settings, model.Caps, false)
		p.Context = event.ContextUsage{Used: contextTokensOf(line),
			Threshold: m.compactParams(ctx, sv.Settings).Threshold(model.ContextWindow, maxOutput(model))}
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
		Assets:      []AssetRef{},
		Theme:       w.Theme,
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
	for _, a := range w.Assets {
		ui.Assets = append(ui.Assets, AssetRef{ID: a.ID, Path: a.Path, Kind: a.Kind, Role: a.Role, URL: work.AssetURL(a), Description: a.Description})
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
	if w.IsDev {
		st, _ = state.Migrate(w.State, st)
	}
	return panel.Render(m.d.Tmpl, w, panel.Vars{State: st, Turn: head.TurnNo, WorkTitle: w.Manifest.Title, SaveTitle: sv.Title}), nil
}

// StateView 是存档当前的状态（GET /saves/{id}/state）：head 节点的状态快照与作品的状态定义。
type StateView struct {
	Migration []state.MigrationValue `json:"migration"`
	NodeID    string                 `json:"node_id"`
	TurnNo    int                    `json:"turn_no"`
	State     map[string]any         `json:"state"`
	Schema    *state.Schema          `json:"schema"`
}

// State 返回存档 head 处的状态。回合进行中的修改是草稿，提交后才在这里出现（草稿经 turn.state 事件推送）。
func (m *Manager) State(ctx context.Context, saveID string) (*StateView, error) {
	_, line, err := m.d.Saves.ProgressLine(ctx, saveID)
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
	records := []state.MigrationValue{}
	for _, n := range line {
		records = append(records, n.Meta.Migration...)
	}
	if w.IsDev {
		var pending []state.MigrationValue
		st, pending = state.Migrate(w.State, st)
		records = append(records, pending...)
	}
	return &StateView{NodeID: head.ID, TurnNo: head.TurnNo, State: st, Schema: w.State, Migration: records}, nil
}

// panelDebounce 是回合内状态变化后重新渲染面板的去抖间隔（《02-技术方案》17）：
// 一次工具调用批次里的多次修改只渲染一次。
const panelDebounce = 100 * time.Millisecond

// panelRefresher 在回合内状态变化后去抖重渲染面板，以本回合当前的状态（草稿）推送 panels 事件。
type panelRefresher struct {
	m *Manager
	d *draft

	mu      sync.Mutex
	timer   *time.Timer
	pending map[string]any // 待渲染的状态快照（state.Apply 产生的新映射，之后不会再被修改）
	stopped bool
}

func (m *Manager) panelRefresher(d *draft) *panelRefresher {
	return &panelRefresher{m: m, d: d}
}

// schedule 记下最新的状态，间隔内的多次调用合并为一次渲染。
func (r *panelRefresher) schedule(st map[string]any) {
	if len(r.d.work.Layout.Panels) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}
	r.pending = st
	if r.timer == nil {
		r.timer = time.AfterFunc(panelDebounce, r.fire)
	}
}

func (r *panelRefresher) fire() {
	r.mu.Lock()
	st := r.pending
	r.timer = nil
	stopped := r.stopped
	r.mu.Unlock()
	if stopped || st == nil {
		return
	}
	d := r.d
	panels := panel.Render(r.m.d.Tmpl, d.work, panel.Vars{State: st, Turn: d.turnNo, WorkTitle: d.work.Manifest.Title, SaveTitle: d.save.Title})
	r.m.publish(d.save.ID, event.Panels, event.PanelsData{Panels: panels})
}

// stop 取消尚未触发的渲染：回合结束后由提交后的 publishPanels 以已提交的状态推送。
func (r *panelRefresher) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
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
