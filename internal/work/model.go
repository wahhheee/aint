package work

import "github.com/wahhheee/yanyi/internal/state"

// Model 是作品某个版本编译后的只读内存模型。创建后不得修改（多个存档并发读取）。
type Model struct {
	WorkID    string
	VersionID string
	TreeHash  string
	Manifest  Manifest

	Narrator        string   // 执笔指令原文（prompts/narrator.md）
	Opening         *Opening // nil 表示没有开场文件：以生成式开场开始
	MemoryGuide     string   // prompts/memory.md
	CompactionGuide string   // prompts/compaction.md
	Readme          string   // README.md（只展示给玩家，不发给 AI）

	Docs     []Doc         // lore/ 下的设定文档，按路径排序
	State    *state.Schema // 未定义状态时为空 Schema（Empty() 为真）
	Commands []Command     // 按名称排序
	Tags     []TagDef      // ui/markup.yaml 中的样式标签，按名称排序
	Layout   Layout        // ui/layout.yaml

	// Files 是作品文件索引：路径 → blob 引用（VFS /work 读取、素材服务、记忆模板复制时按需取内容）。
	Files map[string]FileRef

	// TODO(M2)：Theme（ui/theme.yaml）、Assets（assets.yaml + 素材索引）、MemoryTemplates（memory/）。
}

// FileRef 引用作品中的一个文件（内容在 blob 存储中）。
type FileRef struct {
	Blob string `json:"blob"`
	Size int    `json:"size"`
}

// Opening 是开场（prompts/opening.md）。
type Opening struct {
	Mode    string   `json:"mode"` // static | generate
	Body    string   `json:"body"` // Liquid 模板，建档时以初始状态渲染一次（TODO(M2-11)）
	Choices []Choice `json:"choices,omitzero"`
}

// Choice 是开场的初始选项（仅 static 模式）。
type Choice struct {
	ID     string `yaml:"id" json:"id"`
	Label  string `yaml:"label" json:"label"`
	Detail string `yaml:"detail" json:"detail,omitzero"`
}

// Doc 是一份设定文档（《03-作品格式规范》第 7 节）。
type Doc struct {
	Path      string     `json:"path"` // 相对作品根，如 lore/characters/lin.md
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Kind      string     `json:"kind,omitzero"`
	Aliases   []string   `json:"aliases,omitzero"`
	Summary   string     `json:"summary,omitzero"`
	Load      string     `json:"load"` // always | index | none
	Images    []string   `json:"images,omitzero"`
	PinImages bool       `json:"pin_images,omitzero"`
	Display   DocDisplay `json:"display,omitzero"`
	IsIndex   bool       `json:"is_index,omitzero"` // 目录摘要 _index.md
	Body      string     `json:"-"`
}

type DocDisplay struct {
	Color  string `yaml:"color" json:"color,omitzero"`
	Avatar string `yaml:"avatar" json:"avatar,omitzero"`
}

// Command 是作品命令（《03-作品格式规范》第 9 节）。
type Command struct {
	Name        string       `json:"name"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Args        []CommandArg `json:"args,omitzero"`
	Present     []string     `json:"present"` // slash | mode | button
	ModeLabel   string       `json:"mode_label,omitzero"`
	ModeStyle   string       `json:"mode_style,omitzero"`
	Icon        string       `json:"icon,omitzero"`
	Confirm     string       `json:"confirm,omitzero"`
	AIVisible   bool         `json:"ai_visible"`
	Template    string       `json:"-"` // 发给 AI 的 Liquid 模板
}

type CommandArg struct {
	Name     string   `yaml:"name" json:"name"`
	Hint     string   `yaml:"hint" json:"hint,omitzero"`
	Options  []string `yaml:"options" json:"options,omitzero"`
	Required bool     `yaml:"required" json:"required,omitzero"`
}

// HasPresent 判断命令是否以某种方式呈现。
func (c Command) HasPresent(p string) bool {
	for _, x := range c.Present {
		if x == p {
			return true
		}
	}
	return false
}

// TagDef 是作品样式标签（《03-作品格式规范》第 10 节）。
type TagDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Style       TagStyle `json:"style"`
	Icon        string   `json:"icon,omitzero"`
	Class       string   `json:"class,omitzero"`
	Tooltip     string   `json:"tooltip,omitzero"`
}

type TagStyle struct {
	Color         string   `yaml:"color" json:"color,omitzero"`
	DarkColor     string   `yaml:"dark_color" json:"dark_color,omitzero"`
	Background    string   `yaml:"background" json:"background,omitzero"`
	Weight        string   `yaml:"weight" json:"weight,omitzero"`
	Italic        bool     `yaml:"italic" json:"italic,omitzero"`
	Underline     string   `yaml:"underline" json:"underline,omitzero"`
	Size          float64  `yaml:"size" json:"size,omitzero"`
	Font          string   `yaml:"font" json:"font,omitzero"`
	LetterSpacing float64  `yaml:"letter_spacing" json:"letter_spacing,omitzero"`
	Opacity       *float64 `yaml:"opacity" json:"opacity,omitzero"`
	Effect        string   `yaml:"effect" json:"effect,omitzero"`
}

// Layout 是面板布局（《03-作品格式规范》第 11 节）。
type Layout struct {
	Panels     []PanelDef `yaml:"panels" json:"panels,omitzero"`
	CommandBar []string   `yaml:"command_bar" json:"command_bar,omitzero"`
}

type PanelDef struct {
	ID        string `yaml:"id" json:"id"`
	Title     string `yaml:"title" json:"title,omitzero"`
	Place     string `yaml:"place" json:"place"` // sidebar | topbar | drawer | float
	Template  string `yaml:"template" json:"template"`
	Format    string `yaml:"format" json:"format"` // markdown | html
	Order     int    `yaml:"order" json:"order,omitzero"`
	Collapsed bool   `yaml:"collapsed" json:"collapsed,omitzero"`
	Source    string `yaml:"-" json:"-"` // 模板源码（编译时读入）
}

// 保留名（《03-作品格式规范》第 4 节）。
var (
	BuiltinCommands = []string{"ooc", "continue", "compact", "help"}
	BuiltinTags     = []string{"scene", "cg", "say", "bgm", "sfx", "pause", "fx", "aside", "act"}
)
