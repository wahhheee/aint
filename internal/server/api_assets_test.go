package server

import "testing"

func TestAssetContentType(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"图片按原类型提供", png, "image/png"},
		{"伪装成图片的 HTML 按二进制提供", []byte("<!DOCTYPE html><script>alert(1)</script>"), "application/octet-stream"},
		{"SVG 可以带脚本，按二进制提供", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), "application/octet-stream"},
		{"纯文本按二进制提供", []byte("hello"), "application/octet-stream"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := assetContentType(c.data); got != c.want {
				t.Fatalf("得到 %s，应为 %s", got, c.want)
			}
		})
	}
}
