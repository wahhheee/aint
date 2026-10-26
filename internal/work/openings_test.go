package work

import (
	"strings"
	"testing"
)

func TestOpeningsValidation(t *testing.T) {
	for _, tc := range []struct {
		name, openings, body string
		invalid              bool
	}{
		{name: "无指引的生成式", openings: "  - {id: start, title: 开始, mode: generate}\n"},
		{name: "静态正文与选项", openings: "  - {id: start, title: 开始, mode: static, file: start.md}\n", body: "---\nchoices: [{id: a, label: 推门}]\n---\n开始。"},
		{name: "缺少列表", invalid: true},
		{name: "重复ID", openings: "  - {id: start, title: 甲, mode: generate}\n  - {id: start, title: 乙, mode: generate}\n", invalid: true},
		{name: "非法ID", openings: "  - {id: ../bad, title: 开始, mode: generate}\n", invalid: true},
		{name: "空名称", openings: "  - {id: start, title: ' ', mode: generate}\n", invalid: true},
		{name: "缺少模式", openings: "  - {id: start, title: 开始}\n", invalid: true},
		{name: "非法模式", openings: "  - {id: start, title: 开始, mode: other}\n", invalid: true},
		{name: "静态缺少文件", openings: "  - {id: start, title: 开始, mode: static}\n", invalid: true},
		{name: "文件不存在", openings: "  - {id: start, title: 开始, mode: generate, file: missing.md}\n", invalid: true},
		{name: "重复选项", openings: "  - {id: start, title: 开始, mode: static, file: start.md}\n", body: "---\nchoices: [{id: a, label: 推门}, {id: a, label: 等待}]\n---\n开始。", invalid: true},
		{name: "生成式不声明选项", openings: "  - {id: start, title: 开始, mode: generate, file: start.md}\n", body: "---\nchoices: [{id: a, label: 推门}]\n---\n开始。", invalid: true},
		{name: "未知状态", openings: "  - {id: start, title: 开始, mode: generate, initial_state: {missing: 1}}\n", invalid: true},
		{name: "状态类型错误", openings: "  - {id: start, title: 开始, mode: generate, initial_state: {day: 雨天}}\n", invalid: true},
		{name: "正确覆盖", openings: "  - {id: start, title: 开始, mode: generate, initial_state: {day: 3}}\n"},
		{name: "每个开场都校验模板", openings: "  - {id: a, title: 甲, mode: generate}\n  - {id: b, title: 乙, mode: static, file: start.md}\n", body: "{% if state.day %}", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := NewFileSet()
			manifest := "format: 1\nid: test.openings\nversion: 1.0.0\ntitle: 多开场\n"
			if tc.openings != "" {
				manifest += "openings:\n" + tc.openings
			}
			for path, text := range map[string]string{"work.yaml": manifest, "state.yaml": "fields:\n  day: {type: number, default: 1, desc: 天数}\n", "start.md": tc.body} {
				if err := set.Add(path, []byte(text)); err != nil {
					t.Fatal(err)
				}
			}
			_, report := Compile(set)
			if report.HasErrors() != tc.invalid {
				t.Fatalf("校验结果不符：%+v", report.Issues)
			}
		})
	}
}

func TestOpeningStateIsolation(t *testing.T) {
	m, report := compileExample(t, "misty-harbor")
	if report.HasErrors() {
		t.Fatal(report.Issues)
	}
	op, err := m.Opening("arriving")
	if err != nil {
		t.Fatal(err)
	}
	first, err := op.State(m.State)
	if err != nil {
		t.Fatal(err)
	}
	first["trust"].(map[string]any)["林舟"] = float64(99)
	second, err := op.State(m.State)
	if err != nil {
		t.Fatal(err)
	}
	if second["trust"].(map[string]any)["林舟"] != float64(0) {
		t.Fatal("存档不能修改作品开场的覆盖值")
	}
	defaultOpening, err := m.Opening("")
	if err != nil || defaultOpening.ID != "post-office" {
		t.Fatal("默认开场应为第一项", err)
	}
	if _, err := m.Opening("missing"); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatal("未知 ID 应拒绝")
	}
}
