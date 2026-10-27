package runtime

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/work"
)

func TestToolHistoryCleaner(t *testing.T) {
	native := map[string]jsonx.Value{"signed/model": jsonx.Value(`{"signature":"签名","text":"<offer_choices/>"}`)}
	call := func(id, name string) llm.Part {
		return llm.Part{Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{ID: id, Name: name, Args: jsonx.Value(`{}`)}}
	}
	result := func(id, text string, failed bool) llm.Part {
		return llm.Part{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: id, IsError: failed, Parts: []llm.Part{llm.Text(text)}}}
	}
	before := llm.Message{Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text("此前正常的故事。")}, Native: native}
	bad := llm.Message{Role: llm.RoleAssistant, Parts: []llm.Part{
		{Kind: llm.PartReasoning, Text: "错误的思考"}, llm.Text("灯亮了。<offer_"),
		llm.Text(`choices"><parameter name="choices">[]</｜DSML｜parameter></offer_choices>门开了。`), call("rejected", "update_state"),
	}, Native: native}
	repair := llm.Message{Role: llm.RoleUser, Parts: []llm.Part{
		result("rejected", prompts.ToolTextRejectedCall, true), llm.Text(fmt.Sprintf(prompts.ToolTextRepairNotice, "offer_choices、update_state")),
	}}
	good := llm.Message{Role: llm.RoleAssistant, Parts: []llm.Part{
		{Kind: llm.PartReasoning, Text: "重新调用的思考"}, call("updated", "update_state"), call("choices", "offer_choices"),
	}, Native: native}
	input := []llm.Message{before, bad, repair, good,
		{Role: llm.RoleUser, Parts: []llm.Part{result("updated", "状态已更新", false)}},
		{Role: llm.RoleUser, Parts: []llm.Part{result("choices", "玩家选择 A", false)}},
		{Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text("后来的故事。")}, Native: native},
	}
	stored := jsonx.MustMarshal(input)
	cleaner := toolHistoryCleaner{specs: []llm.ToolSpec{{Name: "offer_choices"}, {Name: "update_state"}}}
	clean := cleaner.clean(input)
	if len(clean) != 6 || clean[1].TextOf() != "灯亮了。门开了。" {
		t.Fatalf("应只去掉错误示范与纠正过程：%+v", clean)
	}
	if len(clean[0].Native) != 1 || len(clean[1].ToolCalls()) != 0 || len(clean[2].ToolCalls()) != 2 {
		t.Fatal("应保留错误前的原生消息与成功调用，移除退回的调用")
	}
	for _, msg := range clean[1:] {
		if len(msg.Native) != 0 {
			t.Fatal("被改写历史之后的签名不能沿用")
		}
		for _, part := range msg.Parts {
			if part.Kind == llm.PartReasoning || strings.Contains(part.Text, "<offer_choices") || isToolRepairNotice(part.Text) {
				t.Fatalf("错误示范仍在发送历史中：%+v", part)
			}
		}
	}
	if !bytes.Equal(jsonx.MustMarshal(input), stored) {
		t.Fatal("不能改写存储消息")
	}
	if !bytes.Equal(jsonx.MustMarshal(llm.RepairToolHistory(clean)), jsonx.MustMarshal(clean)) {
		t.Fatal("成功工具调用与结果必须保持配对")
	}
	// 分回合读取必须与一次性处理一致，后续回合的原生数据也应失效。
	split := toolHistoryCleaner{specs: cleaner.specs}
	first := split.clean(input[:4])
	second := split.clean(input[4:])
	if !bytes.Equal(jsonx.MustMarshal(append(first, second...)), jsonx.MustMarshal(clean)) {
		t.Fatal("历史清理依赖读取分组")
	}
	// 已清理的历史再次读取不应变化。
	again := toolHistoryCleaner{specs: cleaner.specs}
	if !bytes.Equal(jsonx.MustMarshal(again.clean(clean)), jsonx.MustMarshal(clean)) {
		t.Fatal("清理必须幂等")
	}
}

func TestToolHistoryCleanerKeepsLiteralExamples(t *testing.T) {
	for _, tc := range []struct {
		text string
		tags []work.TagDef
	}{
		{text: "`<offer_choices/>`"},
		{text: `<offer_choices>这是样式</offer_choices>`, tags: []work.TagDef{{Name: "offer_choices"}}},
		{text: "<say who=\"lin\">正常台词</say>"},
		{text: "未完的比较符 <"},
	} {
		messages := []llm.Message{{Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text(tc.text)}, Native: map[string]jsonx.Value{"test/model": jsonx.Value(`[]`)}}}
		cleaner := toolHistoryCleaner{specs: []llm.ToolSpec{{Name: "offer_choices"}}, tags: tc.tags}
		if !bytes.Equal(jsonx.MustMarshal(cleaner.clean(messages)), jsonx.MustMarshal(messages)) {
			t.Fatalf("合法正文与其原生数据应保持不变：%q", tc.text)
		}
	}
}
