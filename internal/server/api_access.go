package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/wahhheee/aint/internal/access"
	"github.com/wahhheee/aint/internal/pkg/errs"
)

const sessionCookie = "aint_session"

func sessionToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func accessCookie(w http.ResponseWriter, r *http.Request, token string) {
	maxAge := int(access.SessionLifetime.Seconds())
	if token == "" {
		maxAge = -1
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: maxAge}) //nolint:gosec // 本地服务支持 HTTP；直接 TLS 连接时才设置 Secure，不能信任转发头。
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) error {
	enabled := s.d.Access != nil && s.d.Access.Enabled()
	authenticated := !enabled || s.d.Access.Valid(sessionToken(r))
	host, _, _ := net.SplitHostPort(s.d.Info.Listen)
	ip := net.ParseIP(host)
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled, "authenticated": authenticated, "exposed": ip != nil && !ip.IsLoopback() && !enabled})
	return nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if s.d.Access == nil {
		return errs.Invalid("当前没有设置访问密码")
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	token, err := s.d.Access.Login(ip, req.Password)
	if err != nil {
		return err
	}
	accessCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": true, "authenticated": true, "exposed": false})
	return nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	if s.d.Access != nil {
		if err := s.d.Access.Logout(sessionToken(r)); err != nil {
			return err
		}
	}
	accessCookie(w, r, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) password(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Current  string `json:"current_password"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if s.d.Access == nil {
		return errs.New(errs.CodeUnavailable, "访问密码服务不可用")
	}
	if err := s.d.Access.SetPassword(req.Current, req.Password, false); err != nil {
		return err
	}
	accessCookie(w, r, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) revokeSessions(w http.ResponseWriter, r *http.Request) error {
	if s.d.Access != nil {
		if err := s.d.Access.RevokeAll(); err != nil {
			return err
		}
	}
	accessCookie(w, r, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) githubToken(w http.ResponseWriter, r *http.Request) error {
	if s.d.Secrets == nil {
		return errs.New(errs.CodeUnavailable, "密钥存储不可用")
	}
	if r.Method == http.MethodPut {
		var req struct {
			Token string `json:"token"`
		}
		if err := decode(r, &req); err != nil {
			return err
		}
		if len(req.Token) > 4096 || strings.ContainsAny(req.Token, "\r\n") {
			return errs.Invalid("GitHub 令牌格式不正确")
		}
		if err := s.d.Secrets.Set("github_token", strings.TrimSpace(req.Token)); err != nil {
			return err
		}
	}
	token, _ := s.d.Secrets.Resolve("github_token")
	writeJSON(w, http.StatusOK, map[string]bool{"configured": token != ""})
	return nil
}
