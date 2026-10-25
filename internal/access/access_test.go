package access

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
)

type memorySecrets struct {
	value string
	fail  bool
}

func (s *memorySecrets) Resolve(string) (string, error) { return s.value, nil }
func (s *memorySecrets) Set(_ string, value string) error {
	if s.fail {
		return errors.New("磁盘写入失败")
	}
	s.value = value
	return nil
}

func TestSessions(t *testing.T) {
	secrets := &memorySecrets{}
	s, err := New(secrets)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid("") || s.Enabled() {
		t.Fatal("默认应免登录")
	}
	if err := s.SetPassword("", "八位-password", false); err != nil {
		t.Fatal(err)
	}
	if !s.Enabled() || s.Valid("") {
		t.Fatal("设置密码应生效")
	}
	token, err := s.Login("127.0.0.1", "八位-password")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid(token) || strings.Contains(secrets.value, token) || strings.Contains(secrets.value, "八位-password") {
		t.Fatal("会话未生效或明文泄漏")
	}
	restart, err := New(secrets)
	if err != nil || !restart.Valid(token) {
		t.Fatal("重启后会话应保留", err)
	}
	secrets.fail = true
	if s.RevokeAll() == nil || !s.Valid(token) {
		t.Fatal("落盘失败不能撤销内存会话")
	}
	secrets.fail = false
	changed := s.Changed()
	if err := s.RevokeAll(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	default:
		t.Fatal("没有通知事件流")
	}
	if s.Valid(token) {
		t.Fatal("撤销会话应生效")
	}
	if s.SetPassword("错误密码", "", false) == nil {
		t.Fatal("清除密码必须验证当前密码")
	}
	if err := s.SetPassword("八位-password", "", false); err != nil {
		t.Fatal(err)
	}
	if s.Enabled() || !s.Valid("") {
		t.Fatal("清除密码未生效")
	}
}

func TestLoginThrottleAndExpiry(t *testing.T) {
	s, _ := New(&memorySecrets{})
	if err := s.SetPassword("", "password1", true); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := s.Login("ip", "wrong"); err == nil {
			t.Fatal("错误密码被接受")
		}
	}
	if _, err := s.Login("ip", "password1"); err == nil {
		t.Fatal("节流未生效")
	} else if e, ok := errs.As(err); !ok || e.Code != errs.CodeRateLimited {
		t.Fatal(err)
	}
	token, err := s.Login("another", "password1")
	if err != nil {
		t.Fatal(err)
	}
	s.data.Sessions[hashToken(token)] = time.Now().Add(-time.Second).Unix()
	if s.Valid(token) {
		t.Fatal("过期会话不能访问")
	}
	for _, raw := range []string{`garbage`, `{"hash":"bad","salt":"bad"}`} {
		if _, err := New(&memorySecrets{value: raw}); err == nil {
			t.Fatal("损坏的密码文件不能放行")
		}
	}
}
