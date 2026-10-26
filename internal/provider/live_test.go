package provider

// 真实模型冒烟测试（M0-1 / M0-2 核对清单）。会产生费用，默认跳过。
//
// 运行：
//
//	AINT_LIVE=.local/secrets/live.json go test ./internal/provider -run TestLive -v
//	AINT_LIVE=… AINT_LIVE_ONLY=claude go test ./internal/provider -run "TestLive/claude/roundtrip" -v
//
// live.json（不入库，含密钥与中转站地址）：
//
//	{"providers": [{"name": "…", "protocol": "anthropic", "base_url": "…", "key": "…", "model": "…"}]}
//
// AINT_LIVE_TRACE=目录 时记录协议原始报文（录制回放夹具的来源）。
// 每个子测试都只发最少的请求，并在日志中输出用量，结论汇总在 docs/notes/api-verification.md。

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

type liveProvider struct {
	Name     string       `json:"name"`
	Protocol llm.Protocol `json:"protocol"`
	BaseURL  string       `json:"base_url"`
	Key      string       `json:"key"`
	Model    string       `json:"model"`
}

var liveBlobs = memBlobReader{
	"sha256:red":  solidPNG(color.RGBA{220, 20, 20, 255}),
	"sha256:blue": solidPNG(color.RGBA{20, 40, 220, 255}),
}

func TestLive(t *testing.T) {
	path := os.Getenv("AINT_LIVE")
	if path == "" {
		t.Skip("真实模型测试：设置 AINT_LIVE=<live.json> 后运行（会产生费用）")
	}
	b, err := os.ReadFile(path) //nolint:gosec // G703：测试配置文件路径由开发者通过环境变量指定
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Providers []liveProvider `json:"providers"`
	}
	if err := jsonx.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	only := os.Getenv("AINT_LIVE_ONLY")
	for _, lp := range cfg.Providers {
		if only != "" && !strings.Contains(","+only+",", ","+lp.Name+",") {
			continue
		}
		t.Run(lp.Name, func(t *testing.T) {
			adapter, err := NewAdapter(llm.ProviderConfig{
				Protocol: lp.Protocol, BaseURL: lp.BaseURL, APIKey: lp.Key, Blobs: liveBlobs, TraceDir: os.Getenv("AINT_LIVE_TRACE"),
			})
			if err != nil {
				t.Fatal(err)
			}
			p := llm.WithRetry(adapter, llm.RetryPolicy{MaxRetries: 2, BaseDelay: 3 * time.Second, MaxDelay: 20 * time.Second})
			m := &llm.Model{Name: lp.Model, MaxOutput: 32_000}
			if pre, ok := PresetFor(lp.Protocol, lp.Model); ok {
				m.Caps, m.ContextWindow = pre.Caps, pre.ContextWindow
			} else {
				t.Fatalf("模型 %s 没有预设", lp.Model)
			}
			lt := &liveT{p: p, m: m}
			t.Run("reasoning", lt.reasoning)
			t.Run("roundtrip", lt.roundtrip)
			t.Run("image", lt.image)
			t.Run("tool_image", lt.toolImage)
			t.Run("truncate", lt.truncate)
			t.Run("cache", lt.cache)
		})
	}
}

type liveT struct {
	p llm.Provider
	m *llm.Model
}

func (lt *liveT) call(t *testing.T, req *llm.Request) llm.Done {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if req.Purpose == "" {
		req.Purpose = "probe"
	}
	start := time.Now()
	s, err := lt.p.Stream(ctx, lt.m, req)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer func() { _ = s.Close() }()
	var done *llm.Done
	var text strings.Builder
	for {
		ev, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v（已收到正文 %q）", err, text.String())
		}
		switch ev := ev.(type) {
		case llm.TextDelta:
			text.WriteString(ev.Text)
		case llm.Done:
			done = &ev
		}
	}
	if done == nil {
		t.Fatal("没有收到 Done")
	}
	if done.Message.TextOf() != text.String() {
		t.Errorf("Done 中的正文与增量拼接不一致：%q vs %q", done.Message.TextOf(), text.String())
	}
	t.Logf("%s stop=%s(%s) usage=%+v 耗时=%s 正文=%q", req.Reasoning, done.Stop, done.Raw, done.Usage,
		time.Since(start).Round(time.Millisecond), clip(text.String(), 120))
	return *done
}

