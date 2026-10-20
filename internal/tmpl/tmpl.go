// Package tmpl 渲染作者编写的 Liquid 模板：开场（opening.md）、命令正文、面板（决策 D-08）。
//
// 作者模板是不可信输入：
//   - 只能读到调用方给的变量（state、args、input、turn、work、save），不能访问文件与网络；
//   - 渲染时长 ≤ 50 ms、输出 ≤ 64 KiB（《02-技术方案》11.2）；
//   - 输出中的 HTML 一律由前端 DOMPurify 净化后再展示，本包不承担 XSS 防护。
//
// 平台自己的提示词模板（可信）用 text/template，见 prompts 包；两者刻意分开，避免混用。
package tmpl

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/osteele/liquid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

const (
	MaxRenderTime = 50 * time.Millisecond
	MaxOutput     = 64 << 10
)

// Engine 是 Liquid 渲染器。并发安全。
type Engine struct {
	lq *liquid.Engine
}

// New 创建渲染器并注册附加过滤器。
func New() *Engine {
	e := liquid.NewEngine()
	e.RegisterFilter("markdown", func(s string) string { return Markdown(s) })
	e.RegisterFilter("percent", func(v, max float64) string {
		if max == 0 {
			return "0%"
		}
		return fmt.Sprintf("%.0f%%", v/max*100)
	})
	// TODO(M2-10)：asset 过滤器：素材引用 → /api/v1/assets/{hash}（需要作品素材索引）。
	e.RegisterFilter("asset", func(ref string) string { return "asset:" + ref })
	// TODO(M2-8)：{% render %} 只允许引用作品 ui/ 下的模板（RegisterTemplateStore）；for 循环 ≤ 1000 次。
	return &Engine{lq: e}
}

// Render 渲染模板。name 只用于错误信息（如 "ui/panels/notebook.liquid"）。
func (e *Engine) Render(name, src string, vars map[string]any) (string, error) {
	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := e.lq.ParseAndRenderString(src, vars)
		if err != nil {
			ch <- result{err: fmt.Errorf("%s：%w", name, err)}
			return
		}
		ch <- result{out: out}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return "", r.err
		}
		if len(r.out) > MaxOutput {
			return "", fmt.Errorf("%s：渲染结果超过 %d KiB", name, MaxOutput>>10)
		}
		return r.out, nil
	case <-time.After(MaxRenderTime):
		// TODO(M0-4)：Go 实现无法中断渲染，超时的 goroutine 会跑到结束；评估库能力后决定是否改为前端 LiquidJS。
		return "", fmt.Errorf("%s：渲染超时（> %s）", name, MaxRenderTime)
	}
}

// panelMarkdown 允许原始 HTML 透传：面板模板里混用 Markdown 与 <ui-*> 组件，
// 安全边界在前端的 DOMPurify 白名单（《02-技术方案》17）。
var panelMarkdown = goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()))

// Markdown 把面板 Markdown 渲染为 HTML（不做净化，见上）。
func Markdown(src string) string {
	var buf bytes.Buffer
	if err := panelMarkdown.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}
