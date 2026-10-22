package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/tmpl"
	"github.com/wahhheee/yanyi/internal/tools"
	"github.com/wahhheee/yanyi/internal/work"
)

func runWork(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法：yanyi work validate|prompt|init|pack <目录>")
	}
	sub, args := args[0], args[1:]
	switch sub {
	case "validate":
		return workValidate(args)
	case "prompt":
		return workPrompt(args)
	case "init":
		return fmt.Errorf("yanyi work init 尚未实现（TODO(M6-3)：生成最小作品模板）")
	case "pack":
		return fmt.Errorf("yanyi work pack 尚未实现（TODO(M6-3)：确定性 zip，文件按路径排序、统一时间戳与权限）")
	}
	return fmt.Errorf("未知子命令：work %s", sub)
}

// loadWorkDir 读取并编译一个作品目录。
func loadWorkDir(dir string) (*work.Model, *work.Report, error) {
	m, rep, _, err := loadWorkFiles(dir)
	return m, rep, err
}

// loadWorkFiles 读取作品目录或 zip 包（与导入相同：根目录就是作品根，或只含一个作品文件夹），并编译。
func loadWorkFiles(dir string) (*work.Model, *work.Report, *work.FileSet, error) {
	set, err := readWorkFiles(dir)
	if e, ok := errs.As(err); ok {
		return nil, nil, nil, errors.New(e.Message) // 终端里只需要说明，不需要错误码
	}
	if err != nil {
		return nil, nil, nil, err
	}
	m, rep := work.Compile(set)
	return m, rep, set, nil
}

func readWorkFiles(p string) (*work.FileSet, error) {
	if !strings.EqualFold(filepath.Ext(p), ".zip") {
		return work.FromFS(os.DirFS(p), work.DefaultLimits)
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // 只读
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return work.FromZip(f, info.Size(), work.DefaultLimits)
}

// workValidate：yanyi work validate <目录或 zip 包> [--json]
func workValidate(args []string) error {
	fs := flag.NewFlagSet("work validate", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "以 JSON 输出校验报告")
	_ = fs.Parse(reorderFlags(args))
	if fs.NArg() != 1 {
		return fmt.Errorf("用法：yanyi work validate <目录或 zip 包> [--json]")
	}
	m, rep, err := loadWorkDir(fs.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		b, _ := jsonx.MarshalIndent(rep)
		fmt.Println(string(b))
	} else {
		fmt.Printf("%s %s（%s）\n", m.Manifest.Title, m.Manifest.Version, m.Manifest.ID)
		fmt.Printf("文件树哈希：%s\n\n", m.TreeHash)
		for _, i := range rep.Issues {
			fmt.Println(i.String())
		}
		e, w := rep.Count()
		fmt.Printf("\n%d 个错误，%d 个警告\n", e, w)
	}
	if rep.HasErrors() {
		os.Exit(1)
	}
	return nil
}

// workPrompt：yanyi work prompt <目录> [--vision] [--helpers] [--json]
// 打印主笔实际收到的系统提示词与工具定义，以及每个片段的启用情况（创作者与平台开发者的调试利器）。
func workPrompt(args []string) error {
	fs := flag.NewFlagSet("work prompt", flag.ExitOnError)
	vision := fs.Bool("vision", false, "模拟支持看图的模型")
	helpers := fs.Bool("helpers", false, "模拟开启后台助手")
	asJSON := fs.Bool("json", false, "以 JSON 输出完整请求（含工具 schema）")
	_ = fs.Parse(reorderFlags(args))
	if fs.NArg() != 1 {
		return fmt.Errorf("用法：yanyi work prompt <目录> [--vision] [--helpers] [--json]")
	}
	m, rep, set, err := loadWorkFiles(fs.Arg(0))
	if err != nil {
		return err
	}
	if rep.HasErrors() {
		for _, i := range rep.Issues {
			fmt.Fprintln(os.Stderr, i.String())
		}
		return fmt.Errorf("作品有错误，请先修正")
	}

	settings := save.Settings{Pace: m.Manifest.Narrator.Length, Reasoning: "low", Helpers: *helpers}
	model := fake.Model()
	model.Caps = llm.Caps{Vision: *vision, Tools: true}
	features := contextx.Profile(m, settings, model.Caps, *helpers)
	env := contextx.Envelope{Input: save.TurnInput{Kind: save.InputOpening}, Text: "请按开场指引开始故事。", Pace: settings.Pace}
	// 初始记忆 = 作品 memory/ 模板（与 save.Create 一致）
	var memory []save.MemoryEntry
	for _, p := range set.Paths() {
		if rest, ok := strings.CutPrefix(p, "memory/"); ok {
			data, _ := set.Get(p)
			memory = append(memory, save.MemoryEntry{Path: "/memory/" + rest, Summary: save.FirstLineSummary(string(data))})
		}
	}
	initState := m.State.Defaults()
	built, err := contextx.NewBuilder(tmpl.New()).Build(contextx.BuildInput{
		SaveID: "preview", Work: m, Features: features, Model: model, Settings: settings,
		InitialState: initState, InitialStateHash: "preview", Epoch: contextx.RenderEpoch0(features, m.State, initState, memory),
		Tools: tools.NewRegistry().Specs(features, m), Turn: env.Message(features),
	})
	if err != nil {
		return err
	}
	if *asJSON {
		b, _ := jsonx.MarshalIndent(built.Request)
		fmt.Println(string(b))
		return nil
	}

	rule := strings.Repeat("─", 60)
	fmt.Printf("%s\n能力画像：%s\n%s\n", rule, features.Signature(), rule)
	for _, b := range built.Request.System {
		fmt.Println(b.Text)
		fmt.Println(rule)
	}
	for i, msg := range built.Request.Messages {
		fmt.Printf("[消息 %d · %s]\n", i+1, msg.Role)
		for _, p := range msg.Parts {
			fmt.Println(p.Text) // 每个内容块单独成段（L3、L4 是同一条消息中的不同块）
		}
		fmt.Println(rule)
	}
	fmt.Println("工具：")
	for _, t := range built.Request.Tools {
		fmt.Printf("  - %s：%s\n", t.Name, t.Description)
	}
	fmt.Printf("%s\n片段：\n", rule)
	for _, f := range built.Fragments {
		mark := "✓"
		if !f.Enabled {
			mark = "·"
		}
		fmt.Printf("  %s %-18s %s %s\n", mark, f.ID, f.Section, f.Reason)
	}
	fmt.Printf("%s\n前缀哈希：%s\n", rule, built.PrefixHash)
	return nil
}

// reorderFlags 让标志可以写在位置参数之后（yanyi work validate ./dir --json）。
func reorderFlags(args []string) []string {
	var flags, rest []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
		} else {
			rest = append(rest, a)
		}
	}
	return append(flags, rest...)
}
