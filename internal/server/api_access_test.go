package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wahhheee/aint/internal/access"
	"github.com/wahhheee/aint/internal/provider"
)

func TestAccessProtectsAllData(t *testing.T) {
	secrets, err := provider.OpenSecrets(filepath.Join(t.TempDir(), "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	gate, err := access.New(secrets)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.SetPassword("", "password-for-test", true); err != nil {
		t.Fatal(err)
	}
	server := New(Deps{Access: gate, Secrets: secrets, Web: fstest.MapFS{"index.html": {Data: []byte("登录应用")}}})
	for _, path := range []string{"/works", "/settings", "/system/info", "/events", "/assets/sha256:abc", "/uploads/sha256:abc", "/saves/any/events", "/saves/any/debug"} {
		res := do(t, server.Handler(), http.MethodGet, "/api/v1"+path, "", nil)
		if res.Code != http.StatusUnauthorized {
			t.Errorf("未登录泄漏 %s: %d", path, res.Code)
		}
	}
	for _, path := range []string{"/", "/api/v1/auth/status", "/api/v1/system/health"} {
		if res := do(t, server.Handler(), http.MethodGet, path, "", nil); res.Code != 200 {
			t.Errorf("公开端点 %s: %d", path, res.Code)
		}
	}
	login := do(t, server.Handler(), http.MethodPost, "/api/v1/auth/login", "application/json", strings.NewReader(`{"password":"password-for-test"}`))
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("会话 Cookie 属性错误", cookies)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/settings/github-token", nil)
	req.Host = "127.0.0.1"
	req.AddCookie(cookies[0])
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	if err := gate.RevokeAll(); err != nil {
		t.Fatal(err)
	}
	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != 401 {
		t.Fatal("撤销未生效")
	}
}

func TestAccessRevokesOpenStream(t *testing.T) {
	secrets, err := provider.OpenSecrets(filepath.Join(t.TempDir(), "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	gate, err := access.New(secrets)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.SetPassword("", "password-for-test", true); err != nil {
		t.Fatal(err)
	}
	token, err := gate.Login("local", "password-for-test")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{d: Deps{Access: gate}}
	started := make(chan struct{})
	ts := httptest.NewServer(s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if err := http.NewResponseController(w).Flush(); err != nil {
			return
		}
		close(started)
		<-r.Context().Done()
	})))
	defer ts.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: true})
	response, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	<-started
	if err := gate.RevokeAll(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, response.Body); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("撤销没有结束事件流")
	}
}
