package tmpl

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/osteele/liquid"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// renderer 是渲染核心：进程内实现（New）与工作进程（ServeWorker）共用这一份代码——
// 同样的解析、变量规范化、限制检查与错误格式，两种实现的渲染语义因此一致。并发安全。
type renderer struct {
	lq *liquid.Engine

	mu    sync.Mutex
	cache map[[sha256.Size]byte]*liquid.Template // 已解析的模板，按源码哈希
}

// maxCachedTemplates 是解析缓存的条目上限；满了整体清空（作品的模板数远小于此）。
const maxCachedTemplates = 256

// maxErrorMsg 是错误说明的长度上限（Liquid 的错误信息会带上出错处的源码）。
const maxErrorMsg = 2 << 10

func newRenderer() *renderer {
	return &renderer{lq: newLiquid(partialStore(nil)), cache: map[[sha256.Size]byte]*liquid.Template{}}
}

// newLiquid 创建 Liquid 引擎并注册附加过滤器。store 决定 {% render %} 能读到什么：
// 默认的 FileTemplateStore 会读取当前目录下的任意文件（实测可读到源码文件），所以总是换成 partialStore。
//
// 循环次数不单独计数：失控的循环由渲染时限（MaxRenderTime）与输出上限（MaxOutput）终止。
func newLiquid(store partialStore) *liquid.Engine {
	e := liquid.NewEngine()
	e.RegisterTemplateStore(store)
	e.RegisterFilter("markdown", Markdown)
	e.RegisterFilter("percent", func(v, total float64) string {
		if total == 0 {
			return "0%"
		}
		return fmt.Sprintf("%.0f%%", v/total*100)
	})
	// asset：素材引用写成 asset: 方案，由前端映射为素材地址（面板 HTML 中的 <img src>、ui-image）。
	e.RegisterFilter("asset", func(ref string) string { return "asset:" + ref })
	return e
}

// partialStore 是 {% render %} 可读的模板：作品 ui/ 下的 .liquid 文件（路径 → 源码），由调用方随渲染请求提供。
// 不碰宿主文件系统；{% include %} 同样只能读到这些模板。
type partialStore map[string]string

// ReadTemplate 按名称找模板：'panels/_row'、'ui/panels/_row.liquid' 都指向 ui/panels/_row.liquid。
// 名称由渲染库相对于当前模板所在目录拼接（Windows 上是反斜杠），这里统一为 /。
func (s partialStore) ReadTemplate(name string) ([]byte, error) {
	if p, ok := ResolvePartial(name, func(p string) bool { _, ok := s[p]; return ok }); ok {
		return []byte(s[p]), nil
	}
	if len(s) == 0 {
		return nil, fmt.Errorf("这里不能用 render 引用其他模板（%s）", name)
	}
	return nil, fmt.Errorf("找不到模板 %s：render 只能引用作品 ui/ 下的 .liquid 文件", name)
}

