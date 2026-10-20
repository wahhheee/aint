package llm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"time"
)

// RetryPolicy 是自动重试策略（《02-技术方案》7.7）。
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetry：退避 1s、2s、4s（加抖动），最多 3 次。
var DefaultRetry = RetryPolicy{MaxRetries: 3, BaseDelay: time.Second, MaxDelay: 8 * time.Second}

// WithRetry 包装 Provider：只在"尚未收到任何流式内容"时自动重试可重试的错误。
//
// 为此，Stream 会预读第一个事件后才返回——首个事件之前的失败都可以安全重试；
// 一旦开始输出，中途断开交给运行器按"截断"处理（续写），不在这里重试。
// 不会自动切换到其他供应商或模型。
func WithRetry(p Provider, pol RetryPolicy) Provider {
	return &retryProvider{Provider: p, pol: pol}
}

type retryProvider struct {
	Provider
	pol RetryPolicy
}

func (r *retryProvider) Stream(ctx context.Context, m *Model, req *Request) (Stream, error) {
	for attempt := 0; ; attempt++ {
		s, err := r.Provider.Stream(ctx, m, req)
		if err == nil {
			ev, nerr := s.Next()
			if nerr == nil || errors.Is(nerr, io.EOF) {
				return &peekedStream{Stream: s, first: ev, firstErr: nerr}, nil
			}
			_ = s.Close()
			err = nerr
		}

		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.Retryable() || attempt >= r.pol.MaxRetries || ctx.Err() != nil {
			return nil, err
		}
		wait := r.backoff(attempt, apiErr.RetryAfter)
		slog.WarnContext(ctx, "模型调用失败，稍后重试",
			"attempt", attempt+1, "max", r.pol.MaxRetries, "wait", wait, "kind", apiErr.Kind, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

func (r *retryProvider) backoff(attempt int, retryAfter time.Duration) time.Duration {
	d := r.pol.BaseDelay << attempt
	d += time.Duration(rand.Int64N(int64(r.pol.BaseDelay/4) + 1)) // 抖动
	if retryAfter > d {
		d = retryAfter // 遵守 Retry-After
	}
	return min(d, max(r.pol.MaxDelay, retryAfter))
}

// peekedStream 先吐出预读的第一个事件，再透传其余事件。
type peekedStream struct {
	Stream
	first    Event
	firstErr error
	consumed bool
}

func (p *peekedStream) Next() (Event, error) {
	if !p.consumed {
		p.consumed = true
		return p.first, p.firstErr
	}
	return p.Stream.Next()
}
