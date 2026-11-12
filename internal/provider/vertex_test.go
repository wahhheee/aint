package provider

import (
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/gemini"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

func TestVertexProviderOptions(t *testing.T) {
	s, _ := newService(t)
	ctx := t.Context()
	opts := &gemini.VertexOptions{Project: "my-project", Location: "us-central1", AuthMode: "api_key"}
	p, err := s.Create(ctx, ProviderInput{Name: "Vertex", Protocol: llm.ProtocolVertex, APIKey: "test-vertex-key", Options: opts}) //nolint:gosec // G101：测试用的假密钥
	if err != nil {
		t.Fatal(err)
	}
	if p.Options == nil || *p.Options != *opts || p.BaseURL != gemini.VertexBaseURL || strings.Contains(string(jsonx.MustMarshal(p)), "test-vertex-key") {
		t.Fatalf("配置或脱敏错误：%+v", p)
	}
	cfg, _, err := s.loadProvider(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewAdapter(cfg)
	if err != nil || adapter.Protocol() != llm.ProtocolVertex {
		t.Fatalf("无法构造 Vertex AI 适配器：%v", err)
	}
	newOpts := &gemini.VertexOptions{Project: "p2", Location: "global", AuthMode: "access_token"}
	if _, err := s.Update(ctx, p.ID, ProviderInput{Options: newOpts}); err == nil {
		t.Fatal("更换认证方式必须填写新凭据")
	}
	up, err := s.Update(ctx, p.ID, ProviderInput{Options: newOpts, APIKey: "new-access-value"})
	if err != nil || up.Options == nil || *up.Options != *newOpts {
		t.Fatalf("选项未更新：%v", err)
	}
	up, err = s.Update(ctx, p.ID, ProviderInput{Name: "改名"})
	if err != nil || up.Options == nil || *up.Options != *newOpts {
		t.Fatalf("改名不应丢失连接选项：%v", err)
	}
	pre, ok := PresetFor(llm.ProtocolVertex, "gemini-3.8-flash")
	if !ok || !pre.Caps.Tools || pre.ContextWindow != 1_048_576 {
		t.Fatal("Vertex AI 应应用 Gemini 模型预设")
	}
}
