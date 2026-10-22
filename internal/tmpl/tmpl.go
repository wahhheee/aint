// Package tmpl 渲染作者编写的 Liquid 模板：开场、执笔指令、命令正文、面板（决策 D-08）。
//
// 作者模板是不可信输入（不变量"作品不执行"）：
//   - 只能读到调用方给的变量（state、args、input、turn……），不能访问文件与网络：
//     {% include %} / {% render %} 只能读到调用方提供的作品 ui/ 模板，不读宿主文件（render.go 的 partialStore）；
//   - 输入与输出都有上限（MaxSource、MaxVars、MaxOutput），输出超限时渲染立即中止；
//   - Go 的 Liquid 实现无法中断渲染：服务端在可终止的隔离工作进程里渲染（D-46，worker.go），
//     超时即杀掉该进程并重启，恶意模板最多拖慢一次渲染；
//   - 输出中的 HTML 一律由前端 DOMPurify 净化后再展示，本包不承担 XSS 防护。
//
// 两种实现共用 render.go 的渲染核心（变量先序列化为 JSON 再还原），渲染语义完全一致：
//
//	New()          进程内渲染：命令行工具与单元测试使用
//	NewIsolated()  工作进程渲染：服务（app.wire）使用
//
// 平台自己的提示词模板（可信）用 text/template，见 prompts 包；两者刻意分开，避免混用。
package tmpl

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

// 渲染限制（《02-技术方案》11.2）。
const (
	MaxSource = 64 << 10  // 模板源码
	MaxVars   = 256 << 10 // 序列化后的变量（状态较大的作品留出余量）
	MaxOutput = 64 << 10  // 渲染结果
	// MaxPartials 是随一次渲染提供的、可被 {% render %} 引用的模板合计大小（作品 ui/ 下的 .liquid 文件）。
	MaxPartials = 256 << 10
	// MaxRenderTime 是一次渲染的时限（工作进程：从发出请求到收到结果）。正常的面板渲染在 1 ms 以内；
	// 时限放宽是为了机器繁忙时不误杀，它的作用是让失控的模板在有限时间内被终止。
	MaxRenderTime = time.Second
)

// ErrKind 是渲染错误的类别。
type ErrKind string

const (
	ErrTemplate ErrKind = "template"  // 模板语法或渲染错误（作者可修正）
	ErrTooLarge ErrKind = "too_large" // 输入或输出超过上限
	ErrTimeout  ErrKind = "timeout"   // 渲染超时（工作进程已被终止）
	ErrWorker   ErrKind = "worker"    // 工作进程故障（启动失败、异常退出）
)

// Error 是渲染错误：模板名、行号（若有）与面向作者的说明。
type Error struct {
	Name string  `json:"name"`
	Line int     `json:"line,omitzero"`
	Msg  string  `json:"msg"`
	Kind ErrKind `json:"kind"`
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s 第 %d 行：%s", e.Name, e.Line, e.Msg)
	}
	return e.Name + "：" + e.Msg
}

// Engine 是 Liquid 渲染器。并发安全。
type Engine struct {
	r    *renderer // 进程内
	pool *pool     // 工作进程；非空时优先
}

// New 创建进程内渲染器。
func New() *Engine { return &Engine{r: newRenderer()} }

// Render 渲染模板。name 只用于错误信息（如 "ui/panels/notebook.liquid"）。返回的错误为 *Error。
// 模板中不能用 {% render %} 引用其他模板；需要时用 RenderWith。
func (e *Engine) Render(name, src string, vars map[string]any) (string, error) {
	return e.RenderWith(name, src, vars, nil)
}

// RenderWith 渲染模板，partials 是 {% render %} 可以引用的模板（作品 ui/ 下的 .liquid 文件：路径 → 源码，
// 见 work.Model.Partials）。返回的错误为 *Error。
func (e *Engine) RenderWith(name, src string, vars map[string]any, partials map[string]string) (string, error) {
	data, err := encodeVars(name, vars)
	if err != nil {
		return "", err
	}
	if err := checkInput(name, src, data); err != nil {
		return "", err
	}
	if err := checkPartials(name, partials); err != nil {
		return "", err
	}
	if e.pool != nil {
		return e.pool.render(name, src, data, partials)
	}
	return e.r.render(name, src, data, partials)
}

// Parse 只解析模板、不渲染（校验器检查 E-TEMPLATE）。返回的错误为 *Error，带行号。
func Parse(name, src string) error {
	if len(src) > MaxSource {
		return &Error{Name: name, Msg: fmt.Sprintf("模板超过 %d KiB（%s），请精简或拆分", MaxSource>>10, kib(len(src))), Kind: ErrTooLarge}
	}
	_, err := parseWith(parseEngine, name, src)
	return err
}

// parseEngine 只用于 Parse：解析不读取模板存储，所有调用共用一个引擎。
var parseEngine = newLiquid(partialStore(nil))

// Close 结束工作进程（服务退出时调用）；进程内渲染器无需关闭。
func (e *Engine) Close() {
	if e.pool != nil {
		e.pool.close()
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
