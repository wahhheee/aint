package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/vfs"
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
		if len(pe.Similar) > 0 {
			fmt.Fprintf(&b, "可以写成 %s。", strings.Join(pe.Similar, "、"))
		} else {
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
	case errors.Is(pe.Err, vfs.ErrOutOfRange):
		fmt.Fprintf(&b, "%s 共 %d 行，offset 超出了范围，可以从 offset=1 读起。", pe.Path, pe.Total)
	default:
		b.WriteString(pe.Error())
	}
	return NotExecuted("%s", b.String())
}
