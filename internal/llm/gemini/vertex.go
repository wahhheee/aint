package gemini

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/wire"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// VertexBaseURL 是 Vertex AI 的全球端点；区域端点由 location 决定。
const VertexBaseURL = "https://aiplatform.googleapis.com"

// VertexOptions 只包含非敏感连接选项。凭据通过 ProviderConfig.APIKey 提供。
type VertexOptions struct {
	Project  string `json:"project,omitzero"`
	Location string `json:"location,omitzero"`
	AuthMode string `json:"auth_mode,omitzero"` // api_key | access_token | service_account
}

var vertexSegment = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]*$`)

// NewVertex 创建复用 Gemini 请求体和流解析的 Vertex AI 连接。
// 无项目的 API Key 使用 Express 模式；标准模式要求项目与区域。
func NewVertex(cfg llm.ProviderConfig) (*Provider, error) {
	var opts VertexOptions
	if len(cfg.Options) > 0 {
		if err := jsonx.Unmarshal(cfg.Options, &opts); err != nil {
			return nil, errs.Invalid("Vertex AI 连接选项无效")
		}
	}
	opts.Project, opts.Location = strings.TrimSpace(opts.Project), strings.TrimSpace(opts.Location)
	if opts.AuthMode == "" {
		opts.AuthMode = "api_key"
	}
	auth := http.Header{}
	var token func(context.Context) (string, error)
	switch opts.AuthMode {
	case "api_key":
		auth.Set("X-Goog-Api-Key", cfg.APIKey)
	case "access_token":
		auth.Set("Authorization", "Bearer "+cfg.APIKey)
	case "service_account":
		source, err := newVertexTokenSource(cfg.APIKey, cfg.HTTP)
		if err != nil {
			return nil, err
		}
		token = source.token
		if opts.Project == "" {
			opts.Project = source.account.ProjectID
		}
	default:
		return nil, errs.Invalid("Vertex AI 认证方式无效")
	}
	if opts.AuthMode != "api_key" && opts.Project == "" {
		return nil, errs.Invalid("Vertex AI 标准模式需要项目 ID")
	}
	if opts.Project == "" && opts.Location != "" && opts.Location != "global" {
		return nil, errs.Invalid("Vertex AI Express 模式不支持指定区域，请填写项目 ID")
	}
	if opts.Location == "" {
		opts.Location = "global"
	}
	if (opts.Project != "" && !vertexSegment.MatchString(opts.Project)) || !vertexSegment.MatchString(opts.Location) {
		return nil, errs.Invalid("Vertex AI 项目 ID 或区域格式无效")
	}
	base := wire.TrimVersion(cfg.BaseURL, "v1", "v1beta1")
	if base == "" || base == VertexBaseURL {
		base = VertexBaseURL
		if opts.Project != "" && opts.Location != "global" {
			base = "https://" + opts.Location + "-aiplatform.googleapis.com"
		}
	}
	prefix := "/v1/"
	if opts.Project != "" {
		prefix += "projects/" + wire.PathEscape(opts.Project) + "/locations/" + wire.PathEscape(opts.Location) + "/"
	}
	prefix += "publishers/google/models/"
	ep := wire.NewEndpoint(llm.ProtocolVertex, base, auth, cfg.Headers, cfg.HTTP, cfg.TraceDir)
	ep.Classify = classify
	return &Provider{cfg: cfg, ep: ep, protocol: llm.ProtocolVertex, modelPrefix: prefix, accessToken: token}, nil
}
