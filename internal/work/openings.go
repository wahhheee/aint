package work

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/wahhheee/aint/internal/state"
)

// OpeningSummary 是开场选择列表，不包含正文、指引或初始状态。
type OpeningSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitzero"`
	Mode        string `json:"mode"`
}

// OpeningSummaries 保留作者声明的顺序。
func (m Manifest) OpeningSummaries() []OpeningSummary {
	out := make([]OpeningSummary, 0, len(m.Openings))
	for _, op := range m.Openings {
		out = append(out, OpeningSummary{ID: op.ID, Title: op.Title, Description: op.Description, Mode: op.Mode})
	}
	return out
}

// Opening 按稳定 ID 查找开场；空 ID 选择作者排列的第一项。
func (m *Model) Opening(id string) (*Opening, error) {
	for i := range m.Openings {
		if id == "" || m.Openings[i].ID == id {
			return &m.Openings[i], nil
		}
	}
	return nil, fmt.Errorf("开场 %q 不存在，请重新选择", id)
}

// State 合成默认值与开场覆盖，不修改作品模型。
func (op *Opening) State(schema *state.Schema) (map[string]any, error) {
	st := schema.Defaults()
	keys := slices.Sorted(maps.Keys(op.InitialState))
	var ops []state.Op
	for i, key := range keys {
		for _, previous := range keys[:i] {
			if strings.HasPrefix(key, previous+".") {
				return nil, fmt.Errorf("initial_state 的路径 %s 与 %s 重叠，请分别填写需要覆盖的字段", previous, key)
			}
		}
		ops = append(ops, state.Op{Op: state.OpSet, Path: key, Value: op.InitialState[key]})
	}
	if len(ops) == 0 {
		return st, nil
	}
	next, _, err := state.Apply(schema, st, ops)
	return next, err
}

// openingFront 是开场文件的 frontmatter。
type openingFront struct {
	Choices []Choice `yaml:"choices"`
}

func (c *compiler) openings() {
	if len(c.m.Manifest.Openings) == 0 {
		c.r.Errorf(CodeManifest, "work.yaml", 0, "在 openings 中声明至少一个开场", "缺少开场列表")
	}
	seen := map[string]bool{}
	for _, conf := range c.m.Manifest.Openings {
		op := Opening{OpeningConf: conf}
		if !reDocID.MatchString(op.ID) || seen[op.ID] {
			c.r.Errorf(CodeManifest, "work.yaml", 0, "开场 ID 使用小写字母、数字、下划线或连字符，1–64 字符，且不可重复", "开场 ID 不合法或重复：%q", op.ID)
		}
		seen[op.ID] = true
		if strings.TrimSpace(op.Title) == "" {
			c.r.Errorf(CodeManifest, "work.yaml", 0, "", "开场 %q 缺少 title", op.ID)
		}
		if op.Mode != "static" && op.Mode != "generate" {
			c.r.Errorf(CodeManifest, "work.yaml", 0, "mode 应为 static 或 generate", "开场 %q 的 mode 不合法：%q", op.ID, op.Mode)
		}
		if op.File == "" && op.Mode == "static" {
			c.r.Errorf(CodeRef, "work.yaml", 0, "", "静态开场 %q 必须指定 file", op.ID)
		}
		if op.File != "" {
			if b, ok := c.fs.Get(op.File); ok {
				front, body, _ := splitFrontmatter(b)
				op.Body = strings.TrimSpace(body)
				var fm openingFront
				if front != nil && c.decode(op.File, 1, front, &fm, func() any { return &openingFront{} }) {
					op.Choices = fm.Choices
				}
				choices := map[string]bool{}
				for _, choice := range op.Choices {
					if strings.TrimSpace(choice.ID) == "" || strings.TrimSpace(choice.Label) == "" || choices[choice.ID] {
						c.r.Errorf(CodeManifest, op.File, 0, "选项需要唯一 ID 和非空 label", "开场选项不合法：%q", choice.ID)
					}
					choices[choice.ID] = true
				}
				if op.Mode == "generate" && len(op.Choices) > 0 {
					c.r.Errorf(CodeManifest, op.File, 0, "生成式开场由模型提供选项", "generate 开场不能声明 choices")
				}
			} else {
				c.r.Errorf(CodeRef, "work.yaml", 0, "", "开场 %q 的文件不存在：%s", op.ID, op.File)
			}
		}
		if _, err := op.State(c.m.State); err != nil {
			c.r.Errorf(CodeManifest, "work.yaml", 0, "检查状态定义与覆盖值的类型", "开场 %q：%v", op.ID, err)
		}
		c.m.Openings = append(c.m.Openings, op)
	}
}

