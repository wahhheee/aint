package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/logx"
)

// middleware 组装中间件链（外层先执行）：
//
//	recover → 请求 ID 与访问日志 → Host 校验 → 跨站校验 → 访问密码（TODO(M6-9)）→ 路由
func (s *Server) middleware(next http.Handler) http.Handler {
	h := next
	h = s.auth(h)
	h = crossOrigin(h)
	h = hostGuard(h)
	h = accessLog(h)
	h = recoverer(h)
	return h
}

// recoverer 兜住处理函数中的 panic，返回 500 并记录堆栈。
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				slog.ErrorContext(r.Context(), "处理请求时 panic", "panic", p, "stack", string(debug.Stack()))
				writeError(w, r, errs.New(errs.CodeInternal, "服务器内部错误"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder 记录响应状态码（访问日志用），并透传 Flush（SSE 需要）。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// accessLog 为每个请求分配 request_id（响应头 X-Request-ID、错误体、日志中都有），并记录访问日志。
// 普通成功的读请求记 Debug，其余记 Info，避免日志被轮询淹没。
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		r = r.WithContext(logx.With(r.Context(), "request_id", id))
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		level := slog.LevelDebug
		if r.Method != http.MethodGet || rec.status >= 400 {
			level = slog.LevelInfo
		}
		slog.Log(r.Context(), level, "HTTP", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"duration", time.Since(start).Round(time.Microsecond))
	})
}

func newRequestID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// hostGuard 防 DNS 重绑定：Host 必须是 IP 字面量或 localhost（《02-技术方案》18.1，对玩家无感）。
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && net.ParseIP(host) == nil {
			writeError(w, r, errs.New(errs.CodeForbidden, "请通过 IP 地址或 localhost 访问（当前 Host：%s）", r.Host))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// crossOrigin 拒绝跨站的有副作用请求（防止恶意网页调用本地接口、消耗额度或删改存档）。
// 使用标准库的 CrossOriginProtection：依据 Sec-Fetch-Site 与 Origin/Host 判断，安全方法放行。
func crossOrigin(next http.Handler) http.Handler {
	p := http.NewCrossOriginProtection()
	p.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, errs.New(errs.CodeForbidden, "跨站请求已被拒绝"))
	}))
	return p.Handler(next)
}

// auth 是可选访问密码（默认无鉴权，决策 D-17）。
//
// TODO(M6-9)：设置了访问密码时，除登录接口与登录页外的请求都需要有效的 HttpOnly Cookie；
// 登录失败节流（同一来源每分钟 5 次）；监听非回环地址且未设密码时在界面顶部警告。
func (s *Server) auth(next http.Handler) http.Handler {
	return next
}
