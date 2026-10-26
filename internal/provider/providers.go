package provider

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/pkg/logx"
	"github.com/wahhheee/aint/internal/store"
)

// ProviderView 是前端看到的供应商（密钥只给掩码）。
type ProviderView struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Protocol        llm.Protocol      `json:"protocol"`
	BaseURL         string            `json:"base_url"`
	Headers         map[string]string `json:"headers,omitzero"`
	KeyMasked       string            `json:"key_masked"`
	PriceMultiplier float64           `json:"price_multiplier"`
	// TestOK 是最近一次连接测试的结果；为空表示还没测试过。
	TestOK    *bool  `json:"test_ok,omitzero"`
	TestedAt  *int64 `json:"tested_at,omitzero"`
	CreatedAt int64  `json:"created_at"`
}

// ProviderInput 是添加或修改供应商的输入。修改时为空的字段保持不变（APIKey 为空表示不改密钥）。
type ProviderInput struct {
	Name            string            `json:"name,omitzero"`
	Protocol        llm.Protocol      `json:"protocol,omitzero"`
	BaseURL         string            `json:"base_url,omitzero"`
	APIKey          string            `json:"api_key,omitzero"`
	Headers         map[string]string `json:"headers,omitzero"`
	PriceMultiplier float64           `json:"price_multiplier,omitzero"`
}

// authHeaders 是不允许写在额外请求头里的头（密钥只放 API Key，存 secrets.json）。
var authHeaders = map[string]bool{"authorization": true, "x-api-key": true, "x-goog-api-key": true, "api-key": true}

func secretKey(providerID string) string { return "provider:" + providerID }

const providerColumns = `id, name, protocol, base_url, COALESCE(headers, '{}'), secret_ref, price_multiplier, test_ok, tested_at, created_at`

func (s *Service) scanProvider(row interface{ Scan(...any) error }) (ProviderView, string, error) {
	var v ProviderView
	var headers, ref string
	var ok sql.NullBool
	var at sql.NullInt64
	if err := row.Scan(&v.ID, &v.Name, &v.Protocol, &v.BaseURL, &headers, &ref, &v.PriceMultiplier, &ok, &at, &v.CreatedAt); err != nil {
		return v, "", err
	}
	_ = jsonx.Unmarshal([]byte(headers), &v.Headers)
	if len(v.Headers) == 0 {
		v.Headers = nil
	}
	if key, err := s.secrets.Resolve(ref); err == nil {
		v.KeyMasked = logx.Mask(key)
	}
	if ok.Valid {
		v.TestOK, v.TestedAt = &ok.Bool, &at.Int64
	}
	return v, ref, nil
}

// List 返回全部供应商。
func (s *Service) List(ctx context.Context) ([]ProviderView, error) {
	rows, err := s.db.Read().QueryContext(ctx, `SELECT `+providerColumns+` FROM providers ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ProviderView{}
	for rows.Next() {
		v, _, err := s.scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Get 返回一个供应商。
func (s *Service) Get(ctx context.Context, id string) (ProviderView, error) {
	v, _, err := s.scanProvider(s.db.Read().QueryRowContext(ctx, `SELECT `+providerColumns+` FROM providers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return v, errs.NotFound("供应商不存在：%s", id)
	}
	return v, err
}

