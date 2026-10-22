package tmpl

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// 测试中的工作进程是测试二进制自身：TestMain 见到环境变量后进入工作进程循环。
const envWorker = "YANYI_TMPL_TEST_WORKER"

func TestMain(m *testing.M) {
	switch os.Getenv(envWorker) {
	case "serve":
		if err := ServeWorker(os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "crash":
		os.Exit(3) // 模拟启动后立即异常退出的工作进程
	}
	os.Exit(m.Run())
}

func testWorkerCmd(t *testing.T, mode string) func() *exec.Cmd {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return func() *exec.Cmd {
		cmd := exec.Command(exe) //nolint:noctx // 工作进程的生命周期由渲染器管理
		cmd.Env = append(os.Environ(), envWorker+"="+mode)
		return cmd
	}
}

func isolated(t *testing.T) *Engine {
	t.Helper()
	e := NewIsolated(testWorkerCmd(t, "serve"))
	t.Cleanup(e.Close)
	return e
}

// engines 返回两种实现：同一组用例对两者都跑，渲染语义必须一致。
func engines(t *testing.T) map[string]*Engine {
	return map[string]*Engine{"进程内": New(), "工作进程": isolated(t)}
}

func kindOf(err error) ErrKind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

func TestRenderSemantics(t *testing.T) {
	vars := map[string]any{
		"state": map[string]any{"day": 3.0, "hp": 7.5, "name": "沈遥", "flags": map[string]any{"c": true, "a": false, "b": true}},
		"turn":  map[string]any{"no": 12},
	}
	for name, e := range engines(t) {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct{ src, want string }{
				{`第 {{ turn.no }} 回合，{{ state.name }}`, "第 12 回合，沈遥"},
				{`{% for i in (1..state.day) %}{{ i }}{% endfor %}`, "123"}, // 状态中的整数可以用作范围
				{`{{ state.hp | plus: 1 }}`, "8.5"},                         // 非整数保持浮点
				{`{% for f in state.flags %}{{ f[0] }}{% endfor %}`, "abc"}, // 对象按键排序遍历
				{`{{ "**粗**" | markdown }}`, "<p><strong>粗</strong></p>"},   // 附加过滤器
				{`{{ 3 | percent: 4 }}`, "75%"},
			} {
				got, err := e.Render("t", tc.src, vars)
				if err != nil || got != tc.want {
					t.Errorf("%s：得到 %q（%v），期望 %q", tc.src, got, err, tc.want)
				}
			}
		})
	}
}

func TestRenderErrors(t *testing.T) {
	for name, e := range engines(t) {
		t.Run(name, func(t *testing.T) {
			_, err := e.Render("ui/panels/a.liquid", "第一行\n{% if %}", nil)
			var te *Error
			if !errors.As(err, &te) || te.Kind != ErrTemplate || te.Line != 2 || !strings.HasPrefix(te.Error(), "ui/panels/a.liquid 第 2 行：") {
				t.Fatalf("语法错误应带模板名与行号：%v", err)
			}
			// include / render 不能读宿主文件
			for _, src := range []string{`{% include "tmpl.go" %}`, `{% render "../../go.mod" %}`} {
				if out, err := e.Render("t", src, nil); err == nil || strings.Contains(out, "package") || strings.Contains(out, "module") {
					t.Fatalf("%s 不应读到文件：%q %v", src, out, err)
				}
			}
			// 输出超限立即中止
			if _, err := e.Render("t", `{% for i in (1..100000) %}{{ "0123456789" }}{% endfor %}`, nil); kindOf(err) != ErrTooLarge {
				t.Fatalf("输出超限：%v", err)
			}
			// 输入超限
			if _, err := e.Render("t", strings.Repeat("x", MaxSource+1), nil); kindOf(err) != ErrTooLarge {
				t.Fatalf("源码超限：%v", err)
			}
			if _, err := e.Render("t", "x", map[string]any{"big": strings.Repeat("y", MaxVars)}); kindOf(err) != ErrTooLarge {
				t.Fatalf("变量超限：%v", err)
			}
		})
	}
}

// 失控的模板：超时后杀掉工作进程，返回超时错误；之后的渲染正常（自动重启）。
func TestIsolatedTimeout(t *testing.T) {
	e := isolated(t)
	e.pool.timeout = 300 * time.Millisecond
	start := time.Now()
	_, err := e.Render("ui/panels/loop.liquid", `{% for i in (1..1000000) %}{% for j in (1..1000000) %}{% endfor %}{% endfor %}`, nil)
	if kindOf(err) != ErrTimeout {
		t.Fatalf("应超时：%v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("超时后应立即返回，实际 %s", d)
	}
	for range 3 { // 两个进程位都能恢复
		if out, err := e.Render("t", "{{ 1 | plus: 1 }}", nil); err != nil || out != "2" {
			t.Fatalf("超时之后的渲染：%q %v", out, err)
		}
	}
}

// 工作进程异常退出：返回 ErrWorker，不拖垮服务。
func TestIsolatedCrash(t *testing.T) {
	e := NewIsolated(testWorkerCmd(t, "crash"))
	t.Cleanup(e.Close)
	if _, err := e.Render("t", "x", nil); kindOf(err) != ErrWorker {
		t.Fatalf("应报工作进程故障：%v", err)
	}
}

func TestIsolatedConcurrent(t *testing.T) {
	e := isolated(t)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := range 20 {
		wg.Go(func() {
			out, err := e.Render("t", "{{ n | times: 2 }}", map[string]any{"n": i})
			if err != nil {
				errs <- err
				return
			}
			if want := strconv.Itoa(i * 2); out != want {
				errs <- errors.New("结果串了：" + out + " ≠ " + want)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// 实测开销（写入 docs/notes/api-verification.md 的 M0-4 结论）：典型面板模板的单次渲染耗时。
func TestIsolatedLatency(t *testing.T) {
	src := `## {{ state.name }}
- 第 {{ turn.no }} 天
{% for c in state.clues %}- {{ c }}
{% endfor %}<ui-bar value="{{ state.hp }}" max="10"></ui-bar>`
	vars := map[string]any{"state": map[string]any{"name": "沈遥", "hp": 7, "clues": []any{"信封", "铜铃", "旧照片"}}, "turn": map[string]any{"no": 3}}
	measure := func(e *Engine) time.Duration {
		if _, err := e.Render("p", src, vars); err != nil { // 预热（启动进程、解析缓存）
			t.Fatal(err)
		}
		const n = 50
		start := time.Now()
		for range n {
			if _, err := e.Render("p", src, vars); err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(start) / n
	}
	in := measure(New())
	start := time.Now()
	iso := isolated(t)
	if _, err := iso.Render("p", "x", nil); err != nil {
		t.Fatal(err)
	}
	startup := time.Since(start)
	out := measure(iso)
	t.Logf("单次渲染：进程内 %s，工作进程 %s；工作进程启动 %s", in, out, startup)
}
