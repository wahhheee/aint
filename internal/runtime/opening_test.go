package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/anthropic"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/llm/gemini"
	"github.com/wahhheee/aint/internal/llm/openai"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/work"
)

func TestStaticOpeningToolHistory(t *testing.T) {
	for _, tc := range []struct {
		name, result    string
		input           save.TurnInput
		choices, revise bool
	}{
		{name: "选择开场选项", choices: true, input: save.TurnInput{Kind: save.InputChoice, ChoiceID: "b"}, result: "玩家选择：B「等候」"},
		{name: "自由输入作答", choices: true, input: save.TurnInput{Kind: save.InputFree, Text: "问来人是谁"}, result: "玩家自由输入：问来人是谁"},
		{name: "另有输入", choices: true, input: save.TurnInput{Kind: save.InputContinue}, result: prompts.ChoiceNotAnswered},
		{name: "修改开场正文", choices: true, revise: true, input: save.TurnInput{Kind: save.InputChoice, ChoiceID: "b"}, result: "玩家选择：B「等候」"},
		{name: "选项附带命令", choices: true, input: save.TurnInput{Kind: save.InputContinue, ChoiceID: "a"}, result: "玩家选择：A「开门」"},
		{name: "没有开场选项", input: save.TurnInput{Kind: save.InputFree, Text: "问来人是谁"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []*llm.Request
			e := setupWith(t, recordingProvider{Provider: fake.New(&fake.Script{Responses: []fake.Response{{Text: "来人敲响了门。", DelayMS: 1}}}), reqs: &reqs})
			set := work.NewFileSet()
			body := "门外有人。"
			opening := "---\nchoices: []\n"
			if tc.choices {
				opening = "---\nchoices:\n  - {id: a, label: 开门, command: /continue}\n  - {id: b, label: 等候, detail: 留在原地}\n"
			}
			for path, content := range map[string]string{
				"work.yaml":          "format: 1\nopenings:\n  - {id: default, title: 故事开场, mode: static, file: prompts/opening.md}\nid: test.static\nversion: 0.1.0\ntitle: 静态开场\n",
				"prompts/opening.md": opening + "---\n" + body,
			} {
				if err := set.Add(path, []byte(content)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := work.NewService(e.db, e.blobs).ImportFileSet(t.Context(), set, work.Source{Type: "dir"}); err != nil {
				t.Fatal(err)
			}
			sv, err := e.saves.Create(t.Context(), save.CreateParams{WorkID: "test.static"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.revise {
				body = "窗外响起了敲门声。"
				if _, err := e.saves.Rewrite(t.Context(), sv.ID, sv.HeadNodeID, body); err != nil {
					t.Fatal(err)
				}
			}
			_, line, err := e.saves.ProgressLine(t.Context(), sv.ID)
			if err != nil {
				t.Fatal(err)
			}
			msgs, err := e.saves.LoadMessages(t.Context(), line[len(line)-1].MessagesHash)
			if err != nil {
				t.Fatal(err)
			}
			if len(reqs) != 0 || len(msgs) != 2 || msgs[0].Role != llm.RoleUser || msgs[1].Role != llm.RoleAssistant || msgs[1].TextOf() != body {
				t.Fatalf("静态开场不调用模型，助手文本只含正文：%+v", msgs)
			}
			calls := msgs[1].ToolCalls()
			if tc.choices {
				if len(calls) != 1 || calls[0].Name != "offer_choices" || calls[0].ID == "" {
					t.Fatalf("开场选项应记录为工具调用：%+v", calls)
				}
				var args struct {
					Choices []save.ChoiceItem `json:"choices"`
				}
				if err := jsonx.Unmarshal(calls[0].Args, &args); err != nil {
					t.Fatal(err)
				}
				if len(args.Choices) != 2 || args.Choices[0].Command != "/continue" || args.Choices[1].ID != "b" || args.Choices[1].Label != "等候" || args.Choices[1].Detail != "留在原地" {
					t.Fatalf("工具参数应包含开场选项：%+v", args)
				}
			} else if len(calls) != 0 {
				t.Fatalf("没有选项时不应创建工具调用：%+v", calls)
			}

			sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
			defer sub.Close()
			if _, err := e.mgr.Submit(t.Context(), sv.ID, tc.input); err != nil {
				t.Fatal(err)
			}
			waitFor(t, sub, event.TurnCommitted)
			req := reqs[0]
			results := toolResults(req.Messages[len(req.Messages)-1])
			if tc.choices {
				if len(results) != 1 || results[0].CallID != calls[0].ID || results[0].Parts[0].Text != tc.result {
					t.Fatalf("首轮输入应回填到开场选项：%+v", results)
				}
				checkOpeningWire(t, req)
			} else if len(results) != 0 {
				t.Fatalf("没有开场选项时不应发送工具结果：%+v", results)
			}
		})
	}
}

// checkOpeningWire 检查实际运行器请求经各适配器序列化后，系统指令、工具定义与开场调用往返都在原生协议字段中。
func checkOpeningWire(t *testing.T, req *llm.Request) {
	t.Helper()
	type adapter interface {
		Body(context.Context, *llm.Model, *llm.Request) ([]byte, error)
	}
	for _, tc := range []struct {
		name, cache, system, schema, call, result string
		adapter                                   adapter
	}{
		{"Anthropic", "", "system", "input_schema", `"type":"tool_use"`, `"type":"tool_result"`, anthropic.New(llm.ProviderConfig{})},
		{"Responses instructions", "implicit", "instructions", "parameters", `"type":"function_call"`, `"type":"function_call_output"`, openai.New(llm.ProviderConfig{})},
		{"Responses developer", "breakpoints", "input", "parameters", `"type":"function_call"`, `"type":"function_call_output"`, openai.New(llm.ProviderConfig{})},
		{"Gemini", "", "systemInstruction", "parametersJsonSchema", `"functionCall":`, `"functionResponse":`, gemini.New(llm.ProviderConfig{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := tc.adapter.Body(t.Context(), &llm.Model{Name: "test", Caps: llm.Caps{Tools: true, Cache: tc.cache}}, req)
			if err != nil {
				t.Fatal(err)
			}
			var out map[string]jsonx.Value
			if err := jsonx.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(out[tc.system]), "你是这部互动小说的执笔者") || !strings.Contains(string(out[tc.system]), "## 工具用法") ||
				!strings.Contains(string(out["tools"]), `"name":"offer_choices"`) || !strings.Contains(string(out["tools"]), tc.schema) {
				t.Fatalf("系统指令或原生工具定义缺失：%s", raw)
			}
			for _, want := range []string{tc.call, tc.result} {
				if !strings.Contains(string(raw), want) {
					t.Fatalf("请求缺少 %s：%s", want, raw)
				}
			}
			if tc.name == "Gemini" {
				var contents []struct {
					Role  string        `json:"role"`
					Parts []jsonx.Value `json:"parts"`
				}
				if err := jsonx.Unmarshal(out["contents"], &contents); err != nil {
					t.Fatal(err)
				}
				last := contents[len(contents)-1]
				if last.Role != "user" || len(last.Parts) != 2 || !strings.Contains(string(last.Parts[0]), "<pace>") ||
					!strings.Contains(string(last.Parts[1]), `"functionResponse":`) {
					t.Fatalf("Gemini 开场作答应先发文字、再发工具结果：%s", raw)
				}
			}
		})
	}
}
