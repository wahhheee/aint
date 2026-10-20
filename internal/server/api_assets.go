package server

import (
	"net/http"
	"strings"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
)

// asset 提供作品素材：GET /api/v1/assets/{hash}（《02-技术方案》18.2）。
//
//   - 只提供作品文件引用过的 blob（不能借此读取存档消息等其他内容）；
//   - 内容寻址，可以永久缓存；
//   - TODO(M2-10)：?w= 缩放（派生图缓存在 cache/derived/）；玩家上传的图片（/uploads）。
func (s *Server) asset(w http.ResponseWriter, r *http.Request) error {
	hash := r.PathValue("hash")
	if !strings.HasPrefix(hash, "sha256:") {
		hash = "sha256:" + hash
	}
	data, err := s.d.Works.AssetData(r.Context(), hash)
	if err != nil {
		return err
	}
	if data == nil {
		return errs.NotFound("素材不存在")
	}
	h := w.Header()
	h.Set("Content-Type", http.DetectContentType(data))
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
	return nil
}
