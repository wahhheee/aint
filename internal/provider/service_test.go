package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/store"
)

func newService(t *testing.T) (*Service, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "aint.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx, filepath.Join(dir, "backup")); err != nil {
		t.Fatal(err)
	}
	secretsPath := filepath.Join(dir, "secrets.json")
	secrets, err := OpenSecrets(secretsPath)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(db, store.NewBlobs(filepath.Join(dir, "blobs")), secrets, Options{}), secretsPath
}

func TestProviderCRUD(t *testing.T) {
	ctx := context.Background()
	s, secretsPath := newService(t)

	if _, err := s.Create(ctx, ProviderInput{Name: "x", Protocol: llm.ProtocolAnthropic}); errs.CodeOf(err) != errs.CodeInvalidArgument {
		t.Fatalf("缺少 Key 应报参数错误：%v", err)
	}
	if _, err := s.Create(ctx, ProviderInput{Name: "x", Protocol: llm.ProtocolAnthropic, APIKey: "k", Headers: map[string]string{"Authorization": "Bearer k"}}); err == nil {
		t.Fatal("额外请求头里放密钥应被拒绝")
	}

	p, err := s.Create(ctx, ProviderInput{Name: "我的中转站", Protocol: llm.ProtocolGemini, APIKey: "sk-secret-value-1234", PriceMultiplier: 2})
	if err != nil {
		t.Fatal(err)
	}
	if p.BaseURL != "https://generativelanguage.googleapis.com" || p.PriceMultiplier != 2 || p.TestOK != nil {
		t.Fatalf("新建的供应商 = %+v", p)
	}
	if strings.Contains(p.KeyMasked, "secret-value") {
		t.Fatalf("密钥没有掩码：%s", p.KeyMasked)
	}
	raw, _ := os.ReadFile(secretsPath)
	if !strings.Contains(string(raw), "sk-secret-value-1234") {
		t.Fatal("密钥应写入 secrets.json")
	}

	// 添加模型：按预设预填
	m, err := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, Name: "gemini-3.8-flash"})
	if err != nil {
		t.Fatal(err)
	}
	if m.ContextWindow != 1_048_576 || m.Caps.Reasoning != "level" || m.Caps.ReasoningMap["off"] != "low" || m.DisplayName != "Gemini 3.8 Flash" {
		t.Fatalf("预设没有生效：%+v", m)
	}
	if _, err := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, Name: "gemini-3.8-flash"}); errs.CodeOf(err) != errs.CodeConflict {
		t.Fatalf("重复添加应冲突：%v", err)
	}
	// 没有预设的模型：保守默认值；填写价格
	in, out := 1.5, 7.5
	other, err := s.CreateModel(ctx, ModelInput{ProviderID: p.ID, Name: "my-custom-model", Price: &llm.Price{Input: &in, Output: &out, Currency: "USD"}})
	if err != nil {
		t.Fatal(err)
	}
	if other.Caps.Reasoning != "none" || other.ContextWindow != 128_000 {
		t.Fatalf("默认值不对：%+v", other)
	}

	// Resolve：价格乘以倍率；列表中是模型单价
	_, resolved, err := s.Resolve(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *resolved.Price.Input != 3 || *resolved.Price.Output != 15 {
		t.Fatalf("倍率没有生效：%+v", resolved.Price)
	}
	list, _ := s.Models(ctx)
	for _, x := range list {
		if x.ID == other.ID && *x.Price.Input != 1.5 {
			t.Fatalf("列表应显示模型单价：%v", *x.Price.Input)
		}
	}

	// 修改：协议不可改；改 Key 不回显；倍率修改
	if _, err := s.Update(ctx, p.ID, ProviderInput{Protocol: llm.ProtocolAnthropic}); err == nil {
		t.Fatal("协议不应可改")
	}
	up, err := s.Update(ctx, p.ID, ProviderInput{Name: "改名", APIKey: "sk-new-key-5678", PriceMultiplier: 1}) //nolint:gosec // G101：测试用的假密钥
	if err != nil || up.Name != "改名" || up.PriceMultiplier != 1 {
		t.Fatalf("修改失败：%+v %v", up, err)
	}
	raw, _ = os.ReadFile(secretsPath)
	if !strings.Contains(string(raw), "sk-new-key-5678") {
		t.Fatal("新密钥应写入 secrets.json")
	}

	// 删除：模型级联删除，密钥删除
	if err := s.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Models(ctx); len(list) != 0 {
		t.Fatalf("模型应级联删除：%d", len(list))
	}
	raw, _ = os.ReadFile(secretsPath)
	if strings.Contains(string(raw), "sk-new-key") {
		t.Fatal("密钥应随供应商删除")
	}
	if _, _, err := s.Resolve(ctx, m.ID); errs.CodeOf(err) != errs.CodeModelMissing {
		t.Fatalf("删除后解析应提示模型不存在：%v", err)
	}
}

// TestProbe 用本地假服务验证连接测试的四个步骤与结果记录。
func TestProbe(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-5-5"}]}`))
			return
		}
		var body struct {
			Messages []jsonx.Value `json:"messages"`
		}
		_ = jsonx.UnmarshalRead(r.Body, &body)
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(events ...string) {
			for _, e := range events {
				_, _ = w.Write([]byte("data: " + e + "\n\n"))
			}
		}
		start := `{"type":"message_start","message":{"usage":{"input_tokens":10}}}`
		stop := func(reason string) string {
			return `{"type":"message_delta","delta":{"stop_reason":"` + reason + `"},"usage":{"output_tokens":5}}`
		}
		text := func(s string) []string {
			return []string{`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + s + `"}}`,
				`{"type":"content_block_stop","index":0}`}
		}
		raw := string(body.Messages[len(body.Messages)-1])
		switch {
		case strings.Contains(raw, "probe_echo") && !strings.Contains(raw, "tool_result"):
			send(start, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"probe_echo","input":{}}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"text\":\"雾港\"}"}}`,
				`{"type":"content_block_stop","index":0}`, stop("tool_use"), `{"type":"message_stop"}`)
		case strings.Contains(raw, "tool_result"):
			code := raw[strings.Index(raw, "校验码：")+len("校验码："):]
			code = code[:4]
			send(append(append([]string{start}, text(code)...), stop("end_turn"), `{"type":"message_stop"}`)...)
		case strings.Contains(raw, `"image"`):
			send(append(append([]string{start}, text("红色")...), stop("end_turn"), `{"type":"message_stop"}`)...)
		default:
			send(append(append([]string{start}, text("连接成功")...), stop("end_turn"), `{"type":"message_stop"}`)...)
		}
	}))
	defer srv.Close()

	p, err := s.Create(ctx, ProviderInput{Name: "本地", Protocol: llm.ProtocolAnthropic, BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Test(ctx, p.ID, TestInput{Model: "claude-sonnet-5-5"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || len(report.Steps) != 4 {
		t.Fatalf("测试报告 = %+v", report)
	}
	for _, st := range report.Steps {
		if !st.OK {
			t.Fatalf("步骤 %s 未通过：%+v", st.Name, st)
		}
	}
	if calls != 4 { // 文本 1 + 工具 2 + 看图 1
		t.Fatalf("调用次数 = %d", calls)
	}
	got, _ := s.Get(ctx, p.ID)
	if got.TestOK == nil || !*got.TestOK {
		t.Fatalf("测试结果没有记录：%+v", got)
	}
	var n int
	_ = s.db.Read().QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_log WHERE call_kind = 'probe'`).Scan(&n)
	if n != 4 {
		t.Fatalf("用量记录 = %d", n)
	}
}