// ResolvePartial 把 {% render %} 的名称解析为作品中的路径（ui/ 下的 .liquid 文件）；exists 判断路径是否存在。
// 校验器与渲染共用这一规则。
func ResolvePartial(name string, exists func(string) bool) (string, bool) {
	n := strings.TrimPrefix(strings.ReplaceAll(name, `\`, "/"), "./")
	if n == "" || strings.Contains(n, "..") || strings.HasPrefix(n, "/") {
		return "", false
	}
	for _, c := range []string{n, n + ".liquid", "ui/" + n, "ui/" + n + ".liquid"} {
		if strings.HasPrefix(c, "ui/") && strings.HasSuffix(c, ".liquid") && exists(c) {
			return c, true
		}
	}
	return "", false
}

// render 渲染一次。vars 是序列化后的变量（JSON 对象，由 encodeVars 产生）；partials 是 {% render %} 可读的模板。
// 返回的错误一律是 *Error。
func (r *renderer) render(name, src string, vars []byte, partials map[string]string) (out string, err error) {
	if err := checkInput(name, src, vars); err != nil {
		return "", err
	}
	if err := checkPartials(name, partials); err != nil {
		return "", err
	}
	data, err := decodeVars(name, vars)
	if err != nil {
		return "", err
	}
	var tpl *liquid.Template
	if len(partials) == 0 {
		tpl, err = r.parse(name, src)
	} else {
		// 模板存储属于引擎：带模板集的渲染用一个临时引擎（创建只需几微秒），不进解析缓存
		tpl, err = parseWith(newLiquid(partials), name, src)
	}
	if err != nil {
		return "", err
	}

	w := &limitWriter{max: MaxOutput}
	defer func() {
		// 渲染库在某些模板上会 panic（如 {% assign forloop = 1 %}{% cycle 'a' %}），进程内渲染时会拖垮整个服务。
		if p := recover(); p != nil {
			out = ""
			if w.exceeded { // tablerow 写入失败时直接 panic
				err = outputTooLarge(name)
				return
			}
			err = &Error{Name: name, Msg: clip(fmt.Sprintf("渲染失败：%v", p)), Kind: ErrTemplate}
		}
	}()
	if lerr := tpl.FRender(w, data); lerr != nil {
		if w.exceeded {
			return "", outputTooLarge(name)
		}
		return "", &Error{Name: name, Line: lerr.LineNumber(), Msg: liquidMessage(lerr), Kind: ErrTemplate}
	}
	// 切片类过滤器可能切开多字节字符；输出要能放进 JSON（工作进程回传）与 SSE。
	return strings.ToValidUTF8(w.buf.String(), "�"), nil
}

func (r *renderer) parse(name, src string) (*liquid.Template, error) {
	key := sha256.Sum256([]byte(src))
	r.mu.Lock()
	tpl, ok := r.cache[key]
	r.mu.Unlock()
	if ok {
		return tpl, nil
	}
	tpl, err := parseWith(r.lq, name, src)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if len(r.cache) >= maxCachedTemplates {
		clear(r.cache)
	}
	r.cache[key] = tpl
	r.mu.Unlock()
	return tpl, nil
}

// parseWith 用指定引擎解析模板。路径留空：错误信息带出错处的源码而不是路径；行号从 1 起。
func parseWith(lq *liquid.Engine, name, src string) (*liquid.Template, error) {
	tpl, perr := lq.ParseTemplateLocation([]byte(src), "", 1)
	if perr != nil {
		return nil, &Error{Name: name, Line: perr.LineNumber(), Msg: liquidMessage(perr), Kind: ErrTemplate}
	}
	return tpl, nil
}

// checkPartials 检查随渲染提供的模板集的大小（单个模板与合计）。
func checkPartials(name string, partials map[string]string) error {
	total := 0
	for p, src := range partials {
		if len(src) > MaxSource {
			return &Error{Name: name, Msg: fmt.Sprintf("被引用的模板 %s 超过 %d KiB", p, MaxSource>>10), Kind: ErrTooLarge}
		}
		total += len(src)
	}
	if total > MaxPartials {
		return &Error{Name: name, Msg: fmt.Sprintf("ui/ 下的模板合计超过 %d KiB（%s），请精简", MaxPartials>>10, kib(total)), Kind: ErrTooLarge}
	}
	return nil
}

// checkInput 检查模板源码与变量的大小。父进程发送前、工作进程收到后各查一次。
func checkInput(name, src string, vars []byte) error {
	if len(src) > MaxSource {
		return &Error{Name: name, Msg: fmt.Sprintf("模板超过 %d KiB（%s），请精简或拆分", MaxSource>>10, kib(len(src))), Kind: ErrTooLarge}
	}
	if len(vars) > MaxVars {
		return &Error{Name: name, Msg: fmt.Sprintf("模板变量超过 %d KiB（%s），状态等数据过大", MaxVars>>10, kib(len(vars))), Kind: ErrTooLarge}
	}
	return nil
}

// encodeVars 把变量序列化为 JSON：工作进程靠它传递变量；进程内实现也走同一步，
// 两种实现看到的变量（数字类型、映射类型）因此完全一样。
func encodeVars(name string, vars map[string]any) ([]byte, error) {
	b, err := jsonx.Marshal(vars)
	if err != nil {
		return nil, &Error{Name: name, Msg: clip("模板变量无法序列化：" + err.Error()), Kind: ErrTemplate}
	}
	return b, nil
}

// decodeVars 还原变量并规范化（见 normalize）。
func decodeVars(name string, b []byte) (map[string]any, error) {
	var m map[string]any
	if err := jsonx.Unmarshal(b, &m); err != nil {
		return nil, &Error{Name: name, Msg: clip("模板变量无法解析：" + err.Error()), Kind: ErrTemplate}
	}
	for k, v := range m {
		m[k] = normalize(v)
	}
	return m, nil
}

// normalize 把 JSON 解出的值调整为模板好用的形式：
//   - 整数值的数字转为 int：状态里的数字都是 float64，而 Liquid 的 (1..n)、limit、offset 只接受整数；
//     其余运算对两者结果相同（实测 divided_by、plus、比较等）；
//   - 对象转为 object：for 循环按键排序遍历，同一状态总是渲染出同样的结果。
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalize(e)
		}
		return object(x)
	case []any:
		for i, e := range x {
			x[i] = normalize(e)
		}
		return x
	case float64:
		if x == math.Trunc(x) && math.Abs(x) <= 1<<53 {
			return int(x)
		}
		return x
	default:
		return v
	}
}

// object 是模板中的 JSON 对象。属性访问与 map[string]any 相同；实现了 Liquid 的迭代接口
// （Len / Index），for 循环按键排序得到 [键, 值]——Liquid 对普通 map 按 Go 的随机顺序遍历，
// 同一面板每次渲染的顺序都可能不同。
type object map[string]any

func (o object) Len() int { return len(o) }

// Index 返回排序后第 i 个 [键, 值]。每次调用都排序：状态中的映射通常只有几十个键。
func (o object) Index(i int) any {
	k := slices.Sorted(maps.Keys(o))[i]
	return []any{k, o[k]}
}

// limitWriter 收集输出，超过上限立即返回错误（渲染随之中止，不必等到渲染完）。
type limitWriter struct {
	buf      bytes.Buffer
	max      int
	exceeded bool
}

var errOutputLimit = errors.New("输出超过上限")

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		w.exceeded = true
		return 0, errOutputLimit
	}
	return w.buf.Write(p)
}

func outputTooLarge(name string) *Error {
	return &Error{Name: name, Msg: fmt.Sprintf("渲染结果超过 %d KiB，已中止", MaxOutput>>10), Kind: ErrTooLarge}
}

// liquidMessage 去掉 Liquid 错误的 "Liquid error (line N): " 前缀（行号由 Error 自己表达）。
func liquidMessage(err error) string {
	msg := err.Error()
	if rest, ok := strings.CutPrefix(msg, "Liquid error"); ok {
		if _, after, found := strings.Cut(rest, ": "); found {
			msg = after
		}
	}
	return clip(msg)
}

func clip(s string) string {
	if len(s) <= maxErrorMsg {
		return s
	}
	return strings.ToValidUTF8(s[:maxErrorMsg], "") + "…"
}

func kib(n int) string {
	return fmt.Sprintf("%.1f KiB", float64(n)/1024)
}
