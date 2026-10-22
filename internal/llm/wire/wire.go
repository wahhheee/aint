// Package wire 是三家协议适配器共用的 HTTP 与 SSE 工具，只用标准库（D-44）。
//
// 职责：
//   - Endpoint：拼接地址、加鉴权与额外请求头、发请求；非 2xx 统一转为 *llm.APIError
//     （状态码 → 错误种类、Retry-After、从各家错误 JSON 中取出 message）；
//   - SSE：逐个读取 Server-Sent Events（event / data），容忍中转站在 JSON 后补空格、注释行等；
//   - 原始报文追踪（--trace-llm）：请求体与原样响应字节写到追踪目录，鉴权头脱敏。
//     这些文件也是录制回放测试（M4-8）的夹具来源。
//
// 适配器只负责"中立类型 ⇄ 协议 JSON"的转换，不碰 net/http 细节。
package wire

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// maxErrorBody 是读取错误响应体的上限（错误信息只需要开头）。
const maxErrorBody = 64 << 10

// defaultClient 不设整体超时（流式响应可能持续数分钟），只限制建连与首包等待。
var defaultClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 3 * time.Minute, // 长上下文首包可能较慢
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// Endpoint 是一个供应商的 HTTP 端点。
type Endpoint struct {
	BaseURL  string      // 已规范化（无结尾斜杠）
	Header   http.Header // 鉴权与额外请求头
	Client   *http.Client
	TraceDir string
	Protocol llm.Protocol
	// Classify 可选：按协议细化错误种类（如上下文超限的特定错误码）。返回 "" 表示沿用默认。
	Classify func(status int, body []byte, msg string) llm.ErrorKind
}

// NewEndpoint 创建端点。extra 为用户配置的额外请求头（不得覆盖鉴权头）。
func NewEndpoint(p llm.Protocol, baseURL string, auth http.Header, extra map[string]string, client *http.Client, traceDir string) *Endpoint {
	h := http.Header{}
	for k, v := range extra {
		h.Set(k, v)
	}
	for k, vs := range auth {
		h[k] = vs
	}
	if client == nil {
		client = defaultClient
	}
	return &Endpoint{BaseURL: strings.TrimRight(baseURL, "/"), Header: h, Client: client, TraceDir: traceDir, Protocol: p}
}

// TrimVersion 去掉用户误写在基础地址末尾的版本段（如 ".../v1"、".../v1beta"），适配器再按协议补上完整路径。
func TrimVersion(base string, versions ...string) string {
	base = strings.TrimRight(base, "/")
	for _, v := range versions {
		if strings.HasSuffix(base, "/"+v) {
			return strings.TrimSuffix(base, "/"+v)
		}
	}
	return base
}

// Call 描述一次请求（用于追踪文件命名）。
type Call struct {
	Purpose string
	Model   string
}

// PostStream 发送 JSON 请求，2xx 时返回响应体（调用方负责 Close）；否则返回 *llm.APIError。
// 返回的 ReadCloser 在开启追踪时会把读到的原始字节同时写入追踪文件。
func (e *Endpoint) PostStream(ctx context.Context, path string, body []byte, call Call) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, &llm.APIError{Kind: llm.ErrInvalidRequest, Message: "构造请求失败", Err: err}
	}
	req.Header = e.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	tr := e.startTrace(req, body, call)
	start := time.Now()
	resp, err := e.Client.Do(req)
	if err != nil {
		tr.fail(err)
		return nil, transportError(ctx, err)
	}
	tr.response(resp, time.Since(start))
	if resp.StatusCode/100 != 2 {
		defer func() { _ = resp.Body.Close() }() // 错误响应体只读开头，关闭错误无影响
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		tr.body(b)
		tr.close()
		return nil, e.statusError(resp, b)
	}
	return tr.wrap(resp.Body), nil
}

