package runtime_test

import (
	"testing"

	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/save"
)

func TestBuiltinDemo(t *testing.T) {
	script, err := fake.LoadScript("demo")
	if err != nil {
		t.Fatal(err)
	}
	for i := range script.Responses {
		script.Responses[i].FirstDelayMS = 1
		script.Responses[i].DelayMS = 1
		script.Responses[i].ChunkRunes = 1000
	}
	e := setup(t, script)
	sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "aint.inverted-stars"})
	if err != nil {
		t.Fatal(err)
	}
	for _, choice := range []string{"letter", "b"} {
		id := play(t, e, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: choice})
		node, err := e.saves.Node(t.Context(), sv.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		if node.Meta.Stop != "choices" {
			t.Fatalf("演示回合应交还选项：%+v", node)
		}
		messages, err := e.saves.LoadMessages(t.Context(), node.MessagesHash)
		if err != nil {
			t.Fatal(err)
		}
		for _, message := range messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil && part.ToolResult.IsError {
					t.Fatalf("演示工具调用失败：%+v", part.ToolResult)
				}
			}
		}
	}
}
