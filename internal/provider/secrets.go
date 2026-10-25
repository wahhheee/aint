package provider

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/wahhheee/yanyi/internal/pkg/fsx"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// Secrets 是 <数据目录>/secrets.json：API Key 与访问密码哈希。永不导出、永不发给前端（不变量 7）。
//
// 单独成文件而不放进数据库，是为了备份或分享数据时可以简单地排除它。
// Unix 下权限 0600；Windows 下依赖用户目录的 ACL（文件位于程序旁的 yanyi-data/ 中）。
type Secrets struct {
	path string
	mu   sync.Mutex
	data map[string]string
}

// OpenSecrets 打开（不存在时视为空）密钥文件。
func OpenSecrets(path string) (*Secrets, error) {
	s := &Secrets{path: path, data: map[string]string{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 secrets.json 失败: %w", err)
	}
	if err := jsonx.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("secrets.json 格式不正确（可备份后删除该文件，再重新填写 API Key）: %w", err)
	}
	return s, nil
}

// Resolve 解析密钥引用：普通键取自 secrets.json；"env:NAME" 取自环境变量（便于 CI 与开发调试）。
func (s *Secrets) Resolve(ref string) (string, error) {
	if name, ok := strings.CutPrefix(ref, "env:"); ok {
		v := os.Getenv(name)
		if v == "" {
			return "", fmt.Errorf("环境变量 %s 未设置", name)
		}
		return v, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[ref]
	if !ok {
		return "", fmt.Errorf("密钥 %s 不存在", ref)
	}
	return v, nil
}

// Set 写入密钥并立即原子落盘。
func (s *Secrets) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.data[key]
	s.data[key] = value
	if err := s.flush(); err != nil {
		if existed {
			s.data[key] = previous
		} else {
			delete(s.data, key)
		}
		return err
	}
	return nil
}

// Delete 删除密钥。
func (s *Secrets) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.data[key]
	delete(s.data, key)
	if err := s.flush(); err != nil {
		if existed {
			s.data[key] = previous
		}
		return err
	}
	return nil
}

func (s *Secrets) flush() error {
	b, err := jsonx.MarshalIndent(s.data)
	if err != nil {
		return err
	}
	return fsx.WriteFileAtomic(s.path, b, 0o600)
}