func user(parts ...llm.Part) llm.Message { return llm.Message{Role: llm.RoleUser, Parts: parts} }

var sysShort = []llm.SystemBlock{{Text: "你是简洁的中文助手。"}}

// reasoning：四个推理档位的映射都能被接受（不报 400），并记录推理 token。
func (lt *liveT) reasoning(t *testing.T) {
	for _, level := range []llm.ReasoningLevel{llm.ReasoningOff, llm.ReasoningLow, llm.ReasoningMedium, llm.ReasoningHigh} {
		done := lt.call(t, &llm.Request{System: sysShort, Messages: []llm.Message{user(llm.Text("只回复一个字：好"))},
			MaxOutput: 4000, Reasoning: level})
		if done.Stop != llm.StopEnd || !strings.Contains(done.Message.TextOf(), "好") {
			t.Errorf("%s：stop=%s text=%q", level, done.Stop, done.Message.TextOf())
		}
	}
}

var weatherTool = llm.ToolSpec{Name: "get_weather", Description: "查询城市当前天气",
	Schema: jsonx.Value(`{"type":"object","properties":{"city":{"type":"string","description":"城市名"}},"required":["city"]}`)}

// roundtrip：并行工具调用 → 回填结果（同模型原样回传 Native，含推理签名）→ 最终回答。
func (lt *liveT) roundtrip(t *testing.T) {
	req := &llm.Request{System: []llm.SystemBlock{{Text: "你是简洁的中文助手。需要信息时调用工具，可以一次并行调用多个工具。"}},
		Tools:     []llm.ToolSpec{weatherTool},
		Messages:  []llm.Message{user(llm.Text("先用一句话说你要做什么，然后同时查询杭州和北京的天气，最后告诉我哪里更暖和。"))},
		MaxOutput: 4000, Reasoning: llm.ReasoningLow, CacheKey: "aint:live-roundtrip"}
	done := lt.call(t, req)
	calls := done.Message.ToolCalls()
	if done.Stop != llm.StopToolUse || len(calls) == 0 {
		t.Fatalf("没有工具调用：stop=%s", done.Stop)
	}
	t.Logf("工具调用 %d 个（并行：%v）", len(calls), len(calls) > 1)
	req.Messages = append(req.Messages, done.Message)
	results := llm.Message{Role: llm.RoleUser}
	for _, c := range calls {
		temp := "15℃，多云"
		if strings.Contains(string(c.Args), "杭州") {
			temp = "23℃，晴"
		}
		results.Parts = append(results.Parts, llm.Part{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: c.ID, Parts: []llm.Part{llm.Text(temp)}}})
	}
	req.Messages = append(req.Messages, results)
	for range 2 { // 只查了一个城市时允许再调一次
		done = lt.call(t, req)
		if done.Stop != llm.StopToolUse {
			break
		}
		req.Messages = append(req.Messages, done.Message)
		more := llm.Message{Role: llm.RoleUser}
		for _, c := range done.Message.ToolCalls() {
			temp := "15℃，多云"
			if strings.Contains(string(c.Args), "杭州") {
				temp = "23℃，晴"
			}
			more.Parts = append(more.Parts, llm.Part{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{CallID: c.ID, Parts: []llm.Part{llm.Text(temp)}}})
		}
		req.Messages = append(req.Messages, more)
	}
	if done.Stop != llm.StopEnd || !strings.Contains(done.Message.TextOf(), "杭州") {
		t.Fatalf("最终回答不对：stop=%s text=%q", done.Stop, done.Message.TextOf())
	}
}

// image：用户消息中的图片（程序生成的纯色块）。
func (lt *liveT) image(t *testing.T) {
	if !lt.m.Caps.Vision {
		t.Skip("模型不支持图片")
	}
	done := lt.call(t, &llm.Request{System: sysShort, Messages: []llm.Message{user(
		llm.Part{Kind: llm.PartImage, Image: &llm.ImageRef{Blob: "sha256:red", MIME: "image/png", Width: 64, Height: 64}},
		llm.Text("图片是什么颜色？只回答颜色。"))}, MaxOutput: 2000, Reasoning: llm.ReasoningLow})
	if !strings.Contains(done.Message.TextOf(), "红") {
		t.Fatalf("没有认出红色：%q", done.Message.TextOf())
	}
}

