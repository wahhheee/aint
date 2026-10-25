// Package access 管理可选访问密码与设备会话；密钥存储由装配层注入。
package access

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"maps"
	"sync"
	"time"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

const secretKey = "access"
const iterations = 600000
const SessionLifetime = 30 * 24 * time.Hour

type Secrets interface {
	Resolve(string) (string, error)
	Set(string, string) error
}

type record struct {
	Salt     string           `json:"salt"`
	Hash     string           `json:"hash"`
	Sessions map[string]int64 `json:"sessions"` // 只保存随机会话令牌的 SHA-256。
}

type attempts struct {
	count int
	until time.Time
}

type Service struct {
	mu       sync.Mutex
	secrets  Secrets
	data     record
	attempts map[string]attempts
	global   attempts
	changed  chan struct{}
}

func New(secrets Secrets) (*Service, error) {
	s := &Service{secrets: secrets, attempts: map[string]attempts{}, changed: make(chan struct{})}
	raw, err := secrets.Resolve(secretKey)
	if err == nil && raw != "" {
		if err := jsonx.Unmarshal([]byte(raw), &s.data); err != nil {
			return nil, errs.Invalid("访问密码记录损坏，请使用 yanyi password clear 重置")
		}
	}
	if s.data.Hash != "" {
		h, e1 := hex.DecodeString(s.data.Hash)
		salt, e2 := hex.DecodeString(s.data.Salt)
		if e1 != nil || e2 != nil || len(h) != 32 || len(salt) != 16 {
			return nil, errs.Invalid("访问密码记录损坏，请使用 yanyi password clear 重置")
		}
	}
	return s, nil
}

func (s *Service) Enabled() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.data.Hash != "" }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Valid(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Hash == "" {
		return true
	}
	return token != "" && s.data.Sessions[hashToken(token)] > time.Now().Unix()
}

func verify(data record, password string) bool {
	if len(password) > 1024 {
		return false
	}
	salt, err := hex.DecodeString(data.Salt)
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(data.Hash)
	if err != nil || len(want) != 32 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	return err == nil && subtle.ConstantTimeCompare(want, got) == 1
}

func (s *Service) save(data record) error {
	b, err := jsonx.Marshal(data)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(secretKey, string(b)); err != nil {
		return err
	}
	s.data = data
	close(s.changed)
	s.changed = make(chan struct{})
	return nil
}

// Changed 在密码或设备会话变化时关闭，长连接可据此重新核验权限。
func (s *Service) Changed() <-chan struct{} { s.mu.Lock(); defer s.mu.Unlock(); return s.changed }

// Login 同一来源每分钟最多五次，全局最多二十次；来源只使用实际连接地址。
func (s *Service) Login(ip, password string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Hash == "" {
		return "", errs.Invalid("当前没有设置访问密码")
	}
	now := time.Now()
	for key, a := range s.attempts {
		if now.After(a.until) {
			delete(s.attempts, key)
		}
	}
	a := s.attempts[ip]
	if a.until.IsZero() {
		a.until = now.Add(time.Minute)
	}
	if now.After(s.global.until) {
		s.global = attempts{until: now.Add(time.Minute)}
	}
	if a.count >= 5 || s.global.count >= 20 || len(s.attempts) >= 1024 {
		return "", errs.New(errs.CodeRateLimited, "登录尝试过多，请一分钟后重试")
	}
	a.count++
	s.global.count++
	s.attempts[ip] = a
	if !verify(s.data, password) {
		return "", errs.New(errs.CodeUnauthorized, "访问密码不正确")
	}
	data := s.data
	data.Sessions = maps.Clone(data.Sessions)
	if data.Sessions == nil {
		data.Sessions = map[string]int64{}
	}
	for key, expires := range data.Sessions {
		if expires <= now.Unix() {
			delete(data.Sessions, key)
		}
	}
	if len(data.Sessions) >= 256 {
		return "", errs.Conflict("登录设备过多，请先退出其他设备")
	}
	token := rand.Text()
	data.Sessions[hashToken(token)] = now.Add(SessionLifetime).Unix()
	if err := s.save(data); err != nil {
		return "", err
	}
	delete(s.attempts, ip)
	return token, nil
}

func (s *Service) Logout(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := s.data
	data.Sessions = maps.Clone(data.Sessions)
	delete(data.Sessions, hashToken(token))
	return s.save(data)
}

func (s *Service) RevokeAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := s.data
	data.Sessions = nil
	return s.save(data)
}

// SetPassword 修改后退出全部设备；force 仅供停止服务后的本机命令使用。
func (s *Service) SetPassword(current, password string, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !force && s.data.Hash != "" && !verify(s.data, current) {
		return errs.New(errs.CodeUnauthorized, "当前密码不正确")
	}
	data := record{}
	if password != "" {
		if len(password) < 8 || len(password) > 1024 {
			return errs.Invalid("密码长度须在 8–1024 字节之间")
		}
		salt := make([]byte, 16)
		_, _ = rand.Read(salt)
		hash, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
		if err != nil {
			return err
		}
		data.Salt, data.Hash = hex.EncodeToString(salt), hex.EncodeToString(hash)
	}
	return s.save(data)
}
