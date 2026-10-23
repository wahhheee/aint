package wire

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDebugRedaction(t *testing.T) {
	u, err := url.Parse("https://account:password@example.test/v1?key=private&token=private&model=deepseek")
	if err != nil {
		t.Fatal(err)
	}
	got := safeURL(u)
	if strings.Contains(got, "private") || strings.Contains(got, "password") || strings.Contains(got, "account") || !strings.Contains(got, "model=deepseek") {
		t.Fatalf("URL 脱敏：%s", got)
	}
	h := http.Header{"Authorization": {"Bearer private"}, "Set-Cookie": {"private"}, "X-Auth-Token": {"private"}, "X-Request-Id": {"visible"}}
	safe := safeHeaders(h)
	if safe.Get("Authorization") != "***" || safe.Get("Set-Cookie") != "***" || safe.Get("X-Auth-Token") != "***" || safe.Get("X-Request-Id") != "visible" {
		t.Fatalf("请求头脱敏：%v", safe)
	}
	if h.Get("Authorization") != "Bearer private" {
		t.Fatal("脱敏不能改变实际请求")
	}
	err = transportError(context.Background(), &url.Error{Op: "Post", URL: u.String(), Err: context.DeadlineExceeded})
	if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "password") {
		t.Fatalf("传输错误泄漏凭证：%v", err)
	}
}
