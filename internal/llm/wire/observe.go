package wire

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
)

func sensitiveHeader(name string) bool {
	n := strings.ToLower(name)
	return secretHeaders[n] || n == "set-cookie" || n == "proxy-authorization" ||
		strings.Contains(n, "token") || strings.Contains(n, "secret") || strings.Contains(n, "api-key") || strings.Contains(n, "apikey")
}

func safeHeaders(h http.Header) http.Header {
	out := h.Clone()
	for k := range out {
		if sensitiveHeader(k) {
			out[k] = []string{"***"}
		}
	}
	return out
}

func safeURL(u *url.URL) string {
	v := *u
	if v.User != nil {
		v.User = url.User("***")
	}
	q := v.Query()
	for k := range q {
		if sensitiveHeader(k) || strings.EqualFold(k, "key") {
			q.Set(k, "***")
		}
	}
	v.RawQuery = q.Encode()
	return v.String()
}

type observedBody struct {
	io.ReadCloser
	o      llm.HTTPObserver
	err    error
	closed bool
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.o.Body(p[:n])
	}
	if err != nil && !errors.Is(err, io.EOF) {
		b.err = err
	}
	return n, err
}

func (b *observedBody) Close() error {
	if b.closed {
		return nil
	}
	b.closed = true
	err := b.ReadCloser.Close()
	if b.err == nil {
		b.err = err
	}
	b.o.End(b.err)
	return err
}
