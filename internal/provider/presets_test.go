package provider

import (
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

func TestModelPresetPreview(t *testing.T) {
	s, _ := newService(t)
	p, err := s.Create(t.Context(), ProviderInput{Name: "测试", Protocol: llm.ProtocolAnthropic, APIKey: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	custom := 99.0
	m, err := s.CreateModel(t.Context(), ModelInput{ProviderID: p.ID, Name: "claude-sonnet-5-5", ContextWindow: 9000, Price: &llm.Price{Input: &custom, Currency: "CNY"}})
	if err != nil {
		t.Fatal(err)
	}
	pre, err := s.ModelPreset(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pre == nil || pre.Revision != PresetRevision || pre.ContextWindow != 1_000_000 || pre.Price.Currency != "USD" {
		t.Fatalf("预设=%+v", pre)
	}
	got, err := s.loadModel(t.Context(), s.db.Read(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(jsonx.MustMarshal(got)) != string(jsonx.MustMarshal(m)) {
		t.Fatal("预览预设修改了模型")
	}
	unknown, err := s.CreateModel(t.Context(), ModelInput{ProviderID: p.ID, Name: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	if pre, err := s.ModelPreset(t.Context(), unknown.ID); pre != nil || err != nil {
		t.Fatalf("未知模型应无预设：%+v %v", pre, err)
	}
	if _, ok := PresetFor(llm.ProtocolGemini, "claude-sonnet-5-5"); ok {
		t.Fatal("预设未按协议匹配")
	}
	a, _ := PresetFor(llm.ProtocolAnthropic, "deepseek-flash")
	b, _ := PresetFor(llm.ProtocolOpenAIResponses, "deepseek-flash")
	if a.Caps.ReasoningMap["off"] != "disabled" || b.Caps.ReasoningMap["off"] != "low" {
		t.Fatal("兼容接口推理映射错误")
	}
	*a.Price.Input = 999
	a.Caps.ReasoningMap["off"] = "omit"
	again, _ := PresetFor(llm.ProtocolAnthropic, "deepseek-flash")
	if *again.Price.Input == 999 || again.Caps.ReasoningMap["off"] == "omit" {
		t.Fatal("返回值修改污染了内置预设")
	}
}
