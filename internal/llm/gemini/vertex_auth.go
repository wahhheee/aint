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
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

const vertexTokenURL = "https://oauth2.googleapis.com/token" //nolint:gosec // G101：Google 官方 OAuth 端点，不包含凭据

type vertexAccount struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

// vertexTokenSource 缓存服务账号令牌，并在过期前一分钟刷新；并发请求共享一次刷新。
type vertexTokenSource struct {
	account vertexAccount
	key     *rsa.PrivateKey
	client  *http.Client
	mu      sync.Mutex
	value   string
	expires time.Time
}

func newVertexTokenSource(secret string, client *http.Client) (*vertexTokenSource, error) {
	var account vertexAccount
	if err := jsonx.Unmarshal([]byte(secret), &account); err != nil || account.Type != "service_account" || account.ClientEmail == "" {
		return nil, errs.Invalid("请填写完整的 Google 服务账号 JSON")
	}
	block, _ := pem.Decode([]byte(account.PrivateKey))
	if block == nil {
		return nil, errs.Invalid("服务账号 JSON 中的 private_key 无效")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errs.Invalid("服务账号私钥必须为 PKCS#8 格式")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errs.Invalid("服务账号私钥必须为 RSA 格式")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &vertexTokenSource{account: account, key: key, client: client}, nil
}

func (s *vertexTokenSource) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value != "" && time.Until(s.expires) > time.Minute {
		return s.value, nil
	}
	now := time.Now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := jsonx.MustMarshal(struct {
		Issuer   string `json:"iss"`
		Scope    string `json:"scope"`
		Audience string `json:"aud"`
		Issued   int64  `json:"iat"`
		Expires  int64  `json:"exp"`
	}{s.account.ClientEmail, "https://www.googleapis.com/auth/cloud-platform", vertexTokenURL, now.Unix(), now.Add(time.Hour).Unix()})
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	hash := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, hash[:])
	if err != nil {
		return "", errs.Invalid("无法签署服务账号认证请求")
	}
	assertion := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, vertexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", &llm.APIError{Kind: llm.ErrCanceled, Message: "Google 认证请求已取消", Err: ctx.Err()}
		}
		return "", &llm.APIError{Kind: llm.ErrNetwork, Message: "无法连接 Google 令牌服务"}
	}
	defer func() { _ = resp.Body.Close() }()
	// 认证响应不进入原始请求追踪，错误正文不回传，避免泄露凭据。
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if resp.StatusCode != http.StatusOK || err != nil || jsonx.Unmarshal(data, &out) != nil || out.AccessToken == "" || out.ExpiresIn <= 0 {
		return "", &llm.APIError{Kind: llm.ErrAuth, Status: resp.StatusCode, Message: "Google 服务账号认证失败，请检查私钥、账号状态与本机时间"}
	}
	s.value, s.expires = out.AccessToken, now.Add(time.Duration(out.ExpiresIn)*time.Second)
	return s.value, nil
}