// 选项命令的拆分规则与阅读页一致（web/src/features/reader/commands.ts 的 parseInput、splitArgs），
// 空白按 JavaScript 的 \s 计（含全角空格）。
var (
	reChoiceCommand = regexp.MustCompile(`^/([^\s\v\p{Z}\x{FEFF}]+)[\s\v\p{Z}\x{FEFF}]*([\s\S]*)$`)
	reChoiceArg     = regexp.MustCompile(`([\w-]+=)?(?:"([^"]*)"|'([^']*)'|([^\s\v\p{Z}\x{FEFF}]+))`)
	reChoiceKV      = regexp.MustCompile(`^([\w-]+)=(.*)$`)
)

// openingCommands 检查静态开场选项的 command 能否执行。阅读页解析不了的命令会被当作普通选项提交，
// 所以在校验时报告；放在命令编译之后，以便查找作品命令。
func (c *compiler) openingCommands() {
	for _, op := range c.m.Openings {
		for _, choice := range op.Choices {
			if choice.Command == "" {
				continue
			}
			bad := func(code, hint, format string, args ...any) {
				c.r.Errorf(code, op.File, 0, hint, "选项 %q 的 command：%s", choice.ID, fmt.Sprintf(format, args...))
			}
			sub := reChoiceCommand.FindStringSubmatch(strings.TrimSpace(choice.Command))
			if sub == nil {
				bad(CodeManifest, `写成 "/命令 参数"，如 "/follow 林舟"`, "格式不对：%q", choice.Command)
				continue
			}
			name, rest := sub[1], sub[2]
			switch name {
			case "ooc":
				if !c.m.Manifest.Features.OOC {
					bad(CodeManifest, "在 work.yaml 的 features 中开启 ooc", "作品没有开启场外对话，不能使用 /ooc")
				} else if strings.TrimSpace(rest) == "" {
					bad(CodeManifest, "", "/ooc 后需要写场外对话的内容")
				}
				continue
			case "continue":
				continue
			}
			if slices.Contains(BuiltinCommands, name) {
				bad(CodeManifest, "选项可以执行作品命令、/ooc 或 /continue", "不能执行 /%s", name)
				continue
			}
			i := slices.IndexFunc(c.m.Commands, func(cmd Command) bool { return cmd.Name == name })
			if i < 0 {
				bad(CodeRef, "", "命令 /%s 不存在", name)
				continue
			}
			cmd := c.m.Commands[i]
			if !cmd.HasPresent("slash") && !cmd.HasPresent("button") {
				bad(CodeManifest, "在命令的 present 中加入 slash 或 button", "命令 /%s 不能从选项执行", name)
				continue
			}
			args := map[string]string{}
			var positional []string
			for _, m := range reChoiceArg.FindAllStringSubmatch(rest, -1) {
				tok := m[1] + m[2] + m[3] + m[4]
				kv := reChoiceKV.FindStringSubmatch(tok)
				if kv != nil && slices.ContainsFunc(cmd.Args, func(a CommandArg) bool { return a.Name == kv[1] }) {
					args[kv[1]] = kv[2]
				} else {
					positional = append(positional, tok)
				}
			}
			for _, a := range cmd.Args {
				if _, ok := args[a.Name]; !ok && len(positional) > 0 {
					args[a.Name], positional = positional[0], positional[1:]
				}
			}
			for _, a := range cmd.Args {
				if a.Required && strings.TrimSpace(args[a.Name]) == "" {
					bad(CodeManifest, "", "命令 /%s 缺少参数 %s", name, a.Name)
					break
				}
			}
		}
	}
}
