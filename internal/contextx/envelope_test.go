package contextx_test

import (
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
)

func TestChoiceAnswer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  save.TurnInput
		text   string
		result string
		body   string
	}{
		{"普通选项", save.TurnInput{Kind: save.InputChoice, ChoiceID: "b", ChoiceLabel: "跟着林舟"}, "跟着林舟", "玩家选择：B「跟着林舟」", ""},
		{"自由输入", save.TurnInput{Kind: save.InputFree}, "我去码头", "玩家自由输入：我去码头", ""},
		{"命令选项", save.TurnInput{Kind: save.InputCommand, ChoiceID: "b", ChoiceLabel: "跟着林舟"}, "叙述此后跟随 林舟", "玩家选择：B「跟着林舟」", "<input kind=\"command\">\n叙述此后跟随 林舟\n</input>"},
		{"场外选项", save.TurnInput{Kind: save.InputOOC, ChoiceID: "b", ChoiceLabel: "放慢节奏"}, "节奏慢一点", "玩家选择：B「放慢节奏」", "<ooc>\n节奏慢一点\n</ooc>"},
		{"继续选项", save.TurnInput{Kind: save.InputContinue, ChoiceID: "b", ChoiceLabel: "继续观察"}, "", "玩家选择：B「继续观察」", "<input kind=\"continue\">\n（玩家希望你接着写下去）\n</input>"},
		{"独立命令", save.TurnInput{Kind: save.InputCommand}, "叙述此后跟随 林舟", prompts.ChoiceNotAnswered, "<input kind=\"command\">\n叙述此后跟随 林舟\n</input>"},
		{"独立场外", save.TurnInput{Kind: save.InputOOC}, "节奏慢一点", prompts.ChoiceNotAnswered, "<ooc>\n节奏慢一点\n</ooc>"},
		{"独立继续", save.TurnInput{Kind: save.InputContinue}, "", prompts.ChoiceNotAnswered, "<input kind=\"continue\">\n（玩家希望你接着写下去）\n</input>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := contextx.Envelope{Input: tc.input, Text: tc.text, PendingCall: "choices", Pace: "medium"}
			msg := env.Message(prompts.FeatureSet{prompts.FOOC: true})
			if len(msg.Parts) != 2 || msg.Parts[0].Kind != llm.PartToolResult {
				t.Fatalf("应先回填工具结果，再发送玩家消息：%+v", msg)
			}
			result := msg.Parts[0].ToolResult
			if result.CallID != "choices" || len(result.Parts) != 1 || result.Parts[0].Text != tc.result {
				t.Fatalf("工具结果 = %+v，期望 %q", result, tc.result)
			}
			body := msg.Parts[1].Text
			if tc.body == "" {
				if strings.Contains(body, "<input") || strings.Contains(body, "<ooc>") {
					t.Fatalf("工具结果已包含玩家输入，不应重复发送：%s", body)
				}
			} else if !strings.Contains(body, tc.body) {
				t.Fatalf("玩家消息缺少 %q：%s", tc.body, body)
			}
		})
	}
}
