// Package markup 是叙事标记的服务端实现（《02-技术方案》第 16 节）。
//
// 前端有同一语法的增量解析器（web/src/markup/parse.ts），两端运行同一份测试向量
// testdata/markup/*.json（格式见 testdata/README.md），保证产出相同的 AST 与纯文本。
// 改语法时三处一起改：本包、parse.ts、测试向量。
//
// 服务端用途：生成故事原文的纯文本（原文索引、/transcript、存档标题输入）；
// TODO(M5-9)：检查器警告（正文中直接写了"只应由工具产生"的内容等）。
//
// # 语法
//
// 分段：每个非空行是一个段落（D-26），行首行尾的空白（含全角空格）去掉；整行只有 3 个以上的
// "-" 或 "*"（中间可有空白）是场景分隔；以 ">" 开头的行是引用。
//
// 行内：*强调*、**加重**；白名单标签 <name attr="值">内容</name> 与自闭合 <name attr="值"/>。
// 反斜杠转义 \\ \* \< \> \- 得到字符本身（"\---"、"\>" 可以写出不是分隔、不是引用的行）。
//
// 白名单 = 已启用的内置标签 + 作品样式标签（Options.Tags）；白名单外的标签按原文显示（raw），
// 未启用的内置标签同样如此，不另设特殊逻辑（D-21）。
//
// # 容错（模型输出不可控，任何输入都要有确定的结果）
//
//   - 标签：名称 [a-z][a-z0-9-]*，名称后到第一个引号外的 ">" 为止都属于这个标签（标签不跨行）；
//     其中只认 name="值" 形式的属性（名称为字母或下划线开头的 [A-Za-z0-9_-]），其余（单引号、无引号的值、
//     不合法的名称、杂字）忽略；同名属性取第一个；值原样保留，不解码实体与转义。
//   - scene、cg、bgm、sfx、pause 是空元素：不写 "/" 也按自闭合处理，它们的结束标签忽略。
//   - 行内元素在段落结束时自动闭合；结束标签闭合最近的同名元素及其内部尚未闭合的元素；
//     找不到对应开始标签的结束标签忽略；嵌套超过 4 层时忽略更深的开始标签（内容照常显示）。
//   - 强调：开始符后紧跟非空白字符且本行后面有结束符才成立，结束符前须为非空白字符；
//     只闭合栈顶的强调；不成立的星号按原文显示。
//   - 段首的 <scene/>、<cg/> 提升为块级；整段只有一个 <aside> 时按旁白段落排版。
//   - <ooc>（D-30）：行首的 <ooc> 开始一个可以跨行的场外块，到第一个未转义的 </ooc> 为止（流式中尚未写到
//     </ooc> 时到文末为止）；块内按正文规则解析，不再识别 <ooc>；</ooc> 之后的文字按新的一行解析；
//     空的场外块不显示。不在行首的 <ooc> 是普通的行内标签。
//   - 草稿态（Options.Drafting，流式生成中）只影响最后一行：段尾未写完的标签、"*" 与 "\" 先不显示，
//     后面还没有结束符的强调先按成立显示。
package markup

import "strings"

// NodeType 是 AST 节点的类型，取值与前端 parse.ts 的 type 字段一致。
type NodeType string

// 块级节点。
const (
	NodeP       NodeType = "p"       // 段落：Children 为行内节点
	NodeQuote   NodeType = "quote"   // 引用：Children 为行内节点
	NodeAside   NodeType = "aside"   // 整段旁白：Children 为行内节点
	NodeDivider NodeType = "divider" // 场景分隔
	NodeScene   NodeType = "scene"   // 段首 <scene/>：Attrs
	NodeCG      NodeType = "cg"      // 段首 <cg/>：Attrs
	NodeOOC     NodeType = "ooc"     // 场外块（D-30）：Children 为块级节点
)

// 行内节点。
const (
	NodeText   NodeType = "text"   // 文字：Text
	NodeRaw    NodeType = "raw"    // 按原文显示的标签：Text
	NodeEm     NodeType = "em"     // 强调：Children
	NodeStrong NodeType = "strong" // 加重：Children
	NodeTag    NodeType = "tag"    // 白名单标签：Name、Attrs、Children
)

// Node 是 AST 节点（块级与行内共用一个结构，按 Type 取用字段）。
type Node struct {
	Type     NodeType          `json:"type"`
	Text     string            `json:"text,omitzero"`
	Name     string            `json:"name,omitzero"`
	Attrs    map[string]string `json:"attrs,omitzero"`
	Children []Node            `json:"children,omitzero"`
}

// Options 是解析选项。
type Options struct {
	// Tags 是可渲染的标签白名单：已启用的内置标签 + 作品样式标签。
	Tags map[string]bool
	// AllTags 把所有名称合法的标签都当作已启用（服务端纯文本化：标签都不是故事内容）。
	AllTags bool
	// Drafting 表示流式草稿态：隐藏最后一行末尾尚未写完的标记。
	Drafting bool

	inOOC bool // 场外块内部：不再识别 <ooc>
}

func (o *Options) has(name string) bool {
	if o.inOOC && name == "ooc" {
		return false
	}
	return o.AllTags || o.Tags[name]
}

// PlainText 把叙事标记转为故事原文的纯文本（原文索引、/transcript），规则见 Plain。
// 所有名称合法的标签都按已启用处理：模型写出的标签不论是否启用都不是故事内容。
func PlainText(src string) string { return PlainTextWith(src, nil) }

// PlainTextWith 同 PlainText；name 非空时用它把 <say who> 的值（可能是人物 ID）换成人物名。
func PlainTextWith(src string, name func(who string) string) string {
	return plain(Parse(src, Options{AllTags: true}), name)
}

// Plain 取 AST 的纯文本，与前端 plainText 一致：每个块一行；场景分隔为"◇"；段首场景、插图取标题、
// 说明文字；<say who="林舟">台词</say> 为"林舟：台词"；场外内容（<ooc> 块与行内 <ooc>）不属于故事，
// 整段去掉（D-30）；按原文显示的标签保留原文；其余标签只取其中的文字。
func Plain(blocks []Node) string { return plain(blocks, nil) }

func plain(blocks []Node, name func(string) string) string {
	lines := make([]string, 0, len(blocks))
	for _, b := range blocks {
		var s string
		switch b.Type {
		case NodeDivider:
			s = "◇"
		case NodeScene:
			s = b.Attrs["title"]
		case NodeCG:
			s = b.Attrs["caption"]
		case NodeOOC:
			continue
		default:
			var sb strings.Builder
			writeInline(&sb, b.Children, name)
			s = sb.String()
		}
		if s != "" {
			lines = append(lines, s)
		}
	}
	return strings.Join(lines, "\n")
}

func writeInline(sb *strings.Builder, nodes []Node, name func(string) string) {
	for _, n := range nodes {
		switch n.Type {
		case NodeText, NodeRaw:
			sb.WriteString(n.Text)
		case NodeTag:
			switch n.Name {
			case "ooc":
				continue
			case "say":
				if who := n.Attrs["who"]; who != "" {
					if name != nil {
						who = name(who)
					}
					sb.WriteString(who)
					sb.WriteString("：")
				}
			}
			writeInline(sb, n.Children, name)
		default:
			writeInline(sb, n.Children, name)
		}
	}
}
