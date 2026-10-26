// Package provider 管理模型供应商与模型配置（providers / models 表、secrets.json），
// 并负责把配置组装成可调用的 llm.Provider：适配器 + 重试 + 调试追踪。
//
// 文件组织：
//
//	service.go    服务、组装与解析（runtime.ModelResolver）
//	providers.go  供应商的增删改查（密钥写 secrets.json，永不返回前端）
//	models.go     模型的增删改查（按内置预设预填，presets.go）
//	probe.go      连接测试（模型列表 → 文本 → 工具往返 → 看图）
//
// 组装顺序（外层先执行）：WithTrace(WithRetry(adapter))
//   - trace 在最外层：记录的是运行器真正看到的事件序列（含重试后的结果）；适配器另把协议原始报文写到 <追踪目录>/raw/；
//   - --fake-llm 模式下所有模型都解析为假模型，便于不配置任何 Key 就能跑通全流程。
package provider

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/anthropic"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/llm/gemini"
	"github.com/wahhheee/aint/internal/llm/openai"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/store"
)

// Options 是供应商服务的选项。
type Options struct {
	Fake     *fake.Script // 非空表示 --fake-llm 模式
	TraceDir string       // 非空表示 --trace-llm：调用记录写到该目录
}

// Service 是供应商服务。并发安全。
type Service struct {
	db      *store.DB
	blobs   llm.BlobReader
	secrets *Secrets
	opts    Options
	fake    llm.Provider

	mu    sync.Mutex
	built map[string]llm.Provider // provider ID → 组装好的 Provider
}

// NewService 创建供应商服务。blobs 用于读取请求中的图片。
func NewService(db *store.DB, blobs *store.Blobs, secrets *Secrets, opts Options) *Service {
	s := &Service{db: db, blobs: blobReader{db: db, blobs: blobs}, secrets: secrets, opts: opts, built: map[string]llm.Provider{}}
	if opts.Fake != nil {
		s.fake = s.decorate(fake.New(opts.Fake))
	}
	return s
}

// blobReader 让适配器按哈希读取图片。
type blobReader struct {
	db    *store.DB
	blobs *store.Blobs
}

func (b blobReader) ReadBlob(ctx context.Context, hash string) ([]byte, error) {
	if b.blobs == nil {
		return nil, errs.NotFound("没有图片存储")
	}
	return b.blobs.Get(ctx, b.db.Read(), hash)
}

// FakeMode 表示是否处于 --fake-llm 模式。
func (s *Service) FakeMode() bool { return s.fake != nil }

// Resolve 根据模型配置 ID 返回可调用的 Provider 与模型（runtime.ModelResolver）。
// 返回的模型价格已乘以供应商的价格倍率（D-48），运行时直接用于计费。
func (s *Service) Resolve(ctx context.Context, modelID string) (llm.Provider, *llm.Model, error) {
	if s.fake != nil {
		return s.fake, fake.Model(), nil
	}
	if modelID == "" {
		return nil, nil, errs.New(errs.CodeModelMissing, "开始之前，需要连接一个模型")
	}
	m, err := s.loadModel(ctx, s.db.Read(), modelID)
	if err != nil {
		return nil, nil, err
	}
	cfg, multiplier, err := s.loadProvider(ctx, m.ProviderID)
	if err != nil {
		return nil, nil, err
	}
	p, err := s.providerFor(cfg)
	if err != nil {
		return nil, nil, err
	}
	m.Price = m.Price.Scaled(multiplier)
	return p, m, nil
}

func (s *Service) providerFor(cfg llm.ProviderConfig) (llm.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.built[cfg.ID]; ok {
		return p, nil
	}
	adapter, err := NewAdapter(cfg)
	if err != nil {
		return nil, err
	}
	p := s.decorate(adapter)
	s.built[cfg.ID] = p
	return p, nil
}

func (s *Service) decorate(p llm.Provider) llm.Provider {
	p = llm.WithRetry(p, llm.DefaultRetry)
	if s.opts.TraceDir != "" {
		p = llm.WithTrace(p, s.opts.TraceDir)
	}
	return p
}

// adapterConfig 补齐适配器需要的运行环境（图片读取、原始报文追踪）。
func (s *Service) adapterConfig(cfg llm.ProviderConfig) llm.ProviderConfig {
	cfg.Blobs = s.blobs
	if s.opts.TraceDir != "" {
		cfg.TraceDir = filepath.Join(s.opts.TraceDir, "raw")
	}
	return cfg
}

// NewAdapter 按协议创建适配器（不含重试与追踪装饰）。
func NewAdapter(cfg llm.ProviderConfig) (llm.Provider, error) {
	switch cfg.Protocol {
	case llm.ProtocolOpenAIResponses:
		return openai.New(cfg), nil
	case llm.ProtocolAnthropic:
		return anthropic.New(cfg), nil
	case llm.ProtocolGemini:
		return gemini.New(cfg), nil
	}
	return nil, errs.Invalid("不支持的协议：%s", cfg.Protocol)
}

// DefaultBaseURL 返回协议的官方地址（添加供应商时预填）。
func DefaultBaseURL(p llm.Protocol) string {
	switch p {
	case llm.ProtocolOpenAIResponses:
		return openai.DefaultBaseURL
	case llm.ProtocolAnthropic:
		return anthropic.DefaultBaseURL
	case llm.ProtocolGemini:
		return gemini.DefaultBaseURL
	}
	return ""
}

// invalidate 在供应商配置变更后丢弃缓存的 Provider。
func (s *Service) invalidate(providerID string) {
	s.mu.Lock()
	delete(s.built, providerID)
	s.mu.Unlock()
}
