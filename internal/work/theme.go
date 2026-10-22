package work

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Theme 是作品主题（ui/theme.yaml，《03-作品格式规范》12）编译后的结果。
//
// 主题只作用于阅读页（.work-root）：配色、字体、背景与 custom.css 编译成一段 CSS（CSS），
// 排版数值（Reading）单独给前端，由前端与玩家的阅读设置合并（玩家改过的项优先）。
type Theme struct {
	CSS     string       `json:"css,omitzero"`
	Reading ThemeReading `json:"reading"`
}

// ThemeReading 是主题中的排版数值；空值表示沿用平台默认。
type ThemeReading struct {
	Font             string  `json:"font,omitzero"` // serif | sans | 作品字体的 font-family 名
	HeadingFont      string  `json:"heading_font,omitzero"`
	MaxWidth         string  `json:"max_width,omitzero"` // CSS 长度，如 40em
	LineHeight       float64 `json:"line_height,omitzero"`
	ParagraphSpacing string  `json:"paragraph_spacing,omitzero"`
	Indent           string  `json:"indent,omitzero"`
	Backdrop         bool    `json:"backdrop,omitzero"` // 有背景图（.work-backdrop）
}

// themeFile 是 ui/theme.yaml 的结构。
type themeFile struct {
	Fonts struct {
		Body    string `yaml:"body"`
		Heading string `yaml:"heading"`
	} `yaml:"fonts"`
	Colors struct {
		Light themeColors `yaml:"light"`
		Dark  themeColors `yaml:"dark"`
	} `yaml:"colors"`
	Reading struct {
		MaxWidth         string  `yaml:"max_width"`
		LineHeight       float64 `yaml:"line_height"`
		ParagraphSpacing string  `yaml:"paragraph_spacing"`
		Indent           string  `yaml:"indent"`
	} `yaml:"reading"`
	Background struct {
		Image   string   `yaml:"image"`
		Opacity *float64 `yaml:"opacity"`
	} `yaml:"background"`
	CustomCSS string `yaml:"custom_css"`
}

type themeColors struct {
	Background string `yaml:"background"`
	Text       string `yaml:"text"`
	Muted      string `yaml:"muted"`
	Accent     string `yaml:"accent"`
	Panel      string `yaml:"panel"`
}

var (
	reHexColor  = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	reCSSLength = regexp.MustCompile(`^(?:0|\d+(?:\.\d+)?(?:em|rem|px|ch))$`)
)

