package work

import (
	"regexp"
	"strings"
)

// reDocLink 匹配设定文档中的 [[id]]；已经写成 [[id]](路径) 的不再补全。
var reDocLink = regexp.MustCompile(`\[\[([a-z0-9][a-z0-9_-]{0,63})\]\](\([^)\n]*\))?`)

// Doc 按 ID 查找设定文档（不含目录摘要 _index.md）。
func (m *Model) Doc(id string) (Doc, bool) {
	for _, d := range m.Docs {
		if d.ID == id && !d.IsIndex {
			return d, true
		}
	}
	return Doc{}, false
}

// LinkDocs 把正文中的 [[id]] 补全为 [[id]](/work/lore/…)，让 AI 能顺着链接直接再读（D-35）。
// 只影响读取结果，作品文件本身不变；找不到的 ID 原样保留（导入时已由校验器报告）。
//
// 调用方：VFS 读取 /work/lore 下的文件时（runtime/vfs.go 的 workSource）。
func (m *Model) LinkDocs(text string) string {
	return reDocLink.ReplaceAllStringFunc(text, func(s string) string {
		sub := reDocLink.FindStringSubmatch(s)
		if sub[2] != "" {
			return s
		}
		if d, ok := m.Doc(sub[1]); ok {
			return s + "(/work/" + d.Path + ")"
		}
		return s
	})
}

// checkDocLinks 检查设定文档中的 [[id]] 是否都指向存在的文档（E-REF）。
func (c *compiler) checkDocLinks() {
	for _, d := range c.m.Docs {
		for i, line := range strings.Split(d.Body, "\n") {
			for _, sub := range reDocLink.FindAllStringSubmatch(line, -1) {
				if _, ok := c.m.Doc(sub[1]); !ok {
					c.r.Errorf(CodeRef, d.Path, d.bodyLine+i, "检查 ID 是否拼写正确；文档 ID 默认取文件名，也可在 frontmatter 中用 id 指定",
						"文档链接 [[%s]] 指向的文档不存在", sub[1])
				}
			}
		}
	}
}
