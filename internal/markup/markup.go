// Package markup 是叙事标记的服务端实现（《02-技术方案》第 16 节）。
//
// 前端有同一语法的增量解析器（web/src/markup），两端共用 testdata/markup/*.json 测试向量。
// 服务端用途：
//   - 生成纯文本：写入原文全文索引、/transcript 虚拟文件、存档标题输入；
//   - 检查器警告：正文中直接写了"只应由工具产生"的内容等。
//
// 语法：Markdown 子集（段落、*强调*、**加重**、--- 场景分隔、> 引用）+ 白名单标签
// <name attr="值">内容</name> / <name attr="值"/>；未知标签按纯文本显示原文。
package markup

import (
	"regexp"
	"strings"
)

var (
	reOOC      = regexp.MustCompile(`(?s)<ooc>.*?(</ooc>|$)`) // 未闭合时延续到结尾，与前端一致
	reSay      = regexp.MustCompile(`(?s)<say\s+who="([^"]*)"[^>]*>(.*?)</say>`)
	reSelfTag  = regexp.MustCompile(`<[a-z][a-z0-9-]*(\s+[a-z_-]+="[^"]*")*\s*/>`)
	reOpenTag  = regexp.MustCompile(`<[a-z][a-z0-9-]*(\s+[a-z_-]+="[^"]*")*\s*>`)
	reCloseTag = regexp.MustCompile(`</[a-z][a-z0-9-]*>`)
	reEmphasis = regexp.MustCompile(`\*\*?([^*\n]+)\*\*?`)
	reBlank    = regexp.MustCompile(`\n{3,}`)
)

// PlainText 把叙事标记转为故事原文的纯文本：去掉标签，<say who="林舟">台词</say> 转为"林舟：台词"；
// <ooc> 中的场外内容不属于故事，整段去掉（D-30）。
//
// TODO(M0-5/M2-7)：改为基于正式解析器的实现（标签白名单、嵌套、未闭合容错），与前端共用测试向量；
// 目前用正则做近似处理，足以支撑原文检索。
func PlainText(src string) string {
	s := reOOC.ReplaceAllString(src, "")
	s = reSay.ReplaceAllString(s, "$1：$2")
	s = reSelfTag.ReplaceAllString(s, "")
	s = reOpenTag.ReplaceAllString(s, "")
	s = reCloseTag.ReplaceAllString(s, "")
	s = reEmphasis.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "\n---\n", "\n")
	s = reBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
