package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/fsx"
	"github.com/wahhheee/yanyi/internal/pkg/lru"
	"github.com/wahhheee/yanyi/internal/store"
)

// Store 管理图片的派生与玩家上传。并发安全。
//
//   - ForModel：发送给模型的派生图，存为 blob（内容寻址），消息中引用派生图的哈希；
//   - Thumb：界面显示的缩略图（素材接口的 ?w=），缓存在 <数据目录>/cache/derived/，可随时删除；
//   - SaveUpload / Upload：玩家上传的图片（uploads 表登记，回合输入只能引用登记过的图片）。
type Store struct {
	db       *store.DB
	blobs    *store.Blobs
	cacheDir string

	derived *lru.Cache[string, llm.ImageRef] // "原图哈希|长边" → 派生图引用
}

// derivedCacheSize 是派生图引用的内存缓存条数（只缓存引用，不缓存内容）。
const derivedCacheSize = 512

// NewStore 创建图片存储。cacheDir 为空时不缓存缩略图（每次重新生成）。
func NewStore(db *store.DB, blobs *store.Blobs, cacheDir string) *Store {
	return &Store{db: db, blobs: blobs, cacheDir: cacheDir, derived: lru.New[string, llm.ImageRef](derivedCacheSize)}
}

// ClearDerived 在存储回收后丢弃内存中的派生图引用，下次使用时重新生成。
func (s *Store) ClearDerived() { s.derived.Clear() }

// ForModel 返回发送给模型的图片引用：长边不超过 maxEdge 的派生图（尺寸合适时就是原图）。
func (s *Store) ForModel(ctx context.Context, hash string, maxEdge int) (llm.ImageRef, error) {
	if maxEdge <= 0 {
		maxEdge = DefaultMaxEdge
	}
	key := hash + "|" + strconv.Itoa(maxEdge)
	if ref, ok := s.derived.Get(key); ok {
		return ref, nil
	}
	data, err := s.blobs.Get(ctx, s.db.Read(), hash)
	if err != nil {
		return llm.ImageRef{}, err
	}
	d, err := Derive(data, maxEdge)
	if err != nil {
		return llm.ImageRef{}, err
	}
	ref := llm.ImageRef{Blob: hash, MIME: d.MIME, Width: d.Width, Height: d.Height}
	if !d.Original {
		err := s.db.Tx(ctx, func(tx *sql.Tx) error {
			ref.Blob, err = s.blobs.Put(ctx, tx, d.Data, d.MIME)
			return err
		})
		if err != nil {
			return llm.ImageRef{}, fmt.Errorf("保存派生图失败: %w", err)
		}
	}
	s.derived.Put(key, ref)
	return ref, nil
}

// Thumb 返回长边不超过 edge 的缩略图（界面显示用），结果缓存在磁盘上。
func (s *Store) Thumb(hash string, data []byte, edge int) (Derived, error) {
	name := strings.TrimPrefix(hash, "sha256:") + "-" + strconv.Itoa(edge)
	if s.cacheDir != "" {
		for _, ext := range []string{".jpg", ".png"} {
			if b, err := os.ReadFile(filepath.Join(s.cacheDir, name+ext)); err == nil {
				mime := "image/jpeg"
				if ext == ".png" {
					mime = "image/png"
				}
				return Derived{Data: b, MIME: mime}, nil
			}
		}
	}
	d, err := Derive(data, edge)
	if err != nil || s.cacheDir == "" || d.Original {
		return d, err // 原图已足够小时不缓存
	}
	ext := ".jpg"
	if d.MIME == "image/png" {
		ext = ".png"
	}
	if err := os.MkdirAll(s.cacheDir, fsx.DirPerm); err == nil {
		_ = os.WriteFile(filepath.Join(s.cacheDir, name+ext), d.Data, fsx.PrivateFilePerm) // 缓存写失败只影响下次的速度
	}
	return d, nil
}

// Upload 是一张玩家上传的图片（已重新编码）。
type Upload struct {
	Hash   string `json:"hash"`
	MIME   string `json:"mime"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int    `json:"size"`
	URL    string `json:"url"`
}

// UploadURL 返回上传图片的访问地址。
func UploadURL(hash string) string {
	return "/api/v1/uploads/" + strings.TrimPrefix(hash, "sha256:")
}

// Ext 返回图片媒体类型对应的扩展名（/uploads 中的文件名）。
func Ext(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ".jpg"
}

// SaveUpload 校验并保存玩家上传的图片：重新编码后存为 blob，登记到 uploads 表。同一图片重复上传得到同一个哈希。
func (s *Store) SaveUpload(ctx context.Context, data []byte) (*Upload, error) {
	d, err := Sanitize(data)
	if err != nil {
		return nil, errs.Invalid("%v", err)
	}
	u := &Upload{MIME: d.MIME, Width: d.Width, Height: d.Height, Size: len(d.Data)}
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		if u.Hash, err = s.blobs.Put(ctx, tx, d.Data, d.MIME); err != nil {
			return err
		}
		return insertUpload(ctx, tx, u)
	})
	if err != nil {
		return nil, fmt.Errorf("保存上传图片失败: %w", err)
	}
	u.URL = UploadURL(u.Hash)
	return u, nil
}

// Upload 返回登记过的上传图片；不存在时返回 NotFound。
func (s *Store) Upload(ctx context.Context, hash string) (*Upload, error) {
	u, err := getUpload(ctx, s.db.Read(), hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.NotFound("图片不存在")
	}
	if err != nil {
		return nil, err
	}
	u.URL = UploadURL(u.Hash)
	return u, nil
}

// UploadData 返回上传图片的内容。
func (s *Store) UploadData(ctx context.Context, hash string) ([]byte, *Upload, error) {
	u, err := s.Upload(ctx, hash)
	if err != nil {
		return nil, nil, err
	}
	data, err := s.blobs.Get(ctx, s.db.Read(), u.Hash)
	return data, u, err
}
