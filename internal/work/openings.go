package work

import (
	"fmt"
	"maps"
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
				var fm struct {
					Choices []Choice `yaml:"choices"`
				}
				if front != nil && c.decode(op.File, 1, front, &fm, nil) {
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
