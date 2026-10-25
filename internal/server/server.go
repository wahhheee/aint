// Package server 是 HTTP 层：路由、中间件、错误格式、SSE、静态资源。
//
// 职责边界：只做协议转换（解析请求 → 调用服务 → 编码响应），不含业务规则。
// 业务错误以 *errs.Error 返回，在 respond.go 中统一映射为 HTTP 状态码与 JSON 错误体。
//
// 文件组织：
//
//	routes.go      全部业务路由
//	middleware.go  恢复、请求 ID、访问日志、Host 校验、跨站校验、访问密码
//	respond.go     JSON 编解码与错误响应
//	sse.go         事件流（断线续传、心跳、resync）
//	static.go      前端静态资源与 SPA 回退
//	api_*.go       各资源的处理函数
package server

import (
	"io/fs"
	"net/http"

	"github.com/wahhheee/yanyi/internal/access"

	"github.com/wahhheee/yanyi/internal/buildinfo"
	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/media"
	"github.com/wahhheee/yanyi/internal/provider"
	"github.com/wahhheee/yanyi/internal/runtime"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/settings"
	"github.com/wahhheee/yanyi/internal/usage"
	"github.com/wahhheee/yanyi/internal/work"
)

// SystemInfo 是运行环境信息（/api/v1/system/info；设置页"关于"与"存储"、排查问题时使用）。
type SystemInfo struct {
	Build         buildinfo.Info `json:"build"`
	InstanceID    string         `json:"instance_id"`
	DataDir       string         `json:"data_dir"`
	LogDir        string         `json:"log_dir"`
	Listen        string         `json:"listen"`
	FakeLLM       bool           `json:"fake_llm"`
	TraceLLM      bool           `json:"trace_llm"`
	PromptVersion string         `json:"prompt_version"`
	StartedAt     int64          `json:"started_at"`
	TurnDefaults  TurnDefaults   `json:"turn_defaults"`
}

// TurnDefaults 是界面展示继承值时使用的运行时调用上限。
type TurnDefaults struct {
	MaxCalls     int `json:"max_calls"`
	MaxToolCalls int `json:"max_tool_calls"`
	MaxOutput    int `json:"max_output"`
}

// Deps 是 HTTP 层依赖的服务。
type Deps struct {
	Access    *access.Service
	Secrets   *provider.Secrets
	Info      SystemInfo
	Bus       *event.Bus
	Settings  *settings.Service
	Works     *work.Service
	Imports   *work.Importer
	Saves     *save.Service
	Runtime   *runtime.Manager
	Providers *provider.Service
	Usage     *usage.Service
	Images    *media.Store // 玩家上传、缩略图
	Web       fs.FS        // 前端静态资源（web.Dist()）
}

// Server 是 HTTP 服务。
type Server struct {
	d       Deps
	mux     *http.ServeMux
	handler http.Handler
}

// New 创建 HTTP 服务并注册全部路由。
func New(d Deps) *Server {
	s := &Server{d: d, mux: http.NewServeMux()}
	s.routes()
	s.handler = s.middleware(s.mux)
	return s
}

// Handler 返回带中间件的根处理器。
func (s *Server) Handler() http.Handler { return s.handler }
