package media

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func pngBytes(t *testing.T, w, h int, alpha bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			a := uint8(255)
			if alpha && x == 0 {
				a = 0
			}
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestProbe(t *testing.T) {
	info, err := Probe(pngBytes(t, 30, 20, false))
	if err != nil || info.Format != "png" || info.Width != 30 || info.Height != 20 || info.MIME != "image/png" {
		t.Fatalf("Probe：%+v %v", info, err)
	}
	if _, err := Probe([]byte("<svg></svg>")); err == nil {
		t.Fatal("SVG 不是支持的图片格式")
	}
}

func TestDerive(t *testing.T) {
	small := pngBytes(t, 40, 30, false)
	d, err := Derive(small, 100)
	if err != nil || !d.Original || !bytes.Equal(d.Data, small) {
		t.Fatalf("尺寸合适时应原样返回：%+v %v", d.MIME, err)
	}

	big := pngBytes(t, 400, 200, false)
	d1, err := Derive(big, 100)
	if err != nil {
		t.Fatal(err)
	}
	if d1.Original || d1.Width != 100 || d1.Height != 50 || d1.MIME != "image/jpeg" {
		t.Fatalf("缩放到长边 100、不透明时编码为 JPEG：%dx%d %s", d1.Width, d1.Height, d1.MIME)
	}
	d2, _ := Derive(big, 100)
	if !bytes.Equal(d1.Data, d2.Data) {
		t.Fatal("派生结果应确定（前缀稳定依赖于此）")
	}

	d3, err := Derive(pngBytes(t, 300, 300, true), 100)
	if err != nil || d3.MIME != "image/png" {
		t.Fatalf("有透明像素时编码为 PNG：%s %v", d3.MIME, err)
	}
}

func TestDeriveGIFFirstFrame(t *testing.T) {
	pal := color.Palette{color.Black, color.White}
	frame := func(c uint8) *image.Paletted {
		img := image.NewPaletted(image.Rect(0, 0, 10, 10), pal)
		for i := range img.Pix {
			img.Pix[i] = c
		}
		return img
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{frame(1), frame(0)}, Delay: []int{10, 10}}); err != nil {
		t.Fatal(err)
	}
	d, err := Derive(buf.Bytes(), 100)
	if err != nil || d.Original || d.MIME != "image/jpeg" {
		t.Fatalf("GIF 应取首帧重新编码：%+v %v", d.MIME, err)
	}
	img, _, err := image.Decode(bytes.NewReader(d.Data))
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(5, 5).RGBA(); r < 0xf000 {
		t.Fatal("应取第一帧（白色）")
	}
}

func TestSanitizeStripsMetadata(t *testing.T) {
	// 一张带 EXIF（APP1）段的 JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	exif := append([]byte{0xFF, 0xE1, 0x00, 0x10}, []byte("Exif\x00\x00GPS-SECRET")...)
	withExif := append(append(append([]byte{}, raw[:2]...), exif...), raw[2:]...)
	if !bytes.Contains(withExif, []byte("GPS-SECRET")) {
		t.Fatal("测试数据应含 EXIF")
	}
	d, err := Sanitize(withExif)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(d.Data, []byte("GPS-SECRET")) || bytes.Contains(d.Data, []byte("Exif")) {
		t.Fatal("重新编码后不应保留 EXIF")
	}
	if _, err := Sanitize([]byte("not an image")); err == nil {
		t.Fatal("非图片应拒绝")
	}
}
