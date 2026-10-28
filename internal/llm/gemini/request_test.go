package gemini

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

type orderBlobs struct{}

func (orderBlobs) ReadBlob(_ context.Context, hash string) ([]byte, error) {
	return []byte(hash), nil
}

func TestBodyMixedToolResults(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, split := range []bool{false, true} {
			t.Run(fmt.Sprintf("内嵌图片=%t/分条用户消息=%t", nested, split), func(t *testing.T) {
				m := &llm.Model{Name: "gemini-4-argon", Caps: llm.Caps{ImageInToolResult: nested}}
				native := jsonx.Value(`[{"text":"先查资料","thoughtSignature":"TEXT_SIG"},{"functionCall":{"id":"c1","name":"read","args":{}},"thoughtSignature":"CALL_SIG"},{"functionCall":{"id":"c2","name":"read","args":{}}}]`)
				img := func(blob string) llm.Part {
					return llm.Part{Kind: llm.PartImage, Image: &llm.ImageRef{Blob: blob, MIME: "image/png"}}
				}
				user := []llm.Part{
					llm.Text("<notes>注记</notes>"),
					{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: "c1", Parts: []llm.Part{llm.Text("结果一"), img("a")}}},
					llm.Text("玩家附件"), img("b"),
					{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: "c2", IsError: true, Parts: []llm.Part{llm.Text("结果二"), img("c")}}},
					llm.Text("<pace>简短</pace>继续重写"),
				}
				req := &llm.Request{Messages: []llm.Message{
					{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("开始")}},
					{Role: llm.RoleAssistant, Parts: []llm.Part{
						llm.Text("先查资料"),
						{Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{ID: "c1", Name: "read", Args: jsonx.Value(`{}`)}},
						{Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{ID: "c2", Name: "read", Args: jsonx.Value(`{}`)}},
					}, Native: map[string]jsonx.Value{llm.NativeKey(llm.ProtocolGemini, m.Name): native}},
				}}
				if split {
					for _, part := range user {
						req.Messages = append(req.Messages, llm.Message{Role: llm.RoleUser, Parts: []llm.Part{part}})
					}
				} else {
					req.Messages = append(req.Messages, llm.Message{Role: llm.RoleUser, Parts: user})
				}
				before := jsonx.MustMarshal(req)
				p := New(llm.ProviderConfig{Blobs: orderBlobs{}})
				body, err := p.Body(t.Context(), m, req)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, jsonx.MustMarshal(req)) {
					t.Fatal("序列化修改了中立消息或原生内容")
				}
				var out request
				if err := jsonx.Unmarshal(body, &out); err != nil {
					t.Fatal(err)
				}
				if len(out.Contents) != 3 || out.Contents[2].Role != "user" {
					t.Fatalf("消息角色与合并结果不符：%s", body)
				}
				if !bytes.Equal(jsonx.MustMarshal(out.Contents[1].Parts), native) {
					t.Fatal("助手原生部件或签名发生变化")
				}
				want := `[{"text":"<notes>注记</notes>"},`
				if !nested {
					want += `{"inlineData":{"mimeType":"image/png","data":"YQ=="}},`
				}
				want += `{"text":"玩家附件"},{"inlineData":{"mimeType":"image/png","data":"Yg=="}},`
				if !nested {
					want += `{"inlineData":{"mimeType":"image/png","data":"Yw=="}},`
				}
				want += `{"text":"<pace>简短</pace>继续重写"},{"functionResponse":{"id":"c1","name":"read","response":{"output":"结果一"}`
				if nested {
					want += `,"parts":[{"inlineData":{"mimeType":"image/png","data":"YQ=="}}]`
				}
				want += `}},{"functionResponse":{"id":"c2","name":"read","response":{"error":"结果二"}`
				if nested {
					want += `,"parts":[{"inlineData":{"mimeType":"image/png","data":"Yw=="}}]`
				}
				want += `}}]`
				if got := string(jsonx.MustMarshal(out.Contents[2].Parts)); got != want {
					t.Fatalf("用户部件顺序或内容不符\n实际：%s\n期望：%s", got, want)
				}

				req.Messages = append(req.Messages,
					llm.Message{Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text("本回合正文")}},
					llm.Message{Role: llm.RoleUser, Parts: []llm.Part{llm.Text("下一回合")}},
				)
				next, err := p.Body(t.Context(), m, req)
				if err != nil {
					t.Fatal(err)
				}
				var after request
				if err := jsonx.Unmarshal(next, &after); err != nil {
					t.Fatal(err)
				}
				if len(after.Contents) != 5 || !bytes.Equal(jsonx.MustMarshal(out.Contents), jsonx.MustMarshal(after.Contents[:3])) {
					t.Fatal("后续回合修改了已发送的历史前缀")
				}
			})
		}
	}
}
