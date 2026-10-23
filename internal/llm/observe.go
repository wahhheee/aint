package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
)

// Observer 接收一次逻辑调用及其所有 HTTP 尝试；实现必须并发安全，且不能改变调用结果。
type Observer interface {
	HTTP(method, url string, headers http.Header, body []byte) HTTPObserver
	Event(Event)
	Finish(error)
}

// HTTPObserver 接收适配器实际读取的报文。请求与响应头在进入观察器前脱敏。
type HTTPObserver interface {
	RequestHeader(name string, values []string)
	Response(status int, headers http.Header)
	Body([]byte)
	Truncated()
	End(error)
}

type observerKey struct{}

// ObserverFrom 返回本次调用的观察器；未开启调试时为 nil。
func ObserverFrom(ctx context.Context) Observer {
	o, _ := ctx.Value(observerKey{}).(Observer)
	return o
}

// ObserveStream 在供应商（含自动重试）外观察流，保留错误与中断前的事件。
func ObserveStream(ctx context.Context, p Provider, m *Model, req *Request, o Observer) (Stream, error) {
	if o == nil {
		return p.Stream(ctx, m, req)
	}
	ctx = context.WithValue(ctx, observerKey{}, o)
	s, err := p.Stream(ctx, m, req)
	if err != nil {
		o.Finish(err)
		return nil, err
	}
	return &observedStream{Stream: s, observer: o, ctx: ctx}, nil
}

type observedStream struct {
	Stream
	observer     Observer
	ctx          context.Context
	err          error
	done, closed bool
}

func (s *observedStream) Next() (Event, error) {
	ev, err := s.Stream.Next()
	if err != nil && !errors.Is(err, io.EOF) {
		s.err = err
	}
	if err == nil {
		_, done := ev.(Done)
		s.done = s.done || done
		s.observer.Event(ev)
	}
	return ev, err
}

func (s *observedStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	err := s.Stream.Close()
	if s.err == nil {
		s.err = err
	}
	if s.err == nil && !s.done {
		s.err = s.ctx.Err()
		if s.err == nil {
			s.err = io.ErrUnexpectedEOF
		}
	}
	s.observer.Finish(s.err)
	return err
}
