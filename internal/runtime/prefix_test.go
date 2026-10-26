package runtime_test

// 跨回合前缀稳定性（M0-6，不变量 1"前缀稳定"）：同一阶段内，请求 i 的 system、tools 与 messages
// 是请求 i+1 的字节前缀（逐条比较 jsonx 序列化结果；CacheMark 不参与序列化）。
// 覆盖：选项作答（offer_choices 结果回填）、回合内工具往返、自由输入、截断续写、场外发言、
// 停止后的中断回合，以及中断之后的下一回合。适配器层的字节前缀见各协议包的 Body 测试。

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/anthropic"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/llm/gemini"
	"github.com/wahhheee/aint/internal/llm/openai"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/save"
)

func TestPrefixStableAcrossTurns(t *testing.T) {
	choices := []fake.ToolCall{{Name: "offer_choices", Args: map[string]any{
		"choices": []any{map[string]any{"id": "a", "label": "追问"}, map[string]any{"id": "b", "label": "离开"}},
	}}}
	script := &fake.Script{
		Responses: []fake.Response{
			{Text: "林舟推开邮局的门。", ChunkRunes: 50, DelayMS: 1, ToolCalls: choices}, // 回合 2：选项作答
			{Text: "他翻开登记簿。", ChunkRunes: 50, DelayMS: 1, ToolCalls: []fake.ToolCall{{Name: "read", // 回合 3：工具往返
				Args: map[string]any{"path": "/memory/index.md"}}}},
			{Text: "登记簿上写着老陈的名字。", ChunkRunes: 50, DelayMS: 1, ToolCalls: choices},
			{Text: "雾越来越浓，", ChunkRunes: 50, DelayMS: 1, Stop: "max_tokens"}, // 回合 4：自由输入作答 + 截断续写
			{Text: "码头的灯一盏盏熄了。", ChunkRunes: 50, DelayMS: 1},
			{Text: strings.Repeat("潮水慢慢涨上来。", 200), ChunkRunes: 2, DelayMS: 5}, // 回合 6：中途停止
			{Text: "他回到柜台后面。", ChunkRunes: 50, DelayMS: 1, ToolCalls: choices}, // 回合 7：中断之后
		},
		OOC: &fake.Response{Text: "<ooc>好的。</ooc>", ChunkRunes: 50, DelayMS: 1}, // 回合 5：场外发言
	}
	var reqs []*llm.Request
	e := setupWith(t, recordingProvider{Provider: fake.New(script), reqs: &reqs})
	ctx := context.Background()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor", Settings: save.Settings{Pace: "short", Reasoning: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	run := func(in save.TurnInput, cancel bool) {
		t.Helper()
		id, err := e.mgr.Submit(ctx, sv.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if cancel {
			waitFor(t, sub, event.TurnText)
			if err := e.mgr.Cancel(sv.ID, id); err != nil {
				t.Fatal(err)
			}
		}
		waitFor(t, sub, event.TurnCommitted)
	}
	run(save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}, false) // 静态开场的选项
	run(save.TurnInput{Kind: save.InputChoice, ChoiceID: "b"}, false)
	run(save.TurnInput{Kind: save.InputFree, Text: "我去码头看看。"}, false)
	run(save.TurnInput{Kind: save.InputOOC, Text: "节奏可以快一点吗？"}, false)
	run(save.TurnInput{Kind: save.InputFree, Text: "我站在码头边等。"}, true)
	run(save.TurnInput{Kind: save.InputFree, Text: "回邮局。"}, false)

	if len(reqs) != 8 {
		t.Fatalf("应调用 8 次模型，实际 %d 次", len(reqs))
	}
	for i := 0; i+1 < len(reqs); i++ {
		a, b := reqs[i], reqs[i+1]
		if !bytes.Equal(jsonx.MustMarshal(a.System), jsonx.MustMarshal(b.System)) {
			t.Fatalf("请求 %d → %d：system 变了", i, i+1)
		}
		if !bytes.Equal(jsonx.MustMarshal(a.Tools), jsonx.MustMarshal(b.Tools)) {
			t.Fatalf("请求 %d → %d：工具定义变了", i, i+1)
		}
		if len(b.Messages) <= len(a.Messages) {
			t.Fatalf("请求 %d → %d：消息应只追加（%d → %d 条）", i, i+1, len(a.Messages), len(b.Messages))
		}
		for j, m := range a.Messages {
			x, y := jsonx.MustMarshal(m), jsonx.MustMarshal(b.Messages[j])
			if !bytes.Equal(x, y) {
				t.Fatalf("请求 %d → %d：第 %d 条消息变了\n之前 %s\n之后 %s", i, i+1, j, x, y)
			}
		}
	}
	checkAdapterPrefix(t, reqs, storeBlobs{e})
}

// storeBlobs 让适配器从测试数据目录读取图片（常驻图片随前缀发送）。
type storeBlobs struct{ e *env }

func (s storeBlobs) ReadBlob(ctx context.Context, hash string) ([]byte, error) {
	return s.e.blobs.Get(ctx, s.e.db.Read(), hash)
}

// 协议层：同一组请求经三个适配器构建请求体，"角色 + 内容块"的序列只追加。
// 缓存标记不参与比较（Anthropic 的滚动断点每次落在最后一块；移动断点不影响缓存前缀与推理块签名）；
// 按内容块而不是按消息比较，因为适配器会合并相邻同角色消息。
func checkAdapterPrefix(t *testing.T, reqs []*llm.Request, blobs llm.BlobReader) {
	t.Helper()
	model := &llm.Model{Name: "m", MaxOutput: 8000, Caps: llm.Caps{Tools: true, Reasoning: "effort", Cache: "breakpoints"}}
	marker := regexp.MustCompile(`,"(cache_control|prompt_cache_breakpoint)":\{[^}]*\}`)
	type body struct {
		Messages []struct {
			Role    string        `json:"role"`
			Content []jsonx.Value `json:"content"`
		} `json:"messages"`
		Input    []jsonx.Value `json:"input"`
		Contents []struct {
			Role  string        `json:"role"`
			Parts []jsonx.Value `json:"parts"`
		} `json:"contents"`
	}
	for _, p := range []interface {
		Body(context.Context, *llm.Model, *llm.Request) ([]byte, error)
	}{anthropic.New(llm.ProviderConfig{Blobs: blobs}), openai.New(llm.ProviderConfig{Blobs: blobs}), gemini.New(llm.ProviderConfig{Blobs: blobs})} {
		var prev []string
		for i, req := range reqs {
			raw, err := p.Body(context.Background(), model, req)
			if err != nil {
				t.Fatal(err)
			}
			var b body
			if err := jsonx.Unmarshal(raw, &b); err != nil {
				t.Fatal(err)
			}
			var seq []string
			for _, m := range b.Messages {
				for _, c := range m.Content {
					seq = append(seq, m.Role+" "+marker.ReplaceAllString(string(c), ""))
				}
			}
			for _, it := range b.Input {
				seq = append(seq, marker.ReplaceAllString(string(it), ""))
			}
			for _, c := range b.Contents {
				for _, part := range c.Parts {
					seq = append(seq, c.Role+" "+string(part))
				}
			}
			if len(seq) <= len(prev) {
				t.Fatalf("%T 请求 %d：内容块没有增加", p, i)
			}
			for j := range prev {
				if prev[j] != seq[j] {
					t.Fatalf("%T 请求 %d → %d：第 %d 块变了\n之前 %s\n之后 %s", p, i-1, i, j, prev[j], seq[j])
				}
			}
			prev = seq
		}
	}
}
