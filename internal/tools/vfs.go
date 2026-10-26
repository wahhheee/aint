package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/vfs"
)

// read / list 的结果格式（《02-技术方案》10.2）：首行"# read 路径（第 a–b 行，共 n 行）"，随后是内容；
// 没有读完时在末尾说明还剩多少、下一次从哪一行接着读。出错时陈述事实并给出可行写法（D-21）。

type readArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func handleRead(ctx context.Context, env Env, raw jsonx.Value) Result {
	var a readArgs
	if err := decodeArgs(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {\"path\": \"/memory/index.md\"}。", err)
	}
	res, err := env.VFS().Read(ctx, a.Path, a.Offset, a.Limit)
	if err != nil {
		return pathFailure("read", err)
	}
	if res.Image != nil {
		head := fmt.Sprintf("# read %s（图片", res.Path)
		if res.Summary != "" {
			head += "：" + res.Summary
		}
		head += "）"
		if !env.Features().Has(prompts.FVision) {
			return Text("%s\n当前模型不能看图，可以依据图片说明来写。", head)
		}
		ref, err := env.ModelImage(ctx, res.Image.Blob)
		if err != nil {
			return NotExecuted("%s 无法读取（%v），可以依据图片说明来写。", res.Path, err)
		}
		return Result{Parts: []llm.Part{llm.Text(head), {Kind: llm.PartImage, Image: &ref}}}
	}
	if res.TotalLines == 0 {
		return Text("# read %s（空文件）", res.Path)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# read %s（第 %d–%d 行，共 %d 行）\n%s", res.Path, res.FromLine, res.ToLine, res.TotalLines, res.Text)
	switch {
	case res.LineCut:
		fmt.Fprintf(&b, "\n（第 %d 行过长，只显示了开头 %d 字）", res.ToLine, vfs.MaxReadRunes)
		if res.ToLine < res.TotalLines {
			fmt.Fprintf(&b, "\n（还有 %d 行，可以用 offset=%d 接着读）", res.TotalLines-res.ToLine, res.ToLine+1)
		}
	case res.ToLine < res.TotalLines:
		fmt.Fprintf(&b, "\n（还有 %d 行，可以用 offset=%d 接着读）", res.TotalLines-res.ToLine, res.ToLine+1)
	}
	return Text("%s", b.String())
}

type listArgs struct {
	Path  string `json:"path"`
	Depth int    `json:"depth"`
}

func handleList(ctx context.Context, env Env, raw jsonx.Value) Result {
	var a listArgs
	if err := decodeArgs(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {\"path\": \"/memory\"}。", err)
	}
	if strings.TrimSpace(a.Path) == "" {
		a.Path = "/"
	}
	entries, err := env.VFS().List(ctx, a.Path, a.Depth)
	if err != nil {
		return pathFailure("list", err)
	}
	shown := strings.TrimRight(strings.TrimSpace(a.Path), "/")
	if shown == "" {
		shown = "/"
	}
	if len(entries) == 0 {
		return Text("# list %s\n（空目录）", shown)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# list %s（共 %d 项）", shown, len(entries))
	for i, e := range entries {
		if i == vfs.MaxListShown {
			fmt.Fprintf(&b, "\n（还有 %d 项未显示，可以列出更具体的子目录）", len(entries)-i)
			break
		}
		b.WriteString("\n" + strings.Repeat("  ", max(e.Depth-1, 0)) + e.Path)
		switch {
		case e.Unavailable:
			b.WriteString("/  （暂不可用）")
		case e.IsDir:
			fmt.Fprintf(&b, "/  （%d 个文件）", e.Files)
		}
		if e.Summary != "" {
			b.WriteString("  " + e.Summary)
		}
	}
	return Text("%s", b.String())
}

// search 的结果格式（10.2）：首行"# search "词"（共 N 处）"，每条一行"路径:行号: 片段"，命中部分用【】标出；
// 结果按 记忆 > 作品资料 > 原文（新→旧）排序，被截断时在末尾说明。

type searchArgs struct {
	Query string `json:"query"`
	Path  string `json:"path"`
	Regex bool   `json:"regex"`
	Limit int    `json:"limit"`
}

func handleSearch(ctx context.Context, env Env, raw jsonx.Value) Result {
	var a searchArgs
	if err := decodeArgs(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {\"query\": \"银色钥匙\"}。", err)
	}
	res, err := env.VFS().Search(ctx, vfs.Query{Text: a.Query, Prefix: a.Path, Regex: a.Regex, Limit: a.Limit})
	if err != nil {
		var qe *vfs.QueryError
		if errors.As(err, &qe) {
			return NotExecuted("%s，可以写成 {\"query\": \"银色钥匙\"}。", qe.Reason)
		}
		return pathFailure("search", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# search %q", res.Query)
	if p := strings.TrimRight(strings.TrimSpace(a.Path), "/"); p != "" {
		fmt.Fprintf(&b, " 于 %s", p)
	}
	if res.Total == 0 {
		b.WriteString("（没有找到）\n可以换个说法、缩短关键词")
		if a.Path != "" {
			b.WriteString("，或去掉路径限定")
		}
		b.WriteString("再试；查不到的内容按不确定来处理。")
		return Text("%s", b.String())
	}
	fmt.Fprintf(&b, "（共 %d 处）", res.Total)
	if len(res.Aliases) > 0 {
		fmt.Fprintf(&b, "\n（同时按别名「%s」检索）", strings.Join(res.Aliases, "」「"))
	}
	for _, h := range res.Hits {
		fmt.Fprintf(&b, "\n%s:%d: %s", h.Path, h.Line, h.Snippet)
	}
	if res.Corpora > 1 {
		b.WriteString("\n（结果已按 记忆 > 作品资料 > 原文（新→旧）排序）")
	}
	if res.Truncated {
		fmt.Fprintf(&b, "\n（结果已截断，只列出 %d 处；每个文件最多列 5 处。可以换更具体的词，或用 path 限定目录，再用 read 查看上下文）", len(res.Hits))
	}
	return Text("%s", b.String())
}

// write / edit / remove 的结果格式：首行"# write 路径"，随后一句陈述发生了什么。
// 写入只进入本回合的记忆覆盖层，随回合提交；回合被丢弃时一并作废。

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func handleWrite(ctx context.Context, env Env, raw jsonx.Value) Result {
	var a writeArgs
	if err := decodeArgs(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {\"path\": \"/memory/index.md\", \"content\": \"……\"}。", err)
	}
	res, err := env.VFS().Write(ctx, a.Path, []byte(a.Content))
	if err != nil {
		return pathFailure("write", err)
	}
	verb := "已覆盖"
	if res.Created {
		verb = "已新建"
	}
	return Text("# write %s\n%s（%d 行）%s", res.Path, verb, res.Lines, sizeHint(res.TotalSize))
}

type editArgs struct {
	Path       string `json:"path"`
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all"`
}

func handleEdit(ctx context.Context, env Env, raw jsonx.Value) Result {
	var a editArgs
	if err := decodeArgs(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {\"path\": \"/memory/index.md\", \"old_text\": \"……\", \"new_text\": \"……\"}。", err)
	}
	if a.OldText == "" {
		return NotExecuted("old_text 是空的。要整体改写文件可以用 write。")
	}
	data, err := env.VFS().ReadFile(ctx, a.Path)
	if err != nil {
		return pathFailure("edit", err)
	}
	text := lf(string(data))
	oldText := lf(a.OldText)
	n := strings.Count(text, oldText)
	switch {
	case n == 0:
		return NotExecuted("%s 中没有找到 old_text。可以先 read 确认原文（留意空格与换行），再照原样写出要替换的部分。", strings.TrimSpace(a.Path))
	case n > 1 && !a.ReplaceAll:
		return NotExecuted("old_text 在 %s 中出现了 %d 次。可以多带一些前后文让它只出现一次，或设 replace_all: true 全部替换。", strings.TrimSpace(a.Path), n)
	}
	if !a.ReplaceAll {
		n = 1
	}
	res, err := env.VFS().Write(ctx, a.Path, []byte(strings.Replace(text, oldText, lf(a.NewText), n)))
	if err != nil {
		return pathFailure("edit", err)
	}
	return Text("# edit %s\n已替换 %d 处（现共 %d 行）%s", res.Path, n, res.Lines, sizeHint(res.TotalSize))
}

// lf 把 CRLF 换行统一为 LF（记忆文件以 LF 存储，模型给出的文字可能带 CRLF）。
func lf(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

type removeArgs struct {
	Path string `json:"path"`
}

func handleRemove(ctx context.Context, env Env, raw jsonx.Value) Result {
	var a removeArgs
	if err := decodeArgs(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {\"path\": \"/memory/old.md\"}。", err)
	}
	if err := env.VFS().Remove(ctx, a.Path); err != nil {
		return pathFailure("remove", err)
	}
	p, _ := vfs.Clean(a.Path)
	return Text("# remove %s\n已删除", p)
}

// sizeHint 在记忆总量超过提醒线时附上一句提醒。
func sizeHint(total int) string {
	if total <= vfs.MemoryWarnTotal {
		return ""
	}
	return fmt.Sprintf("\n（记忆总量约 %s，上限 %s，可以合并精简一些文件）", vfs.FormatSize(total), vfs.FormatSize(vfs.MaxMemoryTotal))
}

// pathFailure 把 VFS 错误写成给模型的说明：发生了什么，以及可以怎么改。
func pathFailure(tool string, err error) Result {
	var pe *vfs.PathError
	if !errors.As(err, &pe) {
		return NotExecuted("%s 出错（%v），可以先依据上下文中已有的信息继续写。", tool, err)
	}
	var b strings.Builder
	switch {
	case errors.Is(pe.Err, vfs.ErrInvalid):
		fmt.Fprintf(&b, "%s：%s。", pe.Path, pe.Reason)
		switch {
		case len(pe.Similar) > 0:
			fmt.Fprintf(&b, "可以写成 %s。", strings.Join(pe.Similar, "、"))
		case strings.Contains(pe.Reason, "路径"): // 路径写法本身的问题：给出写法示例
			b.WriteString("路径以 / 开头，如 /memory/index.md、/work/lore。")
		}
	case errors.Is(pe.Err, vfs.ErrNotFound):
		fmt.Fprintf(&b, "%s 不存在。", pe.Path)
		if len(pe.Similar) > 0 {
			fmt.Fprintf(&b, "相近的路径有 %s。", strings.Join(pe.Similar, "、"))
		}
		switch {
		case len(pe.Roots) > 0:
			fmt.Fprintf(&b, "可以访问的目录有 %s。", strings.Join(pe.Roots, "、"))
		case pe.Parent != "":
			fmt.Fprintf(&b, "可以用 list %s 看看其中有哪些文件。", pe.Parent)
		}
	case errors.Is(pe.Err, vfs.ErrIsDir):
		fmt.Fprintf(&b, "%s 是目录，可以用 list 查看", pe.Path)
		if len(pe.Children) > 0 {
			children := pe.Children
			if len(children) > 12 {
				children = append(children[:12:12], "……")
			}
			fmt.Fprintf(&b, "；其中有 %s", strings.Join(children, "、"))
		}
		b.WriteString("。")
	case errors.Is(pe.Err, vfs.ErrNotDir):
		fmt.Fprintf(&b, "%s 是文件，可以用 read 读取。", pe.Path)
	case errors.Is(pe.Err, vfs.ErrUnavailable):
		fmt.Fprintf(&b, "%s。可以先依据上下文中已有的信息继续写。", pe.Reason)
	case errors.Is(pe.Err, vfs.ErrBinary):
		fmt.Fprintf(&b, "%s %s", pe.Path, strings.TrimPrefix(pe.Reason, pe.Path))
		if pe.Summary != "" {
			fmt.Fprintf(&b, "（%s）", pe.Summary)
		}
		b.WriteString("。")
	case errors.Is(pe.Err, vfs.ErrReadOnly):
		fmt.Fprintf(&b, "%s 是只读的，%s，如 /memory/notes.md。", pe.Path, pe.Reason)
	case errors.Is(pe.Err, vfs.ErrTooLarge):
		fmt.Fprintf(&b, "%s：%s。", pe.Path, pe.Reason)
	case errors.Is(pe.Err, vfs.ErrOutOfRange):
		fmt.Fprintf(&b, "%s 共 %d 行，offset 超出了范围，可以从 offset=1 读起。", pe.Path, pe.Total)
	default:
		b.WriteString(pe.Error())
	}
	return NotExecuted("%s", b.String())
}
