package runtime

import (
	"context"
	"log/slog"
	"math"
	"sync"

	"github.com/wahhheee/aint/internal/compact"
	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/settings"
	"github.com/wahhheee/aint/internal/state"
	"github.com/wahhheee/aint/internal/work"
)

// SettingsSource 提供全局设置（整理参数、轻量模型）；由 settings.Service 实现。为空时使用默认值。
type SettingsSource interface {
	Get(ctx context.Context) (settings.Settings, error)
}

// compactParams 返回存档生效的整理参数：存档设置的覆盖 > 全局设置 > 默认值。
func (m *Manager) compactParams(ctx context.Context, st save.Settings) compact.Params {
	p := compact.DefaultParams
	if m.d.Settings != nil {
		if g, err := m.d.Settings.Get(ctx); err == nil {
			p = p.With(g.Context.Budget, g.Context.KeepRecent, g.Context.Mode)
		}
	}
	return p.With(st.ContextBudget, st.KeepRecent, st.CompactMode)
}

// lightModel 返回设置中的轻量模型；没有配置或解析失败时 ok 为 false。
func (m *Manager) lightModel(ctx context.Context) (llm.Provider, *llm.Model, bool) {
	if m.d.Settings == nil {
		return nil, nil, false
	}
	g, err := m.d.Settings.Get(ctx)
	if err != nil || g.Models.Light == "" {
		return nil, nil, false
	}
	p, model, err := m.d.Models.Resolve(ctx, g.Models.Light)
	if err != nil {
		slog.WarnContext(ctx, "轻量模型不可用", "model", g.Models.Light, "err", err)
		return nil, nil, false
	}
	return p, model, true
}

// token 估算的校准（《02-技术方案》8.5）：按模型记录"实际输入 ÷ 估算"的指数滑动平均，估算时乘上它。
// 只在内存中，程序重启后从 1 开始重新校准；估算只用于触发整理与界面显示，计费以 API 返回为准。
type calibration struct {
	mu     sync.Mutex
	factor map[string]float64
}

// calibrationAlpha 是滑动平均的权重：新的一次观测占 30%。
const calibrationAlpha = 0.3

func (m *Manager) calibrate(modelID string, raw, actual int) {
	if raw <= 0 || actual <= 0 {
		return
	}
	ratio := float64(actual) / float64(raw)
	if ratio < 0.2 || ratio > 5 { // 用量口径异常（如中转站不报缓存部分）时不参与校准
		return
	}
	m.cal.mu.Lock()
	defer m.cal.mu.Unlock()
	if m.cal.factor == nil {
		m.cal.factor = map[string]float64{}
	}
	if f, ok := m.cal.factor[modelID]; ok {
		m.cal.factor[modelID] = f*(1-calibrationAlpha) + ratio*calibrationAlpha
	} else {
		m.cal.factor[modelID] = ratio
	}
}

// estimate 估算一次请求的输入 token（按模型校准）。
func (m *Manager) estimate(model *llm.Model, req *llm.Request) int {
	m.cal.mu.Lock()
	f, ok := m.cal.factor[model.ID]
	m.cal.mu.Unlock()
	if !ok {
		f = 1
	}
	return int(math.Ceil(float64(contextx.EstimateRequest(req)) * f))
}

// estimateHistory 估算以 epoch 与 history 开头的请求（本回合消息为空）的输入 token。
func (m *Manager) estimateHistory(ctx context.Context, sv *save.Save, w *work.Model, fs prompts.FeatureSet, model *llm.Model,
	root *save.Node, epoch string, history []llm.Message) (int, error) {
	initState, err := m.d.Saves.LoadState(ctx, root.StateHash)
	if err != nil {
		return 0, err
	}
	initState, _ = state.Migrate(w.State, initState)
	built, err := m.d.Builder.Build(ctx, contextx.BuildInput{
		SaveID: sv.ID, Work: w, Features: fs, Model: model, Settings: sv.Settings,
		InitialState: initState, InitialStateHash: root.StateHash, Epoch: epoch,
		Tools: m.d.Tools.Specs(fs, w), History: history, Turn: llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("")}},
	})
	if err != nil {
		return 0, err
	}
	return m.estimate(model, built.Request), nil
}

// contextTokensOf 返回进度线末尾的上下文规模：最近一个记录了 ContextTokens 的节点（编辑节点沿用之前的规模）。
func contextTokensOf(line []*save.Node) int {
	for i := len(line) - 1; i >= 0; i-- {
		if n := line[i]; n.Meta.ContextTokens > 0 {
			return n.Meta.ContextTokens
		}
		if line[i].Kind == save.KindCompaction {
			return 0
		}
	}
	return 0
}

func jsonxMarshal(v any) []byte { return jsonx.MustMarshal(v) }
