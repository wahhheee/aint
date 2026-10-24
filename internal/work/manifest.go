package work

// Manifest 是 work.yaml（《03-作品格式规范》第 5 节）。
// 同时带 yaml 与 json 标签：yaml 用于解析作品，json 用于存库（work_versions.manifest）与 API。
//
// 默认值的处理方式：先用 defaultManifest() 填好默认值，再把 YAML 解码到它上面——
// 文件中出现的字段覆盖默认值，未出现的保持默认。这样字段都是普通值类型，使用处无需判空。
type Manifest struct {
	Format        int          `yaml:"format" json:"format"`
	ID            string       `yaml:"id" json:"id"`
	Version       string       `yaml:"version" json:"version"`
	Title         string       `yaml:"title" json:"title"`
	Subtitle      string       `yaml:"subtitle" json:"subtitle,omitzero"`
	Authors       []Author     `yaml:"authors" json:"authors,omitzero"`
	Language      string       `yaml:"language" json:"language"`
	Description   string       `yaml:"description" json:"description,omitzero"`
	Tags          []string     `yaml:"tags" json:"tags,omitzero"`
	ContentRating string       `yaml:"content_rating" json:"content_rating,omitzero"` // general | teen | mature
	License       string       `yaml:"license" json:"license,omitzero"`
	Cover         string       `yaml:"cover" json:"cover,omitzero"`   // 素材引用
	Engine        string       `yaml:"engine" json:"engine,omitzero"` // semver 范围
	Requires      Requires     `yaml:"requires" json:"requires"`
	Narrator      NarratorConf `yaml:"narrator" json:"narrator"`
	Opening       OpeningConf  `yaml:"opening" json:"opening"`
	NewGame       NewGame      `yaml:"new_game" json:"new_game"`
	Features      Features     `yaml:"features" json:"features"`
	Presentation  Presentation `yaml:"presentation" json:"presentation"`
	Docs          DocsConf     `yaml:"docs" json:"docs"`
	Changelog     string       `yaml:"changelog" json:"changelog,omitzero"`
}

type Author struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url,omitzero"`
}

type Requires struct {
	Vision string `yaml:"vision" json:"vision"` // none | preferred | required
}

type NarratorConf struct {
	Instructions string `yaml:"instructions" json:"instructions"` // 默认 prompts/narrator.md
	Length       string `yaml:"length" json:"length"`             // short | medium | long | "<最少>-<最多>"
	Reasoning    string `yaml:"reasoning" json:"reasoning,omitzero"`
}

type OpeningConf struct {
	File string `yaml:"file" json:"file,omitzero"` // 默认 prompts/opening.md（存在时）
	Mode string `yaml:"mode" json:"mode,omitzero"` // static | generate
}

type NewGame struct {
	Fields []FormField `yaml:"fields" json:"fields,omitzero"`
}

// FormField 是开局表单字段，值写入初始状态的 Key 路径。
type FormField struct {
	Key       string   `yaml:"key" json:"key"`
	Label     string   `yaml:"label" json:"label"`
	Type      string   `yaml:"type" json:"type"` // text | textarea | number | select | multiselect | toggle
	Default   any      `yaml:"default" json:"default,omitzero"`
	Options   []string `yaml:"options" json:"options,omitzero"`
	Min       *float64 `yaml:"min" json:"min,omitzero"`
	Max       *float64 `yaml:"max" json:"max,omitzero"`
	MaxLength int      `yaml:"max_length" json:"max_length,omitzero"`
	Hint      string   `yaml:"hint" json:"hint,omitzero"`
	Required  bool     `yaml:"required" json:"required,omitzero"`
}

type Features struct {
	FreeInput    bool `yaml:"free_input" json:"free_input"`
	ForceChoices bool `yaml:"force_choices" json:"force_choices"` // 正文结束时未给选项则追加提示，继续工具循环
	Dice         bool `yaml:"dice" json:"dice"`
	TimedChoices bool `yaml:"timed_choices" json:"timed_choices"`
	OOC          bool `yaml:"ooc" json:"ooc"`
}

type Presentation struct {
	Dialogue    string `yaml:"dialogue" json:"dialogue"` // plain | tagged
	Stage       bool   `yaml:"stage" json:"stage"`
	TextEffects bool   `yaml:"text_effects" json:"text_effects"`
}

type DocsConf struct {
	DefaultLoad string `yaml:"default_load" json:"default_load"` // always | index | none
}

// defaultManifest 返回填好默认值的清单（规范 5.1 中各字段的默认值）。
func defaultManifest() Manifest {
	return Manifest{
		Language:     "zh-CN",
		Requires:     Requires{Vision: "none"},
		Narrator:     NarratorConf{Instructions: "prompts/narrator.md", Length: "medium"},
		Features:     Features{FreeInput: true, OOC: true},
		Presentation: Presentation{Dialogue: "plain", Stage: true, TextEffects: false}, // 特效默认关闭（D-23）
		Docs:         DocsConf{DefaultLoad: "index"},
	}
}
