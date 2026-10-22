// Package media 处理图片：解码校验、缩放派生、元数据清除（《02-技术方案》第 15 节）。
//
//   - 作品素材：导入时解码校验尺寸与格式（Probe）；发送给模型前按 Caps.ImageMaxEdge（默认长边 1568px）缩放（Derive）；
//   - 派生图的编码是确定的（同一原图、同一边长总是得到同样的字节），以 blob 存储，消息中引用派生图的哈希：
//     历史消息每次发送的字节一致（前缀稳定）；
//   - 玩家上传：类型嗅探（png/jpeg/webp/gif 首帧）、≤ 20 MiB、长边 ≤ 8192；重新编码以去除 EXIF 等元数据（Sanitize）；
//   - 只用标准库 image/* 与 golang.org/x/image（webp 解码、draw 缩放），不引入 CGO。
//
// AI 生图（P2）预留：显示项 image 已含 source / status 字段。
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // 注册 gif 解码（取首帧）
	"image/jpeg"
	"image/png"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // 注册 webp 解码
)

// DefaultMaxEdge 是发送给模型前缩放的默认长边。
const DefaultMaxEdge = 1568

// 上传限制。
const (
	MaxUploadBytes = 20 << 20
	MaxUploadEdge  = 8192
	// maxPixels 是解码前检查的像素上限（防止尺寸声明很大的小文件在解码时占用大量内存）。
	maxPixels = MaxUploadEdge * MaxUploadEdge
)

// Info 是图片的格式与尺寸。
type Info struct {
	Format string `json:"format"` // png | jpeg | gif | webp
	MIME   string `json:"mime"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// LongEdge 返回长边。
func (i Info) LongEdge() int { return max(i.Width, i.Height) }

// ErrNotImage 表示内容不是支持的图片格式。
var ErrNotImage = errors.New("不是支持的图片格式（png、jpg、webp、gif）")

var mimes = map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}

// Probe 读取图片的格式与尺寸（只解码文件头）。
func Probe(data []byte) (Info, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Info{}, ErrNotImage
	}
	mime, ok := mimes[format]
	if !ok {
		return Info{}, ErrNotImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return Info{}, fmt.Errorf("图片尺寸不对（%d×%d）", cfg.Width, cfg.Height)
	}
	return Info{Format: format, MIME: mime, Width: cfg.Width, Height: cfg.Height}, nil
}

// decode 解码图片（GIF 取首帧），先检查像素数。
func decode(data []byte) (image.Image, Info, error) {
	info, err := Probe(data)
	if err != nil {
		return nil, info, err
	}
	if info.Width*info.Height > maxPixels {
		return nil, info, fmt.Errorf("图片太大（%d×%d）", info.Width, info.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data)) // GIF 的 image.Decode 只返回首帧
	if err != nil {
		return nil, info, fmt.Errorf("图片无法解码：%w", err)
	}
	return img, info, nil
}

// Derived 是派生图：发送给模型或界面显示的图片。
type Derived struct {
	Data     []byte
	MIME     string
	Width    int
	Height   int
	Original bool // 原图尺寸已合适，Data 就是原图
}

// Derive 返回长边不超过 maxEdge 的派生图（maxEdge ≤ 0 时取 DefaultMaxEdge）。
// png / jpeg / webp 尺寸已合适时原样返回；GIF 取首帧；需要缩放时按是否透明编码为 PNG 或 JPEG。
// 结果只取决于原图与 maxEdge（编码参数固定），同一输入总是得到同样的字节。
func Derive(data []byte, maxEdge int) (Derived, error) {
	if maxEdge <= 0 {
		maxEdge = DefaultMaxEdge
	}
	info, err := Probe(data)
	if err != nil {
		return Derived{}, err
	}
	if info.Format != "gif" && info.LongEdge() <= maxEdge {
		return Derived{Data: data, MIME: info.MIME, Width: info.Width, Height: info.Height, Original: true}, nil
	}
	img, _, err := decode(data)
	if err != nil {
		return Derived{}, err
	}
	return encode(resize(img, maxEdge), 85)
}

// Sanitize 处理玩家上传的图片：检查大小与尺寸，取首帧，重新编码（去除 EXIF 等元数据与 GIF 的其余帧）。
func Sanitize(data []byte) (Derived, error) {
	if len(data) > MaxUploadBytes {
		return Derived{}, fmt.Errorf("图片超过 %d MiB", MaxUploadBytes>>20)
	}
	info, err := Probe(data)
	if err != nil {
		return Derived{}, err
	}
	if info.LongEdge() > MaxUploadEdge {
		return Derived{}, fmt.Errorf("图片尺寸过大（%d×%d），长边不能超过 %d", info.Width, info.Height, MaxUploadEdge)
	}
	img, _, err := decode(data)
	if err != nil {
		return Derived{}, err
	}
	return encode(img, 90)
}

// resize 等比缩放到长边 maxEdge（已经足够小时原样返回）。
func resize(img image.Image, maxEdge int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if max(w, h) <= maxEdge {
		return img
	}
	if w >= h {
		h = max(1, h*maxEdge/w)
		w = maxEdge
	} else {
		w = max(1, w*maxEdge/h)
		h = maxEdge
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// encode 编码图片：有透明像素时用 PNG，否则用 JPEG（quality）。
func encode(img image.Image, quality int) (Derived, error) {
	var buf bytes.Buffer
	b := img.Bounds()
	out := Derived{Width: b.Dx(), Height: b.Dy()}
	if hasAlpha(img) {
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, img); err != nil {
			return Derived{}, err
		}
		out.MIME = "image/png"
	} else {
		if err := jpeg.Encode(&buf, flatten(img), &jpeg.Options{Quality: quality}); err != nil {
			return Derived{}, err
		}
		out.MIME = "image/jpeg"
	}
	out.Data = buf.Bytes()
	return out, nil
}

// hasAlpha 判断图片是否有不透明度小于 1 的像素。
func hasAlpha(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return true
			}
		}
	}
	return false
}

// flatten 把调色板等图片转为 RGBA（JPEG 编码器对调色板图片的处理依赖实现细节，统一后结果确定）。
func flatten(img image.Image) image.Image {
	switch img.(type) {
	case *image.YCbCr, *image.RGBA, *image.NRGBA, *image.Gray:
		return img
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, b, img, b.Min, draw.Over)
	return dst
}
