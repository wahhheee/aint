// Package provider 管理模型供应商与模型配置（providers / models 表、secrets.json），
// 并负责把配置组装成可调用的 llm.Provider：适配器 + 重试 + 调试追踪。
//
// 组装顺序（外层先执行）：WithTrace(WithRetry(adapter))
//   - trace 在最外层：记录的是运行器真正看到的事件序列（含重试后的结果）；
//   - --fake-llm 模式下所有模型都解析为假模型，便于不配置任何 Key 就能跑通全流程。
package provider

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/anthropic"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/llm/gemini"
	"github.com/wahhheee/yanyi/internal/llm/openai"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/pkg/logx"
	"github.com/wahhheee/yanyi/internal/store"
)

// Options 是供应商服务的选项。
type Options struct {
	Fake     *fake.Script // 非空表示 --fake-llm 模式
	TraceDir string       // 非空表示 --trace-llm：调用记录写到该目录
}

// Service 是供应商服务。并发安全。
type Service struct {
	db      *store.DB
	secrets *Secrets
	opts    Options
	fake    llm.Provider

	mu    sync.Mutex
	built map[string]llm.Provider // provider ID → 组装好的 Provider
}

// NewService 创建供应商服务。
func NewService(db *store.DB, secrets *Secrets, opts Options) *Service {
	s := &Service{db: db, secrets: secrets, opts: opts, built: map[string]llm.Provider{}}
	if opts.Fake != nil {
		s.fake = s.decorate(fake.New(opts.Fake))
	}
	return s
}

// FakeMode 表示是否处于 --fake-llm 模式。
func (s *Service) FakeMode() bool { return s.fake != nil }

// Resolve 根据模型配置 ID 返回可调用的 Provider 与模型（runtime.ModelResolver）。
func (s *Service) Resolve(ctx context.Context, modelID string) (llm.Provider, *llm.Model, error) {
	if s.fake != nil {
		return s.fake, fake.Model(), nil
	}
	if modelID == "" {
		return nil, nil, errs.New(errs.CodeModelMissing, "开始之前，需要连接一个模型")
	}
	m, providerID, err := s.loadModel(ctx, modelID)
	if err != nil {
		return nil, nil, err
	}
	p, err := s.providerFor(ctx, providerID)
	if err != nil {
		return nil, nil, err
	}
	return p, m, nil
}

func (s *Service) providerFor(ctx context.Context, id string) (llm.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.built[id]; ok {
		return p, nil
	}
	cfg, err := s.loadProvider(ctx, id)
	if err != nil {
		return nil, err
	}
	var adapter llm.Provider
	switch cfg.Protocol {
	case llm.ProtocolOpenAIResponses:
		adapter = openai.New(cfg)
	case llm.ProtocolAnthropic:
		adapter = anthropic.New(cfg)
	case llm.ProtocolGemini:
		adapter = gemini.New(cfg)
	default:
		return nil, errs.Invalid("不支持的协议：%s", cfg.Protocol)
	}
	p := s.decorate(adapter)
	s.built[id] = p
	return p, nil
}

func (s *Service) decorate(p llm.Provider) llm.Provider {
	p = llm.WithRetry(p, llm.DefaultRetry)
	if s.opts.TraceDir != "" {
		p = llm.WithTrace(p, s.opts.TraceDir)
	}
	return p
}

// Invalidate 在供应商配置变更后丢弃缓存的 Provider。
func (s *Service) Invalidate(providerID string) {
	s.mu.Lock()
	delete(s.built, providerID)
	s.mu.Unlock()
}

// ---------------------------------------------------------------------------
// 查询（API 输出）

// ProviderView 是前端看到的供应商（密钥只给掩码）。
type ProviderView struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Protocol  llm.Protocol      `json:"protocol"`
	BaseURL   string            `json:"base_url"`
	Headers   map[string]string `json:"headers,omitzero"`
	KeyMasked string            `json:"key_masked"`
	CreatedAt int64             `json:"created_at"`
}

