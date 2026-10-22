package provider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/ids"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/store"
)

// ModelInput 是添加或修改模型的输入。添加时缺省的字段按内置预设填充（没有预设时用保守的默认值）；
// 修改时为空的字段保持不变，Caps / Price 整体替换。
type ModelInput struct {
	ProviderID    string     `json:"provider_id,omitzero"`
	Name          string     `json:"name,omitzero"`
	DisplayName   *string    `json:"display_name,omitzero"`
	ContextWindow int        `json:"context_window,omitzero"`
	MaxOutput     int        `json:"max_output,omitzero"`
	Caps          *llm.Caps  `json:"caps,omitzero"`
	Price         *llm.Price `json:"price,omitzero"`
	// ClearPrice 为 true 时清除价格（费用显示"无法估算"）。
	ClearPrice bool `json:"clear_price,omitzero"`
}

// fallbackCaps 是没有预设时的能力：只假定支持工具，其余由用户在模型设置中打开。
var fallbackCaps = llm.Caps{Tools: true, Reasoning: "none", Cache: "implicit", ImageMaxEdge: 1568}

const modelColumns = `id, provider_id, model, display_name, context_window, max_output, caps, price`

func scanModel(row interface{ Scan(...any) error }) (*llm.Model, error) {
	m := &llm.Model{}
	var caps string
	var price, display sql.NullString
	if err := row.Scan(&m.ID, &m.ProviderID, &m.Name, &display, &m.ContextWindow, &m.MaxOutput, &caps, &price); err != nil {
		return nil, err
	}
	m.DisplayName = display.String
	if err := jsonx.Unmarshal([]byte(caps), &m.Caps); err != nil {
		return nil, fmt.Errorf("模型 %s 的能力配置损坏: %w", m.ID, err)
	}
	if price.Valid && price.String != "" && price.String != "null" {
		m.Price = &llm.Price{}
		if err := jsonx.Unmarshal([]byte(price.String), m.Price); err != nil {
			return nil, fmt.Errorf("模型 %s 的价格配置损坏: %w", m.ID, err)
		}
	}
	return m, nil
}

