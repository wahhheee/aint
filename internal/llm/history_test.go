package llm

import (
	"testing"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

func TestRepairToolHistory(t *testing.T) {
	call := func(id string) Part { return Part{Kind: PartToolCall, ToolCall: &ToolCall{ID: id, Name: "read"}} }
	result := func(id string) Part {
		return Part{Kind: PartToolResult, ToolResult: &ToolResult{CallID: id, Parts: []Part{Text("结果"), {Kind: PartImage, Image: &ImageRef{Blob: "image"}}}}}
	}
	msg := func(role Role, parts ...Part) Message { return Message{Role: role, Parts: parts} }
	native := map[string]jsonx.Value{"anthropic/model": jsonx.Value(`[{"type":"thinking","signature":"SIG"}]`)}
	for _, tc := range []struct {
		name           string
		in             []Message
		calls, results int
		keepNative     bool
	}{
		{"完整往返", []Message{msg(RoleAssistant, call("a"), call("b")), msg(RoleUser, result("b"), result("a"))}, 2, 2, true},
		{"只完成部分工具", []Message{msg(RoleAssistant, Text("正文"), call("a"), call("b")), msg(RoleUser, result("a"))}, 1, 1, false},
		{"末尾未执行", []Message{msg(RoleAssistant, Text("正文"), call("a"))}, 0, 0, false},
		{"不能跨助手消息配对", []Message{msg(RoleAssistant, call("a")), msg(RoleUser, Text("继续")), msg(RoleAssistant, Text("下一轮")), msg(RoleUser, result("a"))}, 0, 0, false},
		{"同角色消息组", []Message{msg(RoleAssistant, Text("正文")), msg(RoleAssistant, call("a")), msg(RoleUser, result("a")), msg(RoleUser, Text("继续"))}, 1, 1, true},
		{"重复结果", []Message{msg(RoleAssistant, call("a")), msg(RoleUser, result("a"), result("a"))}, 1, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.in[0].Native = native
			before := string(jsonx.MustMarshal(tc.in))
			out := RepairToolHistory(tc.in)
			calls, results := 0, 0
			for _, m := range out {
				calls += len(m.ToolCalls())
				for _, p := range m.Parts {
					if p.Kind == PartToolResult {
						results++
					}
				}
			}
			if calls != tc.calls || results != tc.results {
				t.Fatalf("调用/结果 = %d/%d，期望 %d/%d", calls, results, tc.calls, tc.results)
			}
			if tc.keepNative && len(out[0].Native) == 0 {
				t.Fatal("完整前缀丢失 Native")
			}
			if !tc.keepNative {
				for _, m := range out {
					if len(m.Native) > 0 {
						t.Fatal("修改后的签名未清理")
					}
				}
			}
			if before != string(jsonx.MustMarshal(tc.in)) {
				t.Fatal("修改了存储消息")
			}
			if string(jsonx.MustMarshal(out)) != string(jsonx.MustMarshal(RepairToolHistory(out))) {
				t.Fatal("重复修复结果不同")
			}
		})
	}
	for _, role := range []Role{RoleUser, RoleAssistant} {
		out := RepairToolHistory([]Message{msg(role, result("orphan"))})
		if len(out) != 1 || len(out[0].Parts) != 2 || out[0].Parts[1].Image.Blob != "image" {
			t.Fatal("孤儿结果未保留图片")
		}
	}
}