// List 返回全部供应商。
func (s *Service) List(ctx context.Context) ([]ProviderView, error) {
	rows, err := s.db.Read().QueryContext(ctx,
		`SELECT id, name, protocol, base_url, COALESCE(headers, '{}'), secret_ref, created_at FROM providers ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderView{}
	for rows.Next() {
		var v ProviderView
		var headers, ref string
		if err := rows.Scan(&v.ID, &v.Name, &v.Protocol, &v.BaseURL, &headers, &ref, &v.CreatedAt); err != nil {
			return nil, err
		}
		_ = jsonx.Unmarshal([]byte(headers), &v.Headers)
		if key, err := s.secrets.Resolve(ref); err == nil {
			v.KeyMasked = logx.Mask(key)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Models 返回全部已配置的模型；--fake-llm 模式下只返回假模型。
func (s *Service) Models(ctx context.Context) ([]llm.Model, error) {
	if s.fake != nil {
		return []llm.Model{*fake.Model()}, nil
	}
	rows, err := s.db.Read().QueryContext(ctx, `SELECT id FROM models ORDER BY display_name, model`)
	if err != nil {
		return nil, err
	}
	var idList []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		idList = append(idList, id)
	}
	_ = rows.Close()
	out := []llm.Model{}
	for _, id := range idList {
		m, _, err := s.loadModel(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, nil
}

// TODO(M1-11/M4-7)：供应商与模型的增删改（Create/Update/Delete，密钥写入 secrets.json 后 Invalidate）、
// 连接测试（模型列表 → 文本 → 强制工具调用往返 → 可选看图，逐项结果）、拉取远端模型列表、内置模型预设。

func (s *Service) loadProvider(ctx context.Context, id string) (llm.ProviderConfig, error) {
	var cfg llm.ProviderConfig
	var headers, options sql.NullString
	var ref string
	err := s.db.Read().QueryRowContext(ctx,
		`SELECT id, name, protocol, base_url, headers, options, secret_ref FROM providers WHERE id = ?`, id).
		Scan(&cfg.ID, &cfg.Name, &cfg.Protocol, &cfg.BaseURL, &headers, &options, &ref)
	if err == sql.ErrNoRows {
		return cfg, errs.NotFound("供应商不存在：%s", id)
	}
	if err != nil {
		return cfg, err
	}
	if headers.Valid {
		_ = jsonx.Unmarshal([]byte(headers.String), &cfg.Headers)
	}
	if options.Valid {
		cfg.Options = jsonx.Value(options.String)
	}
	if cfg.APIKey, err = s.secrets.Resolve(ref); err != nil {
		return cfg, errs.Wrap(err, errs.CodeModelMissing, "供应商「%s」的 API Key 不可用", cfg.Name)
	}
	return cfg, nil
}

func (s *Service) loadModel(ctx context.Context, id string) (*llm.Model, string, error) {
	m := &llm.Model{}
	var caps string
	var price, display sql.NullString
	err := s.db.Read().QueryRowContext(ctx,
		`SELECT id, provider_id, model, display_name, context_window, max_output, caps, price FROM models WHERE id = ?`, id).
		Scan(&m.ID, &m.ProviderID, &m.Name, &display, &m.ContextWindow, &m.MaxOutput, &caps, &price)
	if err == sql.ErrNoRows {
		return nil, "", errs.New(errs.CodeModelMissing, "存档使用的模型已不存在，请在设置中重新选择")
	}
	if err != nil {
		return nil, "", err
	}
	m.DisplayName = display.String
	if err := jsonx.Unmarshal([]byte(caps), &m.Caps); err != nil {
		return nil, "", fmt.Errorf("模型 %s 的能力配置损坏: %w", id, err)
	}
	if price.Valid {
		m.Price = &llm.Price{}
		_ = jsonx.Unmarshal([]byte(price.String), m.Price)
	}
	return m, m.ProviderID, nil
}
