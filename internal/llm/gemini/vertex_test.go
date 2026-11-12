package gemini

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

func TestVertexRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, base, project, location, mode, wantBase, wantPath, wantKey, wantAuth string
	}{
		{"Express", "", "", "", "api_key", VertexBaseURL, "/v1/publishers/google/models/", "secret", ""},
		{"全球", VertexBaseURL + "/v1", "my-project", "global", "api_key", VertexBaseURL, "/v1/projects/my-project/locations/global/publishers/google/models/", "secret", ""},
		{"区域", "", "my-project", "us-central1", "access_token", "https://us-central1-aiplatform.googleapis.com", "/v1/projects/my-project/locations/us-central1/publishers/google/models/", "", "Bearer secret"},
		{"网关", "https://gateway.example/prefix/v1", "p", "asia-east1", "api_key", "https://gateway.example/prefix", "/v1/projects/p/locations/asia-east1/publishers/google/models/", "secret", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewVertex(llm.ProviderConfig{BaseURL: tc.base, APIKey: "secret", Options: jsonx.MustMarshal(VertexOptions{Project: tc.project, Location: tc.location, AuthMode: tc.mode})})
			if err != nil {
				t.Fatal(err)
			}
			if p.Protocol() != llm.ProtocolVertex || p.ep.BaseURL != tc.wantBase || p.modelPrefix != tc.wantPath || p.ep.Header.Get("X-Goog-Api-Key") != tc.wantKey || p.ep.Header.Get("Authorization") != tc.wantAuth {
				t.Fatalf("端点或鉴权不匹配：%s %s", p.ep.BaseURL, p.modelPrefix)
			}
		})
	}
}

func TestVertexInvalidOptions(t *testing.T) {
	for _, opts := range []VertexOptions{
		{AuthMode: "unknown"}, {AuthMode: "access_token"}, {AuthMode: "service_account"},
		{Project: "../p"}, {Project: "p", Location: "x/y"}, {Location: "us-central1"},
	} {
		if _, err := NewVertex(llm.ProviderConfig{APIKey: "invalid", Options: jsonx.MustMarshal(opts)}); err == nil {
			t.Fatalf("应拒绝选项：%+v", opts)
		}
	}
}

func TestVertexStreamAndNativeReplay(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.String() != "/v1/publishers/google/models/gemini-test:streamGenerateContent?alt=sse" || r.Header.Get("X-Goog-Api-Key") != "secret" {
			t.Errorf("请求路径或鉴权错误")
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "CIVIC_INTEGRITY") || strings.Contains(string(body), "JAILBREAK") {
			t.Error("Vertex AI 不应携带不支持的安全类别")
		}
		if calls == 2 && !strings.Contains(string(body), `"thoughtSignature":"signed"`) {
			t.Error("工具往返丢失原生签名")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"weather","args":{}},"thoughtSignature":"signed"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`+"\n\n")
	}))
	defer srv.Close()
	p, err := NewVertex(llm.ProviderConfig{BaseURL: srv.URL, APIKey: "secret", HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	m := &llm.Model{Name: "gemini-test"}
	s, err := p.Stream(t.Context(), m, userHi)
	if err != nil {
		t.Fatal(err)
	}
	_, done := drain(t, s)
	if len(done.Message.ToolCalls()) != 1 || len(done.Message.Native["vertex/gemini-test"]) == 0 {
		t.Fatalf("流解析丢失工具或原生部件：%+v", done)
	}
	call := done.Message.ToolCalls()[0]
	req := &llm.Request{Messages: append(append([]llm.Message{}, userHi.Messages...), done.Message, llm.Message{Role: llm.RoleUser, Parts: []llm.Part{{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: call.ID, Parts: []llm.Part{llm.Text("sunny")}}}}})}
	s, err = p.Stream(t.Context(), m, req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = drain(t, s)
	if calls != 2 {
		t.Fatalf("请求次数 = %d", calls)
	}
}

type vertexRoundTripper func(*http.Request) (*http.Response, error)

func (f vertexRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVertexServiceAccountRefresh(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	secret := string(jsonx.MustMarshal(vertexAccount{Type: "service_account", ProjectID: "from-json", ClientEmail: "test@example.iam.gserviceaccount.com", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))}))
	var exchanges atomic.Int32
	client := &http.Client{Transport: vertexRoundTripper(func(r *http.Request) (*http.Response, error) {
		exchanges.Add(1)
		if r.URL.String() != vertexTokenURL {
			t.Errorf("令牌必须发送至 Google 官方端点")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		parts := strings.Split(r.Form.Get("assertion"), ".")
		if len(parts) != 3 {
			t.Fatal("JWT 格式错误")
		}
		hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, hash[:], sig); err != nil {
			t.Error("JWT 签名错误")
		}
		claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
		if !strings.Contains(string(claims), "cloud-platform") || !strings.Contains(string(claims), vertexTokenURL) {
			t.Error("JWT scope 或 audience 错误")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"oauth-value","expires_in":3600}`)), Header: http.Header{}}, nil
	})}
	source, err := newVertexTokenSource(secret, client)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			value, err := source.token(context.Background())
			if err != nil || value != "oauth-value" {
				t.Errorf("令牌读取失败：%v", err)
			}
		})
	}
	wg.Wait()
	if exchanges.Load() != 1 {
		t.Fatal("并发请求应共享令牌刷新")
	}
	source.expires = time.Now().Add(30 * time.Second)
	if _, err := source.token(t.Context()); err != nil || exchanges.Load() != 2 {
		t.Fatalf("令牌应提前刷新：%v", err)
	}
	p, err := NewVertex(llm.ProviderConfig{APIKey: secret, HTTP: client, Options: jsonx.MustMarshal(VertexOptions{AuthMode: "service_account"})})
	if err != nil || !strings.Contains(p.modelPrefix, "/projects/from-json/") || p.accessToken == nil {
		t.Fatalf("应从服务账号读取项目：%v", err)
	}
}