// tool_image：工具结果中带图片（看图工具）。
func (lt *liveT) toolImage(t *testing.T) {
	if !lt.m.Caps.Vision {
		t.Skip("模型不支持图片")
	}
	look := llm.ToolSpec{Name: "view_image", Description: "查看素材图片", Schema: jsonx.Value(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`)}
	req := &llm.Request{System: []llm.SystemBlock{{Text: "你是简洁的中文助手。需要看图时调用 view_image。"}}, Tools: []llm.ToolSpec{look},
		Messages: []llm.Message{user(llm.Text("请调用 view_image 查看素材 cover，然后告诉我它是什么颜色，只回答颜色。"))}, MaxOutput: 2000, Reasoning: llm.ReasoningLow}
	done := lt.call(t, req)
	calls := done.Message.ToolCalls()
	if len(calls) == 0 {
		t.Fatalf("没有调用看图工具：%q", done.Message.TextOf())
	}
	req.Messages = append(req.Messages, done.Message, llm.Message{Role: llm.RoleUser, Parts: []llm.Part{{Kind: llm.PartToolResult, ToolResult: &llm.ToolResult{
		CallID: calls[0].ID, Parts: []llm.Part{llm.Text("素材 cover："), {Kind: llm.PartImage, Image: &llm.ImageRef{Blob: "sha256:blue", MIME: "image/png"}}}}}}})
	done = lt.call(t, req)
	if !strings.Contains(done.Message.TextOf(), "蓝") {
		t.Fatalf("没有认出工具结果中的蓝色：%q", done.Message.TextOf())
	}
}

// truncate：输出上限截断 → 以 user 提示续写（不使用 assistant 预填）。
func (lt *liveT) truncate(t *testing.T) {
	req := &llm.Request{System: sysShort, Messages: []llm.Message{user(llm.Text("写一段约 600 字的雾港小镇描写，不要分点。"))},
		MaxOutput: 150, Reasoning: llm.ReasoningOff}
	done := lt.call(t, req)
	if done.Stop != llm.StopMaxTokens {
		t.Fatalf("期望截断：stop=%s", done.Stop)
	}
	req.Messages = append(req.Messages, done.Message, user(llm.Text("（输出被截断了。请从断处直接接着写，不要重复已写的内容，也不要加说明。）")))
	req.MaxOutput = 400
	done2 := lt.call(t, req)
	if done2.Message.TextOf() == "" {
		t.Fatalf("续写没有正文：stop=%s", done2.Stop)
	}
}

// cache：同一前缀连续两次请求，第二次应读到缓存（M0-2）。前缀约 3000 token；
// Gemini 隐式缓存在约 3.6k token 时实测不命中、约 11k token 时命中，所以 Gemini 用约 1 万 token 的前缀。
func (lt *liveT) cache(t *testing.T) {
	n := 60
	if lt.m.Caps.Reasoning == "level" { // Gemini
		n = 180
	}
	var long strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&long, "第 %d 条设定：雾港的第 %d 号码头在涨潮时会被雾吞没，守夜人用铜铃报时，铃声每隔 %d 分钟响一次，镇上的人据此判断潮水与来信的时辰。\n", i, i, i+3)
	}
	sys := []llm.SystemBlock{{Text: "你是互动小说的执笔者，回答尽量简短。"}, {Text: long.String(), CacheMark: true}}
	req := &llm.Request{System: sys, Messages: []llm.Message{user(llm.Text("第 7 号码头的铃声间隔是多少分钟？只回答数字。"))},
		MaxOutput: 2000, Reasoning: llm.ReasoningLow, CacheKey: "aint:live-cache", CacheTTL: llm.CacheShort}
	first := lt.call(t, req)
	time.Sleep(3 * time.Second) // 让缓存写入生效（DeepSeek 硬盘缓存需要一点时间）
	req.Messages = append(req.Messages, first.Message, user(llm.Text("第 12 号码头呢？只回答数字。")))
	second := lt.call(t, req)
	t.Logf("缓存：第一次 写入=%d 读取=%d；第二次 写入=%d 读取=%d（输入合计 %d）",
		first.Usage.CacheWrite, first.Usage.CacheRead, second.Usage.CacheWrite, second.Usage.CacheRead, second.Usage.InputTotal())
	if second.Usage.CacheRead == 0 {
		t.Errorf("第二次请求没有读到缓存（%s）", lt.m.Caps.Cache)
	}
}