// Models 返回全部已配置的模型（价格为模型单价，不含供应商倍率）；--fake-llm 模式下只返回假模型。
func (s *Service) Models(ctx context.Context) ([]llm.Model, error) {
	if s.fake != nil {
		return []llm.Model{*fake.Model()}, nil
	}
	rows, err := s.db.Read().QueryContext(ctx, `SELECT `+modelColumns+` FROM models ORDER BY provider_id, COALESCE(display_name, model)`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []llm.Model{}
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (s *Service) loadModel(ctx context.Context, q store.Querier, id string) (*llm.Model, error) {
	m, err := scanModel(q.QueryRowContext(ctx, `SELECT `+modelColumns+` FROM models WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.New(errs.CodeModelMissing, "存档使用的模型已不存在，请在设置中重新选择")
	}
	return m, err
}

// CreateModel 添加模型：按名字匹配内置预设预填窗口、输出上限、能力与官方价格。
func (s *Service) CreateModel(ctx context.Context, in ModelInput) (*llm.Model, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, errs.Invalid("请填写模型 ID")
	}
	prov, err := s.Get(ctx, in.ProviderID)
	if err != nil {
		return nil, err
	}
	var exists int
	if err := s.db.Read().QueryRowContext(ctx, `SELECT COUNT(*) FROM models WHERE provider_id = ? AND model = ?`, in.ProviderID, in.Name).Scan(&exists); err != nil {
		return nil, err
	}
	if exists > 0 {
		return nil, errs.Conflict("模型 %s 已经添加过了", in.Name)
	}

	m := llm.Model{ID: ids.New(), ProviderID: in.ProviderID, Name: in.Name, ContextWindow: 128_000, MaxOutput: 8192, Caps: fallbackCaps}
	if pre, ok := PresetFor(prov.Protocol, in.Name); ok {
		m.DisplayName, m.ContextWindow, m.MaxOutput, m.Caps, m.Price = pre.DisplayName, pre.ContextWindow, pre.MaxOutput, pre.Caps, pre.Price
	}
	applyModelInput(&m, in)
	if err := validateModel(m); err != nil {
		return nil, err
	}
	if err := s.writeModel(ctx, m, true); err != nil {
		return nil, err
	}
	return s.loadModel(ctx, s.db.Read(), m.ID)
}

// UpdateModel 修改模型。
func (s *Service) UpdateModel(ctx context.Context, id string, in ModelInput) (*llm.Model, error) {
	m, err := s.loadModel(ctx, s.db.Read(), id)
	if err != nil {
		return nil, err
	}
	if in.ProviderID != "" && in.ProviderID != m.ProviderID {
		return nil, errs.Invalid("模型不能移到其他供应商")
	}
	if in.Name != "" {
		m.Name = strings.TrimSpace(in.Name)
	}
	applyModelInput(m, in)
	if err := validateModel(*m); err != nil {
		return nil, err
	}
	if err := s.writeModel(ctx, *m, false); err != nil {
		return nil, err
	}
	return s.loadModel(ctx, s.db.Read(), id)
}

// DeleteModel 删除模型。使用它的存档在下次游玩时会提示重新选择模型。
func (s *Service) DeleteModel(ctx context.Context, id string) error {
	if _, err := s.loadModel(ctx, s.db.Read(), id); err != nil {
		return err
	}
	return s.exec(ctx, `DELETE FROM models WHERE id = ?`, id)
}

func applyModelInput(m *llm.Model, in ModelInput) {
	if in.DisplayName != nil {
		m.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	if in.ContextWindow > 0 {
		m.ContextWindow = in.ContextWindow
	}
	if in.MaxOutput > 0 {
		m.MaxOutput = in.MaxOutput
	}
	if in.Caps != nil {
		m.Caps = *in.Caps
	}
	if in.Price != nil {
		m.Price = in.Price
	}
	if in.ClearPrice {
		m.Price = nil
	}
}

func validateModel(m llm.Model) error {
	switch {
	case m.ContextWindow < 1024:
		return errs.Invalid("上下文窗口太小")
	case m.MaxOutput < 256:
		return errs.Invalid("单次输出上限太小")
	}
	switch m.Caps.Reasoning {
	case "effort", "budget", "level", "none":
	default:
		return errs.Invalid("推理控制方式只能是 effort / budget / level / none")
	}
	switch m.Caps.Cache {
	case "breakpoints", "implicit", "none":
	default:
		return errs.Invalid("缓存方式只能是 breakpoints / implicit / none")
	}
	if p := m.Price; p != nil {
		for _, v := range []*float64{p.Input, p.CacheRead, p.CacheWrite, p.CacheWriteLong, p.Output} {
			if v != nil && *v < 0 {
				return errs.Invalid("价格不能为负数")
			}
		}
	}
	return nil
}

func (s *Service) writeModel(ctx context.Context, m llm.Model, insert bool) error {
	caps, err := jsonx.Marshal(m.Caps)
	if err != nil {
		return err
	}
	var price any
	if m.Price != nil {
		b, err := jsonx.Marshal(m.Price)
		if err != nil {
			return err
		}
		price = string(b)
	}
	if insert {
		return s.exec(ctx, `INSERT INTO models (id, provider_id, model, display_name, context_window, max_output, caps, price)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, m.ID, m.ProviderID, m.Name, store.NullString(m.DisplayName), m.ContextWindow, m.MaxOutput, string(caps), price)
	}
	return s.exec(ctx, `UPDATE models SET model = ?, display_name = ?, context_window = ?, max_output = ?, caps = ?, price = ? WHERE id = ?`,
		m.Name, store.NullString(m.DisplayName), m.ContextWindow, m.MaxOutput, string(caps), price, m.ID)
}
