package work

// YAML 与 frontmatter 的输入结构，供编译器与编辑器 Schema 共用。
type docFront struct {
	ID        string     `yaml:"id"`
	Title     string     `yaml:"title"`
	Kind      string     `yaml:"kind"`
	Aliases   []string   `yaml:"aliases"`
	Summary   string     `yaml:"summary"`
	Load      string     `yaml:"load"`
	Images    []string   `yaml:"images"`
	PinImages bool       `yaml:"pin_images"`
	Display   DocDisplay `yaml:"display"`
}

type cmdFront struct {
	Title       string       `yaml:"title"`
	Description string       `yaml:"description"`
	Args        []CommandArg `yaml:"args"`
	Present     []string     `yaml:"present"`
	ModeLabel   string       `yaml:"mode_label"`
	ModeStyle   string       `yaml:"mode_style"`
	Icon        string       `yaml:"icon"`
	Confirm     string       `yaml:"confirm"`
	AIVisible   *bool        `yaml:"ai_visible"`
}

type tagRaw struct {
	Description string   `yaml:"description"`
	Style       TagStyle `yaml:"style"`
	Icon        string   `yaml:"icon"`
	Class       string   `yaml:"class"`
	Tooltip     string   `yaml:"tooltip"`
}

type meta struct {
	Path        string `yaml:"path"`
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	Pin         bool   `yaml:"pin"`
	Role        string `yaml:"role"`
}