// Create 添加供应商。密钥写入 secrets.json，数据库只存引用。
func (s *Service) Create(ctx context.Context, in ProviderInput) (ProviderView, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.APIKey = strings.TrimSpace(in.APIKey)
	if in.Name == "" {
		return ProviderView{}, errs.Invalid("请填写名称")
	}
	if in.APIKey == "" {
		return ProviderView{}, errs.Invalid("请填写 API Key")
	}
	if DefaultBaseURL(in.Protocol) == "" {
		return ProviderView{}, errs.Invalid("不支持的协议：%s", in.Protocol)
	}
	if in.BaseURL == "" {
		in.BaseURL = DefaultBaseURL(in.Protocol)
	}
	if in.PriceMultiplier == 0 {
		in.PriceMultiplier = 1
	}
	if err := validateProvider(in); err != nil {
		return ProviderView{}, err
	}
	id := ids.New()
	ref := secretKey(id)
	if err := s.secrets.Set(ref, in.APIKey); err != nil {
		return ProviderView{}, err
	}
	headers, err := jsonx.Marshal(in.Headers)
	if err != nil {
		return ProviderView{}, err
	}
	err = s.exec(ctx, `INSERT INTO providers (id, name, protocol, base_url, headers, secret_ref, price_multiplier, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, in.Name, in.Protocol, strings.TrimRight(in.BaseURL, "/"), string(headers), ref, in.PriceMultiplier, store.NowMillis())
	if err != nil {
		_ = s.secrets.Delete(ref)
		return ProviderView{}, err
	}
	return s.Get(ctx, id)
}

// Update 修改供应商（协议不可修改）。修改后清除连接测试结果并丢弃缓存的连接。
func (s *Service) Update(ctx context.Context, id string, in ProviderInput) (ProviderView, error) {
	cur, ref, err := s.scanProvider(s.db.Read().QueryRowContext(ctx, `SELECT `+providerColumns+` FROM providers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return cur, errs.NotFound("供应商不存在：%s", id)
	}
	if err != nil {
		return cur, err
	}
	if in.Protocol != "" && in.Protocol != cur.Protocol {
		return cur, errs.Invalid("协议不能修改；请另外添加一个供应商")
	}
	next := ProviderInput{Name: cur.Name, Protocol: cur.Protocol, BaseURL: cur.BaseURL, Headers: cur.Headers, PriceMultiplier: cur.PriceMultiplier}
	if in.Name != "" {
		next.Name = strings.TrimSpace(in.Name)
	}
	if in.BaseURL != "" {
		next.BaseURL = in.BaseURL
	}
	if in.Headers != nil {
		next.Headers = in.Headers
	}
	if in.PriceMultiplier != 0 {
		next.PriceMultiplier = in.PriceMultiplier
	}
	if err := validateProvider(next); err != nil {
		return cur, err
	}
	if key := strings.TrimSpace(in.APIKey); key != "" {
		if strings.HasPrefix(ref, "env:") { // 手动输入的密钥存入 secrets.json，使用供应商专属引用
			ref = secretKey(id)
		}
		if err := s.secrets.Set(ref, key); err != nil {
			return cur, err
		}
	}
	headers, err := jsonx.Marshal(next.Headers)
	if err != nil {
		return cur, err
	}
	err = s.exec(ctx, `UPDATE providers SET name = ?, base_url = ?, headers = ?, secret_ref = ?, price_multiplier = ?,
		test_ok = NULL, tested_at = NULL WHERE id = ?`, next.Name, strings.TrimRight(next.BaseURL, "/"), string(headers), ref, next.PriceMultiplier, id)
	if err != nil {
		return cur, err
	}
	s.invalidate(id)
	return s.Get(ctx, id)
}

// Delete 删除供应商及其模型（models 外键级联），并删除密钥。
func (s *Service) Delete(ctx context.Context, id string) error {
	_, ref, err := s.scanProvider(s.db.Read().QueryRowContext(ctx, `SELECT `+providerColumns+` FROM providers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return errs.NotFound("供应商不存在：%s", id)
	}
	if err != nil {
		return err
	}
	if err := s.exec(ctx, `DELETE FROM providers WHERE id = ?`, id); err != nil {
		return err
	}
	s.invalidate(id)
	if !strings.HasPrefix(ref, "env:") {
		return s.secrets.Delete(ref)
	}
	return nil
}

// exec 在写事务中执行一条语句。
func (s *Service) exec(ctx context.Context, query string, args ...any) error {
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
}

func validateProvider(in ProviderInput) error {
	if r := []rune(in.Name); len(r) > 64 {
		return errs.Invalid("名称太长（最多 64 个字）")
	}
	u, err := url.Parse(in.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errs.Invalid("Base URL 应以 http:// 或 https:// 开头")
	}
	for k := range in.Headers {
		if authHeaders[strings.ToLower(k)] {
			return errs.Invalid("请把密钥填在 API Key 中，不要放在额外请求头 %s 里", k)
		}
	}
	if in.PriceMultiplier < 0 || in.PriceMultiplier > 1000 {
		return errs.Invalid("价格倍率应大于 0")
	}
	return nil
}

// loadProvider 读取适配器配置（含解析后的密钥）与价格倍率。
func (s *Service) loadProvider(ctx context.Context, id string) (llm.ProviderConfig, float64, error) {
	v, ref, err := s.scanProvider(s.db.Read().QueryRowContext(ctx, `SELECT `+providerColumns+` FROM providers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return llm.ProviderConfig{}, 0, errs.NotFound("供应商不存在：%s", id)
	}
	if err != nil {
		return llm.ProviderConfig{}, 0, err
	}
	cfg := llm.ProviderConfig{ID: v.ID, Name: v.Name, Protocol: v.Protocol, BaseURL: v.BaseURL, Headers: v.Headers}
	if cfg.APIKey, err = s.secrets.Resolve(ref); err != nil {
		return cfg, 0, errs.Wrap(err, errs.CodeModelMissing, "供应商「%s」的 API Key 不可用", v.Name)
	}
	return s.adapterConfig(cfg), v.PriceMultiplier, nil
}

// RemoteModels 拉取供应商侧的模型列表（添加模型时勾选）。
func (s *Service) RemoteModels(ctx context.Context, id string) ([]llm.RemoteModel, error) {
	cfg, _, err := s.loadProvider(ctx, id)
	if err != nil {
		return nil, err
	}
	p, err := s.providerFor(cfg)
	if err != nil {
		return nil, err
	}
	list, err := p.ListModels(ctx)
	if err != nil {
		return nil, describeLLMError(err, "拉取模型列表失败")
	}
	return list, nil
}
