package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wahhheee/aint/internal/pkg/fsx"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// InlineMax 是内联存储的上限：不超过它的 blob 直接存在 blobs.inline 列中。
const InlineMax = 64 << 10

// ErrBlobNotFound 表示 blob 不存在。
var ErrBlobNotFound = errors.New("blob 不存在")

// Blobs 是内容寻址存储：同内容只存一份，以 'sha256:<hex>' 引用。
//
// 用途：作品文件、状态快照、记忆清单与记忆文件、LLM 消息、显示项、阶段块、上传图片。
// 写入顺序（崩溃安全）：先写文件 blob（原子写），再在事务中插入记录；
// 崩溃最多留下未被引用的 blob，由引用标记 GC 回收（见 gc.go）。
type Blobs struct {
	dir string // <数据目录>/blobs
}

// NewBlobs 创建 blob 存储，dir 为大文件存放目录。
func NewBlobs(dir string) *Blobs {
	return &Blobs{dir: dir}
}

// HashBytes 计算内容哈希，格式 'sha256:<hex>'。
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Put 存入 data 并返回哈希；已存在则直接返回。q 可以是事务，以便与业务写入一起提交。
func (s *Blobs) Put(ctx context.Context, q Querier, data []byte, mime string) (string, error) {
	hash := HashBytes(data)
	var inline []byte
	if len(data) <= InlineMax {
		inline = data
		if inline == nil {
			inline = []byte{} // 空内容也内联，避免与"落文件"混淆
		}
	} else if err := s.writeFile(hash, data); err != nil {
		return "", err
	}
	_, err := q.ExecContext(ctx,
		`INSERT OR IGNORE INTO blobs (hash, size, mime, inline, created_at) VALUES (?, ?, ?, ?, ?)`,
		hash, len(data), nullString(mime), inline, NowMillis())
	if err != nil {
		return "", fmt.Errorf("写入 blob 记录失败: %w", err)
	}
	return hash, nil
}

// PutJSON 以确定性 JSON 序列化 v 后存入。相同内容得到相同哈希（前缀稳定、去重都依赖于此）。
func (s *Blobs) PutJSON(ctx context.Context, q Querier, v any) (string, error) {
	b, err := jsonx.Marshal(v)
	if err != nil {
		return "", err
	}
	return s.Put(ctx, q, b, "application/json")
}

// Get 读取 blob 内容。
func (s *Blobs) Get(ctx context.Context, q Querier, hash string) ([]byte, error) {
	var inline []byte
	var size int64
	err := q.QueryRowContext(ctx, `SELECT inline, size FROM blobs WHERE hash = ?`, hash).Scan(&inline, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrBlobNotFound, hash)
	}
	if err != nil {
		return nil, err
	}
	if inline != nil {
		return inline, nil
	}
	data, err := os.ReadFile(s.filePath(hash))
	if err != nil {
		return nil, fmt.Errorf("读取 blob 文件失败（%s）: %w", hash, err)
	}
	return data, nil
}

// GetJSON 读取并反序列化 JSON blob。
func (s *Blobs) GetJSON(ctx context.Context, q Querier, hash string, v any) error {
	b, err := s.Get(ctx, q, hash)
	if err != nil {
		return err
	}
	if err := jsonx.Unmarshal(b, v); err != nil {
		return fmt.Errorf("解析 blob（%s）失败: %w", hash, err)
	}
	return nil
}

func (s *Blobs) writeFile(hash string, data []byte) error {
	p := s.filePath(hash)
	if _, err := os.Stat(p); err == nil {
		return nil // 内容寻址：同哈希即同内容
	}
	if err := fsx.WriteFileAtomic(p, data, 0o644); err != nil {
		return fmt.Errorf("写入 blob 文件失败: %w", err)
	}
	return nil
}

// filePath：sha256:abcdef… → <dir>/ab/cd/abcdef…
func (s *Blobs) filePath(hash string) string {
	h := strings.TrimPrefix(hash, "sha256:")
	return filepath.Join(s.dir, h[0:2], h[2:4], h)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
