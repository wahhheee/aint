package state

import "github.com/wahhheee/yanyi/internal/pkg/errs"

// Op 是 update_state 的一个操作（《02-技术方案》11.1）。
type Op struct {
	Op    string `json:"op"`   // set | inc | append | remove | delete
	Path  string `json:"path"` // a.b.c；映射键可为中文，不含 "."
	Value any    `json:"value,omitzero"`
}

// Change 记录一次实际发生的变化，用于工具结果文案（"day 2 → 3"）、检查器状态差异与 SSE 推送。
type Change struct {
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// Apply 在 st 的副本上原子执行 ops：任一操作类型不符则整体不执行并返回说明。
//
// TODO(M2-1)：实现。语义：
//   - set：任意类型，值需匹配字段类型；inc：数字加减；append/remove：列表追加 / 按值移除；
//     delete：删除映射键，其他类型恢复默认值；
//   - 路径语法 a.b.c；映射可以新增键；对象与顶层只能写已定义的字段；
//   - 不限制数值范围，不拒绝"不合情理"的变化（那属于故事本身）。
//
// 返回的 error 文案直接作为工具结果给 AI（如"day 是数字字段，可以写成 3"），见 Check。
func Apply(s *Schema, st map[string]any, ops []Op) (next map[string]any, changes []Change, err error) {
	return nil, nil, errs.NotImplemented("update_state 操作")
}
