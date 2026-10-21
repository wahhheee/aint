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
//   - 作品内容不可信：只按嗅探到的媒体类型（图片、音频、字体）提供，其余一律当作二进制下载，
//     并加 CSP sandbox——伪装成图片的 HTML 不能在本站源下执行脚本；
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
	h.Set("Content-Type", assetContentType(data))
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	// 写失败即客户端已断开，没有可补救的。
	_, _ = w.Write(data) //nolint:gosec // G705：Content-Type 已限定为媒体类型且带 CSP sandbox（assetContentType）
	return nil
}

// assetContentType 嗅探素材的媒体类型；不是图片、音频、字体的内容按二进制提供（不会被浏览器渲染）。
// http.DetectContentType 不会把 SVG 判为图片（它可以带脚本），所以 SVG 也按二进制提供。
func assetContentType(data []byte) string {
	ct := http.DetectContentType(data)
	for _, prefix := range []string{"image/", "audio/", "video/", "font/", "application/ogg"} {
		if strings.HasPrefix(ct, prefix) {
			return ct
		}
	}
	return "application/octet-stream"
}
