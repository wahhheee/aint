package server

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/wahhheee/aint/internal/media"
	"github.com/wahhheee/aint/internal/pkg/errs"
)

// asset 提供作品素材：GET /api/v1/assets/{hash}?w=（《02-技术方案》18.2）。
//
//   - 只提供作品文件引用过的 blob（不能借此读取存档消息等其他内容）；
//   - 内容寻址，可以永久缓存；
//   - 作品内容不可信：只按嗅探到的媒体类型（图片、音频、字体）提供，其余一律当作二进制下载，
//     并加 CSP sandbox——伪装成图片的 HTML 不能在本站源下执行脚本；
//   - ?w= 返回长边不超过 w 的缩略图（图片才有，缓存在 cache/derived/），用于书架封面等小图。
func (s *Server) asset(w http.ResponseWriter, r *http.Request) error {
	hash := blobHash(r.PathValue("hash"))
	data, err := s.d.Works.AssetData(r.Context(), hash)
	if err != nil {
		return err
	}
	if data == nil {
		return errs.NotFound("素材不存在")
	}
	return s.serveMedia(w, r, hash, data)
}

// upload 提供玩家上传的图片：GET /api/v1/uploads/{hash}?w=。只提供登记过的上传图片。
func (s *Server) upload(w http.ResponseWriter, r *http.Request) error {
	hash := blobHash(r.PathValue("hash"))
	data, _, err := s.d.Images.UploadData(r.Context(), hash)
	if err != nil {
		return err
	}
	return s.serveMedia(w, r, hash, data)
}

// createUpload 接收玩家上传的图片：POST /api/v1/uploads（multipart，字段 file）。
// 图片重新编码后保存（去除 EXIF 等元数据），返回引用；提交回合时以 attachments 引用它。
func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, media.MaxUploadBytes+64<<10)
	file, _, err := r.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errs.Invalid("图片超过 %d MiB", media.MaxUploadBytes>>20)
		}
		return errs.Invalid("没有收到图片（表单字段 file）")
	}
	defer func() { _ = file.Close() }() // 只读的上传文件，关闭失败无影响
	data, err := io.ReadAll(io.LimitReader(file, media.MaxUploadBytes+1))
	if err != nil {
		return errs.Invalid("读取图片失败")
	}
	u, err := s.d.Images.SaveUpload(r.Context(), data)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, u)
	return nil
}

// serveMedia 以媒体类型提供内容（带不可变缓存头与 CSP sandbox）；?w= 时提供缩略图。
func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request, hash string, data []byte) error {
	contentType := assetContentType(data)
	if edge, _ := strconv.Atoi(r.URL.Query().Get("w")); edge > 0 && strings.HasPrefix(contentType, "image/") {
		d, err := s.d.Images.Thumb(hash, data, min(max(edge, 32), 2048))
		if err == nil {
			data, contentType = d.Data, d.MIME
		}
	}
	h := w.Header()
	h.Set("Content-Type", contentType)
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	// 写失败即客户端已断开，没有可补救的。
	_, _ = w.Write(data) //nolint:gosec // G705：Content-Type 已限定为媒体类型且带 CSP sandbox（assetContentType）
	return nil
}

func blobHash(h string) string {
	if !strings.HasPrefix(h, "sha256:") {
		return "sha256:" + h
	}
	return h
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
