package work

import (
	"fmt"
	"math"
	"strconv"
)

// 样式标签的深色模式颜色（《03-作品格式规范》10.1）：作品没有写 dark_color 时，在 OKLCH 空间逐步提高亮度
// （保持色相，超出色域时降低彩度），直到与深色背景的对比度达到 4.5（WCAG AA 正文）。

// minContrast 是正文文字与背景的最低对比度。
const minContrast = 4.5

// platformDarkBackground 是平台夜读的底色（D-41：纯黑）；作品主题有深色背景时用主题的。
const platformDarkBackground = "#000000"

// autoDarkColor 返回 color 在深色背景 bg 上可读的版本；本来就够清楚时原样返回。颜色不合法时返回空串。
func autoDarkColor(color, bg string) string {
	fg, ok1 := parseHex(color)
	back, ok2 := parseHex(bg)
	if !ok1 || !ok2 {
		return ""
	}
	if contrast(fg, back) >= minContrast {
		return color
	}
	l, c, h := toOKLCH(fg)
	for l < 1 {
		l = math.Min(1, l+0.02)
		rgb := fromOKLCHInGamut(l, c, h)
		if contrast(rgb, back) >= minContrast {
			return toHex(rgb)
		}
	}
	return "#ffffff"
}

type rgb [3]float64 // sRGB，0–1

func parseHex(s string) (rgb, bool) {
	if !reHexColor.MatchString(s) {
		return rgb{}, false
	}
	h := s[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	var out rgb
	for i := range 3 {
		v, err := strconv.ParseUint(h[i*2:i*2+2], 16, 8)
		if err != nil {
			return rgb{}, false
		}
		out[i] = float64(v) / 255
	}
	return out, true
}

func toHex(c rgb) string {
	b := func(v float64) int { return int(math.Round(math.Max(0, math.Min(1, v)) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", b(c[0]), b(c[1]), b(c[2]))
}

func toLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func fromLinear(v float64) float64 {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

// luminance 是 WCAG 的相对亮度。
func luminance(c rgb) float64 {
	return 0.2126*toLinear(c[0]) + 0.7152*toLinear(c[1]) + 0.0722*toLinear(c[2])
}

func contrast(a, b rgb) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// toOKLCH 把 sRGB 转为 OKLCH（https://bottosson.github.io/posts/oklab/）。
func toOKLCH(c rgb) (l, ch, h float64) {
	r, g, b := toLinear(c[0]), toLinear(c[1]), toLinear(c[2])
	lm := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	mm := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	sm := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*lm + 0.7936177850*mm - 0.0040720468*sm
	A := 1.9779984951*lm - 2.4285922050*mm + 0.4505937099*sm
	B := 0.0259040371*lm + 0.7827717662*mm - 0.8086757660*sm
	return L, math.Hypot(A, B), math.Atan2(B, A)
}

func fromOKLCH(l, ch, h float64) rgb {
	A, B := ch*math.Cos(h), ch*math.Sin(h)
	lm := l + 0.3963377774*A + 0.2158037573*B
	mm := l - 0.1055613458*A - 0.0638541728*B
	sm := l - 0.0894841775*A - 1.2914855480*B
	lm, mm, sm = lm*lm*lm, mm*mm*mm, sm*sm*sm
	return rgb{
		fromLinear(4.0767416621*lm - 3.3077115913*mm + 0.2309699292*sm),
		fromLinear(-1.2684380046*lm + 2.6097574011*mm - 0.3413193965*sm),
		fromLinear(-0.0041960863*lm - 0.7034186147*mm + 1.7076147010*sm),
	}
}

// fromOKLCHInGamut 转回 sRGB；超出色域时降低彩度（保持亮度与色相）。
func fromOKLCHInGamut(l, ch, h float64) rgb {
	for range 40 {
		c := fromOKLCH(l, ch, h)
		if c[0] >= -0.001 && c[0] <= 1.001 && c[1] >= -0.001 && c[1] <= 1.001 && c[2] >= -0.001 && c[2] <= 1.001 {
			return c
		}
		ch *= 0.9
	}
	return fromOKLCH(l, 0, h)
}
