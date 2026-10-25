package anthropic

import (
	"io"
	"strings"
	"testing"

	"github.com/wahhheee/yanyi/internal/llm"
)

// 手工协议夹具覆盖难以低成本稳定触发的截断、暂停、拒答与隐藏推理。
func TestStreamEdgeCases(t *testing.T) {
	for _, tc := range []struct {
		name, blocks, stop, contains string
		want                         llm.StopReason
		calls                        int
	}{
		{"截断工具参数", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"a","name":"read","input":{}}}
{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`, "max_tokens", "", llm.StopMaxTokens, 0},
		{"拒答", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"无法回答"}}`, "refusal", "无法回答", llm.StopRefusal, 0},
		{"隐藏推理与暂停", `{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"OPAQUE"}}
{"type":"content_block_stop","index":0}
{"type":"content_block_start","index":1,"content_block":{"type":"server_tool_use","id":"srv","name":"web_search","input":{}}}
{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"雾港\"}"}}
{"type":"content_block_stop","index":1}`, "pause_turn", `"query":"雾港"`, llm.StopPause, 0},
		{"首块已有参数", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"a","name":"read","input":{"path":"/memory"}}}
{"type":"content_block_stop","index":0}`, "tool_use", `"path":"/memory"`, llm.StopToolUse, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			for _, line := range strings.Split(tc.blocks, "\n") {
				b.WriteString("data: " + line + "\n\n")
			}
			b.WriteString(`data: {"type":"message_delta","delta":{"stop_reason":"` + tc.stop + `"}}` + "\n\n" + `data: {"type":"message_stop"}` + "\n\n")
			_, done := collect(t, newStream(t.Context(), io.NopCloser(strings.NewReader(b.String())), "anthropic/m"))
			if done.Stop != tc.want || len(done.Message.ToolCalls()) != tc.calls {
				t.Fatalf("stop=%s calls=%d", done.Stop, len(done.Message.ToolCalls()))
			}
			if tc.contains != "" && !strings.Contains(string(done.Message.Native["anthropic/m"]), tc.contains) {
				t.Fatal("原生块字段丢失")
			}
		})
	}
}
