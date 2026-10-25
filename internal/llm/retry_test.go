package llm

import (
	"context"
	"io"
	"testing"
	"time"
)

type retryStub struct {
	Provider
	calls      int
	startError bool
	kind       ErrorKind
	event      Event
}

func (p *retryStub) Stream(context.Context, *Model, *Request) (Stream, error) {
	p.calls++
	if p.calls == 1 {
		err := &APIError{Kind: p.kind, RetryAfter: time.Nanosecond}
		if p.startError {
			return nil, err
		}
		return &retryStubStream{event: p.event, err: err}, nil
	}
	return &retryStubStream{event: Done{Stop: StopEnd}}, nil
}

type retryStubStream struct {
	event Event
	err   error
}

func (s *retryStubStream) Next() (Event, error) {
	if s.event != nil {
		e := s.event
		s.event = nil
		return e, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return nil, io.EOF
}
func (*retryStubStream) Close() error { return nil }

func TestRetryBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start bool
		kind  ErrorKind
		event Event
		calls int
	}{
		{"HTTP限流", true, ErrRateLimited, nil, 2},
		{"流开始前过载", false, ErrOverloaded, nil, 2},
		{"鉴权不重试", true, ErrAuth, nil, 1},
		{"请求错误不重试", false, ErrInvalidRequest, nil, 1},
		{"已有文本不重试", false, ErrNetwork, TextDelta{Text: "雾"}, 1},
		{"已有工具调用不重试", false, ErrServer, ToolCallStart{ID: "c", Name: "read"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &retryStub{startError: tc.start, kind: tc.kind, event: tc.event}
			s, err := WithRetry(p, RetryPolicy{MaxRetries: 2}).Stream(t.Context(), &Model{}, &Request{})
			if s != nil {
				defer func() { _ = s.Close() }()
				for {
					_, nextErr := s.Next()
					if nextErr != nil {
						break
					}
				}
			}
			if tc.calls == 2 && err != nil {
				t.Fatal(err)
			}
			if tc.event == nil && tc.calls == 1 && KindOf(err) != tc.kind {
				t.Fatalf("错误=%v", err)
			}
			if p.calls != tc.calls {
				t.Fatalf("实际调用 %d，期望 %d", p.calls, tc.calls)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	p := &retryStub{startError: true, kind: ErrRateLimited}
	_, _ = WithRetry(p, DefaultRetry).Stream(ctx, &Model{}, &Request{})
	if p.calls != 1 {
		t.Fatal("取消后仍重试")
	}
}

func TestRetryAfter(t *testing.T) {
	r := retryProvider{pol: DefaultRetry}
	if d := r.backoff(0, 30*time.Second); d != 30*time.Second {
		t.Fatalf("Retry-After=%s", d)
	}
}
