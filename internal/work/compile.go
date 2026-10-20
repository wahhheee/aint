package work

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/wahhheee/yanyi/internal/state"
	"github.com/wahhheee/yanyi/internal/store"
)

var (
	reWorkID   = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{2,63}$`)
	reDocID    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	reTagName  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	reCmdName  = regexp.MustCompile(`^[\p{L}\p{N}_-]{1,24}$`)
	reSemver   = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	reYAMLLine = regexp.MustCompile(`^\[(\d+):\d+\]`)
)

// Compile 把文件集编译为 Model 并给出校验报告。
//
// 报告有 error 时仍尽量返回 Model（开发模式需要展示"哪里错了"），调用方据 Report.HasErrors 决定是否接受。
// 编译是纯函数：不读写数据库与磁盘，便于测试与 `yanyi work validate`。
func Compile(fs *FileSet) (*Model, *Report) {
	c := &compiler{
		fs: fs,
		r:  &Report{Issues: []Issue{}},
		m:  &Model{Files: map[string]FileRef{}, State: &state.Schema{Fields: map[string]*state.Field{}}},
	}
	for _, p := range fs.Paths() {
		data, _ := fs.Get(p)
		c.m.Files[p] = FileRef{Blob: store.HashBytes(data), Size: len(data)}
	}
	c.m.TreeHash = fs.TreeHash()

	if !c.manifest() {
		return c.m, c.r // 清单都读不出来，后续检查没有意义
	}
	c.m.WorkID = c.m.Manifest.ID
	c.prompts()
	c.stateSchema()
	c.lore()
	c.commands()
	c.markup()
	c.layout()
	c.assets()
	c.crossRefs()
	// TODO(M2-9)：ui/theme.yaml 与 custom.css 校验。
	// TODO(M2-10)：素材解码校验（E-ASSET）、Git LFS 指针检测（E-LFS）、常驻图片 token 估算（W-PIN-IMAGE）。
	// TODO(M2-8)：面板与命令中的 Liquid 模板解析（E-TEMPLATE）。
	// TODO(M2-12)：前缀 token 估算（I-TOKENS、W-PREFIX-LARGE、W-INDEX-LARGE）。
	// TODO(M1-4)：engine 版本范围检查（E-ENGINE）。
	return c.m, c.r
}

type compiler struct {
	fs *FileSet
	m  *Model
	r  *Report
}

// yamlError 把 YAML 解析错误转成报告条目，尽量带上行号。
func (c *compiler) yamlError(file string, lineOffset int, err error) {
	line := 0
	msg := err.Error()
	if sub := reYAMLLine.FindStringSubmatch(msg); sub != nil {
		n, _ := strconv.Atoi(sub[1])
		line = n + lineOffset
	}
	c.r.Errorf(CodeParse, file, line, "检查缩进、冒号与引号", "YAML 解析失败：%s", firstLine(msg))
}

// decode 解码 YAML，并用严格模式再解一次以发现未知字段（W-UNKNOWN-FIELD：保留但不使用）。
func (c *compiler) decode(file string, lineOffset int, data []byte, v any, fresh func() any) bool {
	if err := yaml.Unmarshal(data, v); err != nil {
		c.yamlError(file, lineOffset, err)
		return false
	}
	if fresh != nil {
		if err := yaml.UnmarshalWithOptions(data, fresh(), yaml.DisallowUnknownField()); err != nil {
			c.r.Warnf(CodeUnknownField, file, 0, "可能是拼写错误，或是新版本才支持的字段", "未知字段：%s", firstLine(err.Error()))
		}
	}
	return true
}

func (c *compiler) manifest() bool {
	const file = "work.yaml"
	data, ok := c.fs.Get(file)
	if !ok {
		c.r.Errorf(CodeManifest, file, 0, "作品根目录需要一个 work.yaml", "缺少 work.yaml")
		return false
	}
	c.m.Manifest = defaultManifest()
	if !c.decode(file, 0, data, &c.m.Manifest, func() any { m := defaultManifest(); return &m }) {
		return false
	}
	mf := &c.m.Manifest
	if mf.Format != 1 {
		c.r.Errorf(CodeManifest, file, 0, "目前只支持 format: 1", "format 应为 1，实际为 %d", mf.Format)
	}
	if !reWorkID.MatchString(mf.ID) {
		c.r.Errorf(CodeManifest, file, 0, "建议写成“作者.作品名”，如 someone.first-story",
			"作品 id 不合法：%q（小写字母、数字、点与连字符，3–64 字符）", mf.ID)
	}
	if !reSemver.MatchString(mf.Version) {
		c.r.Errorf(CodeManifest, file, 0, "如 0.1.0", "version 应为 semver 格式，实际为 %q", mf.Version)
	}
	if strings.TrimSpace(mf.Title) == "" {
		c.r.Errorf(CodeManifest, file, 0, "", "缺少 title")
	}
	return !c.r.HasErrors()
}

func (c *compiler) prompts() {
	mf := &c.m.Manifest
	if b, ok := c.fs.Get(mf.Narrator.Instructions); ok {
		c.m.Narrator = strings.TrimSpace(string(b))
	} else if mf.Narrator.Instructions != "prompts/narrator.md" {
		c.r.Errorf(CodeRef, "work.yaml", 0, "", "narrator.instructions 指向的文件不存在：%s", mf.Narrator.Instructions)
	}

	openingFile := mf.Opening.File
	if openingFile == "" {
		openingFile = "prompts/opening.md"
	}
	if b, ok := c.fs.Get(openingFile); ok {
		front, body, _ := splitFrontmatter(b)
		op := &Opening{Mode: mf.Opening.Mode, Body: strings.TrimSpace(body)}
		// TODO(待确认)：规范未写明有 opening.md 但未设 mode 时的默认值，暂按 static（直接展示作者写好的开场）。
		if op.Mode == "" {
			op.Mode = "static"
		}
		if front != nil {
			var fm struct {
				Choices []Choice `yaml:"choices"`
			}
			if c.decode(openingFile, 1, front, &fm, nil) {
				op.Choices = fm.Choices
			}
		}
		c.m.Opening = op
	} else if mf.Opening.File != "" {
		c.r.Errorf(CodeRef, "work.yaml", 0, "", "opening.file 指向的文件不存在：%s", mf.Opening.File)
	}

	if b, ok := c.fs.Get("prompts/memory.md"); ok {
		c.m.MemoryGuide = strings.TrimSpace(string(b))
	}
	if b, ok := c.fs.Get("prompts/compaction.md"); ok {
		c.m.CompactionGuide = strings.TrimSpace(string(b))
	}
	if b, ok := c.fs.Get("README.md"); ok {
		c.m.Readme = string(b)
	}
}

func (c *compiler) stateSchema() {
	data, ok := c.fs.Get("state.yaml")
	if !ok {
		return
	}
	s, err := state.ParseSchema(data)
	if err != nil {
		c.r.Errorf(CodeState, "state.yaml", 0, "类型只有 boolean、number、string、list、map、object 六种", "%s", firstLine(err.Error()))
		return
	}
	c.m.State = s
	for _, name := range s.Order {
		if f := s.Fields[name]; f.Desc == "" && f.Type != state.Object {
			c.r.Warnf(CodeNoDesc, "state.yaml", 0, "写清含义与范围，AI 才会稳定地维护它", "状态字段 %s 缺少 desc", name)
		}
	}
}

func (c *compiler) lore() {
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
	seen := map[string]string{}
	for _, p := range c.fs.Glob("lore", ".md") {
		data, _ := c.fs.Get(p)
		front, body, _ := splitFrontmatter(data)
		var fm docFront
		if front != nil && !c.decode(p, 1, front, &fm, func() any { return &docFront{} }) {
			continue
		}
		base := strings.TrimSuffix(path.Base(p), ".md")
		d := Doc{
			Path: p, ID: fm.ID, Title: fm.Title, Kind: fm.Kind, Aliases: fm.Aliases, Summary: fm.Summary,
			Load: fm.Load, Images: fm.Images, PinImages: fm.PinImages, Display: fm.Display,
			IsIndex: base == "_index", Body: body,
		}
		if d.IsIndex {
			base = path.Base(path.Dir(p))
		}
		if d.ID == "" {
			d.ID = strings.ToLower(base)
		}
		if d.Title == "" {
			d.Title = base
		}
		if d.Load == "" {
			d.Load = c.m.Manifest.Docs.DefaultLoad
		}
		if !d.IsIndex && !reDocID.MatchString(d.ID) {
			c.r.Errorf(CodeName, p, 1, "可在 frontmatter 中用 id 指定，如 id: lin", "文档 ID 不合法：%q", d.ID)
		}
		if prev, dup := seen[d.ID]; dup && !d.IsIndex {
			c.r.Errorf(CodeName, p, 1, "用 frontmatter 的 id 区分", "文档 ID %q 与 %s 重复", d.ID, prev)
		}
		seen[d.ID] = p
		c.m.Docs = append(c.m.Docs, d)
	}
}

func (c *compiler) commands() {
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
	for _, p := range c.fs.Glob("commands", ".md") {
		if path.Dir(p) != "commands" {
			continue // 一个命令一个文件，不递归子目录
		}
		name := strings.TrimSuffix(path.Base(p), ".md")
		if !reCmdName.MatchString(name) {
			c.r.Errorf(CodeName, p, 0, "命令名取文件名：字母（含中文）、数字、_、-，1–24 字符", "命令名不合法：%q", name)
			continue
		}
		if slices.Contains(BuiltinCommands, name) {
			c.r.Errorf(CodeName, p, 0, "换一个文件名", "命令 /%s 与内置命令重名", name)
			continue
		}
		data, _ := c.fs.Get(p)
		front, body, _ := splitFrontmatter(data)
		var fm cmdFront
		if front != nil && !c.decode(p, 1, front, &fm, func() any { return &cmdFront{} }) {
			continue
		}
		cmd := Command{
			Name: name, Title: fm.Title, Description: fm.Description, Args: fm.Args, Present: fm.Present,
			ModeLabel: fm.ModeLabel, ModeStyle: fm.ModeStyle, Icon: fm.Icon, Confirm: fm.Confirm,
			AIVisible: fm.AIVisible == nil || *fm.AIVisible, Template: strings.TrimSpace(body),
		}
		if cmd.Title == "" {
			cmd.Title = name
		}
		if len(cmd.Present) == 0 {
			cmd.Present = []string{"slash"} // TODO(待确认)：规范未写 present 的默认值，暂按斜杠命令
		}
		if cmd.Description == "" {
			c.r.Warnf(CodeNoDesc, p, 1, "description 同时给玩家和 AI 看", "命令 /%s 缺少 description", name)
		}
		c.m.Commands = append(c.m.Commands, cmd)
	}
	slices.SortFunc(c.m.Commands, func(a, b Command) int { return strings.Compare(a.Name, b.Name) })
}

func (c *compiler) markup() {
	const file = "ui/markup.yaml"
	data, ok := c.fs.Get(file)
	if !ok {
		return
	}
	type tagRaw struct {
		Description string   `yaml:"description"`
		Style       TagStyle `yaml:"style"`
		Icon        string   `yaml:"icon"`
		Class       string   `yaml:"class"`
		Tooltip     string   `yaml:"tooltip"`
	}
	var doc struct {
		Tags map[string]tagRaw `yaml:"tags"`
	}
	if !c.decode(file, 0, data, &doc, nil) {
		return
	}
	for name, t := range doc.Tags {
		switch {
		case !reTagName.MatchString(name):
			c.r.Errorf(CodeName, file, 0, "小写字母开头，可含数字与连字符", "标签名不合法：%q", name)
			continue
		case slices.Contains(BuiltinTags, name):
			c.r.Errorf(CodeName, file, 0, "换一个名字", "标签 <%s> 与内置标签重名", name)
			continue
		}
		if t.Description == "" {
			c.r.Warnf(CodeNoDesc, file, 0, "description 告诉 AI 这个标签表示什么、什么时候用", "标签 <%s> 缺少 description", name)
		}
		c.m.Tags = append(c.m.Tags, TagDef{
			Name: name, Description: t.Description, Style: t.Style, Icon: t.Icon, Class: t.Class, Tooltip: t.Tooltip,
		})
	}
	slices.SortFunc(c.m.Tags, func(a, b TagDef) int { return strings.Compare(a.Name, b.Name) })
}

func (c *compiler) layout() {
	const file = "ui/layout.yaml"
	data, ok := c.fs.Get(file)
	if !ok {
		return
	}
	if !c.decode(file, 0, data, &c.m.Layout, func() any { return &Layout{} }) {
		return
	}
	for i := range c.m.Layout.Panels {
		p := &c.m.Layout.Panels[i]
		if p.Format == "" {
			p.Format = "markdown"
		}
		if !slices.Contains([]string{"sidebar", "topbar", "drawer", "float"}, p.Place) {
			c.r.Errorf(CodeManifest, file, 0, "可选 sidebar、topbar、drawer、float", "面板 %s 的 place 不合法：%q", p.ID, p.Place)
		}
		if src, ok := c.fs.Get(p.Template); ok {
			p.Source = string(src)
		} else {
			c.r.Errorf(CodeRef, file, 0, "", "面板 %s 的模板不存在：%s", p.ID, p.Template)
		}
	}
	slices.SortStableFunc(c.m.Layout.Panels, func(a, b PanelDef) int { return a.Order - b.Order })
}

// assetKinds 是支持的素材格式（《03-作品格式规范》13.1）。
var assetKinds = map[string]string{
	".png": "image", ".jpg": "image", ".jpeg": "image", ".webp": "image", ".gif": "image",
	".mp3": "audio", ".ogg": "audio", ".m4a": "audio", ".woff2": "font",
}

func (c *compiler) assets() {
	type meta struct {
		Path        string `yaml:"path"`
		ID          string `yaml:"id"`
		Description string `yaml:"description"`
		Pin         bool   `yaml:"pin"`
		Role        string `yaml:"role"`
	}
	byPath := map[string]meta{}
	if data, ok := c.fs.Get("assets.yaml"); ok {
		var list []meta
		if c.decode("assets.yaml", 0, data, &list, func() any { return &[]meta{} }) {
			for _, m := range list {
				if !c.fs.Has(m.Path) {
					c.r.Errorf(CodeRef, "assets.yaml", 0, "", "素材文件不存在：%s", m.Path)
					continue
				}
				byPath[m.Path] = m
			}
		}
	}
	seen := map[string]string{}
	for _, p := range c.fs.Paths() {
		if !strings.HasPrefix(p, "assets/") {
			continue
		}
		ext := strings.ToLower(path.Ext(p))
		kind, ok := assetKinds[ext]
		if !ok {
			c.r.Warnf(CodeAsset, p, 0, "支持 png、jpg、webp、gif、mp3、ogg、m4a、woff2", "不支持的素材格式，已忽略")
			continue
		}
		m := byPath[p]
		a := Asset{ID: m.ID, Path: p, Kind: kind, Role: m.Role, Description: m.Description, Pin: m.Pin, Blob: c.m.Files[p].Blob}
		if a.ID == "" {
			a.ID = strings.ToLower(strings.TrimSuffix(path.Base(p), path.Ext(p)))
		}
		if a.Role == "" {
			a.Role = map[string]string{"image": "reference", "audio": "bgm", "font": "font"}[kind]
		}
		if prev, dup := seen[a.ID]; dup {
			c.r.Errorf(CodeName, p, 0, "在 assets.yaml 中为其中一个指定 id", "素材 ID %q 与 %s 重复", a.ID, prev)
			continue
		}
		seen[a.ID] = p
		c.m.Assets = append(c.m.Assets, a)
	}
	slices.SortFunc(c.m.Assets, func(a, b Asset) int { return strings.Compare(a.ID, b.ID) })
}

func (c *compiler) crossRefs() {
	for _, f := range c.m.Manifest.NewGame.Fields {
		if !c.statePathExists(f.Key) {
			c.r.Errorf(CodeRef, "work.yaml", 0, "开局表单写入的字段需要先在 state.yaml 中定义",
				"开局表单字段 %s 在状态定义中不存在", f.Key)
		}
	}
	for _, name := range c.m.Layout.CommandBar {
		i := slices.IndexFunc(c.m.Commands, func(cmd Command) bool { return cmd.Name == name })
		if i < 0 {
			c.r.Errorf(CodeRef, "ui/layout.yaml", 0, "", "command_bar 引用了不存在的命令：%s", name)
		} else if !c.m.Commands[i].HasPresent("button") {
			c.r.Warnf(CodeRef, "ui/layout.yaml", 0, "在命令的 present 中加入 button", "命令 /%s 出现在 command_bar 中，但 present 不含 button", name)
		}
	}
	if cover := c.m.Manifest.Cover; cover != "" {
		if _, ok := c.m.Asset(cover); !ok {
			c.r.Errorf(CodeRef, "work.yaml", 0, "", "封面素材不存在：%s", cover)
		}
	}
	// TODO(M2)：其余素材引用（images、display.avatar）与文档链接 [[id]] 的完整性检查。
}

func (c *compiler) statePathExists(p string) bool {
	parts := strings.Split(p, ".")
	fields, order := c.m.State.Fields, c.m.State.Order
	for i, part := range parts {
		if !slices.Contains(order, part) {
			return false
		}
		f := fields[part]
		if i == len(parts)-1 {
			return true
		}
		if f.Type != state.Object {
			return false
		}
		fields, order = f.Fields, f.Order
	}
	return false
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