// theme 编译 ui/theme.yaml 与 custom.css（E-THEME、E-REF）。
func (c *compiler) theme() {
	const file = "ui/theme.yaml"
	data, ok := c.fs.Get(file)
	if !ok {
		return
	}
	var t themeFile
	if !c.decode(file, 0, data, &t, func() any { return &themeFile{} }) {
		return
	}
	th := &c.m.Theme
	var css strings.Builder

	// 字体：serif / sans / 作品字体素材（@font-face）
	font := func(field, ref, family string) string {
		switch ref {
		case "", "serif", "sans":
			return ref
		}
		a, ok := c.m.Asset(ref)
		if !ok || a.Kind != "font" {
			c.r.Errorf(CodeRef, file, 0, "写 serif、sans，或 assets/ 下 woff2 字体的 ID 或路径", "fonts.%s 引用的字体不存在：%s", field, ref)
			return ""
		}
		fmt.Fprintf(&css, "@font-face { font-family: %q; src: url(%q) format(\"woff2\"); font-display: swap; }\n", family, AssetURL(a))
		return family
	}
	th.Reading.Font = font("body", t.Fonts.Body, "work-body")
	th.Reading.HeadingFont = font("heading", t.Fonts.Heading, "work-heading")

	// 配色：浅色写在 .work-root，深色写在 .dark .work-root；只提供一套时另一模式用平台默认
	c.themeColors(&css, file, "light", ".work-root", t.Colors.Light)
	if c.themeColors(&css, file, "dark", ".dark .work-root", t.Colors.Dark) && t.Colors.Dark.Background != "" {
		c.darkBackground = t.Colors.Dark.Background
	}

	// 排版
	r := t.Reading
	for name, v := range map[string]*string{"max_width": &r.MaxWidth, "paragraph_spacing": &r.ParagraphSpacing, "indent": &r.Indent} {
		if *v != "" && !reCSSLength.MatchString(*v) {
			c.r.Errorf(CodeTheme, file, 0, "写成 CSS 长度，如 40em、0.9em、2em；0 表示没有", "reading.%s 不合法：%q", name, *v)
			*v = ""
		}
	}
	if r.LineHeight != 0 && (r.LineHeight < 1.2 || r.LineHeight > 2.6) {
		c.r.Errorf(CodeTheme, file, 0, "行距在 1.2–2.6 之间", "reading.line_height 不合法：%v", r.LineHeight)
		r.LineHeight = 0
	}
	th.Reading.MaxWidth, th.Reading.LineHeight, th.Reading.ParagraphSpacing, th.Reading.Indent = r.MaxWidth, r.LineHeight, r.ParagraphSpacing, r.Indent

	// 背景图：铺在正文后面，透明度保证可读
	if ref := t.Background.Image; ref != "" {
		a, ok := c.m.Asset(ref)
		if !ok || a.Kind != "image" {
			c.r.Errorf(CodeRef, file, 0, "写素材 ID 或 assets/ 下的路径", "background.image 引用的图片不存在：%s", ref)
		} else {
			opacity := 0.12
			if o := t.Background.Opacity; o != nil {
				if *o < 0 || *o > 0.5 {
					c.r.Errorf(CodeTheme, file, 0, "0–0.5 之间；太不透明会影响正文的可读性", "background.opacity 不合法：%v", *o)
				} else {
					opacity = *o
				}
			}
			fmt.Fprintf(&css, ".work-root .work-backdrop { background-image: url(%q); opacity: %s; }\n", AssetURL(a), strconv.FormatFloat(opacity, 'f', -1, 64))
			th.Reading.Backdrop = true
			c.m.markUIOnly(a)
		}
	}

	if p := t.CustomCSS; p != "" {
		src, ok := c.fs.Get(p)
		if !ok {
			c.r.Errorf(CodeRef, file, 0, "", "custom_css 指向的文件不存在：%s", p)
		} else if scoped, issues := ScopeCSS(string(src), c.m.cssAsset); len(issues) > 0 {
			for _, is := range issues {
				c.r.Errorf(CodeTheme, p, is.Line, "custom.css 的作用域限于阅读页，不能引入外部资源", "%s", is.Msg)
			}
		} else {
			css.WriteString(scoped)
		}
	}
	th.CSS = css.String()
}

// themeColors 生成一套配色的 CSS 变量；派生色（凹面、分隔线、淡字）用 color-mix 由主色算出。
// 返回配色是否合法且已生成。
func (c *compiler) themeColors(css *strings.Builder, file, scheme, selector string, tc themeColors) bool {
	fields := []struct{ name, value string }{
		{"background", tc.Background}, {"text", tc.Text}, {"muted", tc.Muted}, {"accent", tc.Accent}, {"panel", tc.Panel},
	}
	set := false
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		if !reHexColor.MatchString(f.value) {
			c.r.Errorf(CodeTheme, file, 0, "颜色写成 #rrggbb", "colors.%s.%s 不合法：%q", scheme, f.name, f.value)
			return false
		}
		set = true
	}
	if !set {
		return false
	}
	var decls []string
	add := func(v, val string) {
		if val != "" {
			decls = append(decls, v+": "+val)
		}
	}
	add("--paper", tc.Background)
	add("--paper-raised", tc.Panel)
	add("--paper-overlay", tc.Panel)
	add("--ink", tc.Text)
	add("--ink-muted", tc.Muted)
	add("--gold", tc.Accent)
	if tc.Background != "" && tc.Text != "" {
		decls = append(decls,
			"--paper-sunken: color-mix(in oklab, var(--paper) 92%, var(--ink))",
			"--rule: color-mix(in oklab, var(--ink) 12%, var(--paper))",
			"--rule-strong: color-mix(in oklab, var(--ink) 20%, var(--paper))",
		)
	}
	if tc.Muted != "" && tc.Background != "" {
		decls = append(decls, "--ink-faint: color-mix(in oklab, var(--ink-muted) 70%, var(--paper))")
	}
	fmt.Fprintf(css, "%s { %s; }\n", selector, strings.Join(decls, "; "))
	return true
}

// cssAsset 把 custom.css 中 url(asset:…) 的素材引用解析为地址。
func (m *Model) cssAsset(ref string) (string, bool) {
	a, ok := m.Asset(ref)
	if !ok {
		return "", false
	}
	return AssetURL(a), true
}
