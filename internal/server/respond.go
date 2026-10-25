package server

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// maxBodyBytes 是 JSON 请求体上限（上传图片走单独的端点）。
const maxBodyBytes = 1 << 20

// handlerFunc 是返回 error 的处理函数：错误统一由 h() 转为 JSON 错误响应，处理函数里不必各自写错误输出。
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (s *Server) h(fn handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			writeError(w, r, err)
		}
	}
}

// writeJSON 写 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := jsonx.MarshalWrite(w, v); err != nil {
		slog.Warn("写 JSON 响应失败", "err", err)
	}
}

// errorBody 是错误响应：{error: {code, message, details, request_id}}（《02-技术方案》18.2）。
// request_id 方便玩家反馈问题时在日志中定位。
type errorBody struct {
	Error errorInfo `json:"error"`
}

type errorInfo struct {
	Code      errs.Code `json:"code"`
	Message   string    `json:"message"`
	Details   any       `json:"details,omitzero"`
	RequestID string    `json:"request_id,omitzero"`
}

// statusOf 把错误码映射为 HTTP 状态码。新增错误码时在这里补充。
func statusOf(code errs.Code) int {
	switch code {
	case errs.CodeRateLimited:
		return http.StatusTooManyRequests
	case errs.CodeInvalidArgument:
		return http.StatusBadRequest
	case errs.CodeNotFound:
		return http.StatusNotFound
	case errs.CodeConflict:
		return http.StatusConflict
	case errs.CodeUnauthorized:
		return http.StatusUnauthorized
	case errs.CodeForbidden:
		return http.StatusForbidden
	case errs.CodeWorkInvalid:
		return http.StatusUnprocessableEntity
	case errs.CodeModelMissing:
		return http.StatusPreconditionFailed
	case errs.CodeNotImplemented:
		return http.StatusNotImplemented
	case errs.CodeUnavailable:
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

// writeError 写错误响应。业务错误原样返回文案；内部错误只返回通用文案，完整错误链写日志。
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	reqID := w.Header().Get("X-Request-ID")
	info := errorInfo{Code: errs.CodeInternal, Message: "服务器内部错误（详见日志）", RequestID: reqID}
	if e, ok := errs.As(err); ok {
		info.Code, info.Message, info.Details = e.Code, e.Message, e.Details
	}
	status := statusOf(info.Code)
	if status >= 500 && status != http.StatusNotImplemented {
		slog.ErrorContext(r.Context(), "请求处理失败", "method", r.Method, "path", r.URL.Path, "err", err)
	} else {
		slog.DebugContext(r.Context(), "请求被拒绝", "method", r.Method, "path", r.URL.Path, "status", status, "err", err)
	}
	writeJSON(w, status, errorBody{Error: info})
}

// decode 解析 JSON 请求体（拒绝未知字段，尽早暴露前后端契约不一致）。
func decode(r *http.Request, v any) error {
	body := http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	if err := jsonx.UnmarshalStrict(body, v); err != nil {
		if errors.Is(err, io.EOF) {
			return errs.Invalid("请求体为空")
		}
		return errs.Invalid("请求格式不正确：%v", err)
	}
	return nil
}

// readBody 读取原始请求体（用于部分更新等需要自行解析的场景）。
func readBody(r *http.Request) ([]byte, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBodyBytes))
	if err != nil {
		return nil, errs.Invalid("读取请求体失败：%v", err)
	}
	return b, nil
}
