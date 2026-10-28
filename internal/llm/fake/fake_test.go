package fake

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/wahhheee/aint/internal/llm"
)

func TestOperationResponses(t *testing.T) {
	p := New(&Script{
		Responses: []Response{{Text: "普通回合"}},
		Operations: map[string][]Response{
			"rewrite": {{Text: "第一版"}, {Text: "第二版"}},
			"choices": {{Text: "首组选项"}, {Text: "后续选项"}},
		},
	})
	for _, tc := range []struct{ save, purpose, want string }{
		{"a", "narrate", "普通回合"},
		{"a", "rewrite", "第一版"},
		{"a", "choices", "首组选项"},
		{"a", "rewrite", "第二版"},
		{"a", "choices", "后续选项"},
		{"a", "rewrite", "第二版"},
		{"b", "rewrite", "第一版"},
		{"b", "choices", "首组选项"},
	} {
		s, err := p.Stream(t.Context(), nil, &llm.Request{CacheKey: tc.save, Purpose: tc.purpose})
		if err != nil {
			t.Fatal(err)
		}
		ev, err := s.Next()
		if err != nil {
			t.Fatal(err)
		}
		if delta, ok := ev.(llm.TextDelta); !ok || delta.Text != tc.want {
			t.Fatalf("存档 %s 用途 %s：得到 %#v，期望 %s", tc.save, tc.purpose, ev, tc.want)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWaitForCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := New(&Script{Responses: []Response{{Text: "雨声", WaitForCancel: true}}})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		s, err := p.Stream(ctx, nil, &llm.Request{})
		if err != nil {
			t.Fatal(err)
		}
		if ev, err := s.Next(); err != nil || ev != (llm.TextDelta{Text: "雨声"}) {
			t.Fatalf("等待取消前的正文：%#v，错误：%v", ev, err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := s.Next()
			done <- err
		}()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("取消前结束了调用：%v", err)
		default:
		}
		cancel()
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("取消后得到 %v", err)
		}
	})
}
