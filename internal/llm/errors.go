package llm

import (
	"errors"
	"fmt"
	"time"
)

// ErrorKind 是归一化的错误类别，决定重试策略与界面提示（《04-交互与界面设计》8.9、16）。
type ErrorKind string

const (
	ErrRateLimited     ErrorKind = "rate_limited"     // 429：可重试
	ErrOverloaded      ErrorKind = "overloaded"       // 529 等：可重试
	ErrServer          ErrorKind = "server"           // 5xx：可重试
	ErrNetwork         ErrorKind = "network"          // 连接失败：可重试
	ErrAuth            ErrorKind = "auth"             // 鉴权失败 / 余额不足
	ErrInvalidRequest  ErrorKind = "invalid_request"  // 请求不合法（多为适配器缺陷，需看 trace）
	ErrContextOverflow ErrorKind = "context_overflow" // 上下文超限
	ErrUnsupported     ErrorKind = "unsupported"      // 模型不支持该能力
	ErrCanceled        ErrorKind = "canceled"         // 玩家停止
	ErrUnknown         ErrorKind = "unknown"
)

// APIError 是适配器返回的统一错误。
type APIError struct {
	Kind       ErrorKind
	Status     int           // HTTP 状态码（若有）
	Message    string        // 供应商返回的错误信息
	RetryAfter time.Duration // 供应商要求的等待时间（若有）
	Err        error
	// PartialUsage 是断流前供应商已报告的累计用量，缺失的最终输出用量不估算。
	PartialUsage Usage
}

// WithPartialUsage 给流式错误附上已收到的累计用量，保留原有错误分类与原因。
func WithPartialUsage(err error, u Usage) error {
	if err == nil || u == (Usage{}) {
		return err
	}
	var api *APIError
	if errors.As(err, &api) {
		withUsage := *api
		withUsage.PartialUsage = u
		return &withUsage
	}
	return &APIError{Kind: ErrUnknown, Err: err, PartialUsage: u}
}

// UsageOf 返回错误携带的已知用量，没有报告时为零。
func UsageOf(err error) Usage {
	var api *APIError
	if errors.As(err, &api) {
		return api.PartialUsage
	}
	return Usage{}
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("llm %s", e.Kind)
	if e.Status != 0 {
		s += fmt.Sprintf(" (HTTP %d)", e.Status)
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *APIError) Unwrap() error { return e.Err }

// Retryable 表示该错误在尚未收到任何流式内容时可以自动重试（《02-技术方案》7.7）。
func (e *APIError) Retryable() bool {
	switch e.Kind {
	case ErrRateLimited, ErrOverloaded, ErrServer, ErrNetwork:
		return true
	}
	return false
}

// KindOf 返回错误类别；非 APIError 返回 ErrUnknown。
func KindOf(err error) ErrorKind {
	var e *APIError
	if errors.As(err, &e) {
		return e.Kind
	}
	return ErrUnknown
}

// KindFromStatus 根据 HTTP 状态码给出默认类别，适配器可在此基础上细化。
func KindFromStatus(status int) ErrorKind {
	switch {
	case status == 429:
		return ErrRateLimited
	case status == 529:
		return ErrOverloaded
	case status == 401 || status == 402 || status == 403:
		return ErrAuth
	case status == 400 || status == 404 || status == 422:
		return ErrInvalidRequest
	case status >= 500:
		return ErrServer
	}
	return ErrUnknown
}
