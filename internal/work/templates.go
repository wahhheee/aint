package work

import (
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/wahhheee/aint/internal/tmpl"
)

// reTemplateRef 找出模板中以字面量写出的 {% render 'x' %} / {% include 'x' %}（检查引用是否存在）。
var reTemplateRef = regexp.MustCompile(`\{%-?\s*(render|include)\s+['"]([^'"]+)['"]`)

// templates 解析作品中的全部 Liquid 模板（E-TEMPLATE），检查 {% render %} 引用（E-REF），并收集 ui/ 下的模板集。
// 模板：执笔指令、开场、命令正文、ui/ 下的 .liquid（面板与被 render 引用的片段）。
func (c *compiler) templates() {
	c.m.Partials = map[string]string{}
	for _, p := range c.fs.Paths() {
		if strings.HasPrefix(p, "ui/") && strings.HasSuffix(p, ".liquid") {
			data, _ := c.fs.Get(p)
			c.m.Partials[p] = string(data)
		}
	}

	if data, ok := c.fs.Get(c.m.Manifest.Narrator.Instructions); ok {
		c.checkTemplate(c.m.Manifest.Narrator.Instructions, string(data), 1)
	}
	opening := c.m.Manifest.Opening.File
	if opening == "" {
		opening = "prompts/opening.md"
	}
	for _, p := range append([]string{opening}, c.fs.Glob("commands", ".md")...) {
		data, ok := c.fs.Get(p)
		if !ok {
			continue
		}
		_, body, line := splitFrontmatter(data)
		c.checkTemplate(p, body, line)
	}
	for _, p := range slices.Sorted(maps.Keys(c.m.Partials)) {
		c.checkTemplate(p, c.m.Partials[p], 1)
	}
}

// checkTemplate 解析一个模板；line 是模板正文在文件中的起始行（frontmatter 之后）。
func (c *compiler) checkTemplate(file, src string, line int) {
	if err := tmpl.Parse(file, src); err != nil {
		var te *tmpl.Error
		msg, at := err.Error(), 0
		if errors.As(err, &te) {
			msg = te.Msg
			if te.Line > 0 {
				at = line + te.Line - 1
			}
		}
		c.r.Errorf(CodeTemplate, file, at, "检查 {% %} 与 {{ }} 是否成对、标签名是否拼写正确", "模板解析失败：%s", firstLine(msg))
		return
	}
	for _, m := range reTemplateRef.FindAllStringSubmatchIndex(src, -1) {
		tag, name := src[m[2]:m[3]], src[m[4]:m[5]]
		at := line + strings.Count(src[:m[0]], "\n")
		if tag == "include" {
			c.r.Errorf(CodeTemplate, file, at, "改用 {% render '"+name+"' %}，被引用的模板只能看到传给它的变量", "不支持 include")
			continue
		}
		if _, ok := tmpl.ResolvePartial(name, func(p string) bool { _, ok := c.m.Partials[p]; return ok }); !ok {
			c.r.Errorf(CodeRef, file, at, "render 只能引用 ui/ 下的 .liquid 文件，如 {% render 'panels/_row' %}", "render 引用的模板不存在：%s", name)
		}
	}
}