// GetJSON 发送 GET 请求并把 JSON 响应解码到 out（模型列表等）。
func (e *Endpoint) GetJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.BaseURL+path, nil)
	if err != nil {
		return &llm.APIError{Kind: llm.ErrInvalidRequest, Message: "构造请求失败", Err: err}
	}
	req.Header = e.Header.Clone()
	req.Header.Set("Accept", "application/json")
	resp, err := e.Client.Do(req)
	if err != nil {
		return transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }() // 已读完整个响应体，关闭错误无影响
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return transportError(ctx, err)
	}
	if resp.StatusCode/100 != 2 {
		return e.statusError(resp, b)
	}
	if err := jsonx.Unmarshal(b, out); err != nil {
		return &llm.APIError{Kind: llm.ErrUnknown, Status: resp.StatusCode, Message: "响应不是预期的 JSON：" + snippet(b), Err: err}
	}
	return nil
}

// transportError 把网络层错误转为 APIError（取消单独归类，不重试）。
func transportError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return &llm.APIError{Kind: llm.ErrCanceled, Err: ctx.Err()}
	}
	return &llm.APIError{Kind: llm.ErrNetwork, Message: "连接供应商失败", Err: err}
}

// StreamError 把读取流过程中的错误转为 APIError（流中途断开、取消）。
func StreamError(ctx context.Context, err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return err
	}
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) {
		return err
	}
	return transportError(ctx, err)
}

func (e *Endpoint) statusError(resp *http.Response, body []byte) error {
	msg, code := ErrorMessage(body)
	kind := llm.KindFromStatus(resp.StatusCode)
	if e.Classify != nil {
		if k := e.Classify(resp.StatusCode, body, msg); k != "" {
			kind = k
		}
	}
	if msg == "" {
		msg = snippet(body)
	}
	if code != "" && !strings.Contains(msg, code) {
		msg = code + ": " + msg
	}
	return &llm.APIError{Kind: kind, Status: resp.StatusCode, Message: msg, RetryAfter: RetryAfter(resp.Header)}
}

// ErrorMessage 从各家错误 JSON 中取出错误信息与类型 / 错误码：
// Anthropic {"type":"error","error":{"type","message"}}；OpenAI {"error":{"message","type","code"}}；
// Gemini {"error":{"code","message","status"}}；中转站常见 {"message"} 或 {"error":"…"}。
func ErrorMessage(body []byte) (msg, code string) {
	var v struct {
		Error   jsonx.Value `json:"error"`
		Message string      `json:"message"`
		Detail  string      `json:"detail"`
	}
	if jsonx.Unmarshal(bytes.TrimSpace(body), &v) != nil {
		return "", ""
	}
	if len(v.Error) > 0 {
		var s string
		if jsonx.Unmarshal(v.Error, &s) == nil {
			return s, ""
		}
		var o struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
			Status  string `json:"status"`
		}
		if jsonx.Unmarshal(v.Error, &o) == nil {
			code := o.Type
			if s, ok := o.Code.(string); ok && s != "" {
				code = s
			}
			if o.Status != "" {
				code = o.Status
			}
			return o.Message, code
		}
	}
	if v.Message != "" {
		return v.Message, ""
	}
	return v.Detail, ""
}

// RetryAfter 解析 Retry-After（秒数或 HTTP 日期）与 retry-after-ms。
func RetryAfter(h http.Header) time.Duration {
	if ms := h.Get("Retry-After-Ms"); ms != "" {
		if n, err := strconv.ParseFloat(ms, 64); err == nil && n > 0 {
			return time.Duration(n * float64(time.Millisecond))
		}
	}
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
		return time.Duration(n * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "…"
	}
	return s
}

// PathEscape 转义 URL 路径段（Gemini 的模型名放在路径中）。
func PathEscape(s string) string { return url.PathEscape(s) }

// ErrNoDone 是流在没有终止事件的情况下结束（连接被提前关闭）。
var ErrNoDone = fmt.Errorf("流在没有结束事件的情况下中断")
