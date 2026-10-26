package tmpl

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// 隔离工作进程（D-46）。
//
// 协议：标准输入 / 输出上逐行传 JSON。父进程每行一个请求 {"id", "name", "src", "vars", "partials"}，
// 工作进程每行一个结果 {"id", "out"} 或 {"id", "err": {...}}。一个工作进程同一时刻只处理一个请求；
// 父进程维护一个小进程池（PoolSize），进程按需启动。
//
// 故障处理：
//   - 超时（MaxRenderTime）：杀掉该进程，返回 ErrTimeout；下一次渲染时重新启动一个；
//   - 进程异常退出（如渲染库栈溢出）：返回 ErrWorker，同样在下一次渲染时重启；
//   - 启动失败：返回 ErrWorker（服务仍可运行，只是作者模板渲染不可用，日志中有原因）。

// PoolSize 是工作进程数：回合提交后的面板渲染与命令展开偶尔并发，两个足够。
const PoolSize = 2

// maxLine 是协议中单行的上限（源码 + 变量 + 模板集 + JSON 开销；JSON 转义最多让长度翻倍）。
const maxLine = MaxSource*2 + MaxVars*2 + MaxPartials*2 + 4<<10

type workerRequest struct {
	ID       uint64            `json:"id"`
	Name     string            `json:"name"`
	Src      string            `json:"src"`
	Vars     jsonx.Value       `json:"vars"`
	Partials map[string]string `json:"partials,omitzero"`
}

type workerResponse struct {
	ID  uint64 `json:"id"`
	Out string `json:"out,omitzero"`
	Err *Error `json:"err,omitzero"`
}

// ServeWorker 是工作进程的主循环：从 r 逐行读请求，渲染后把结果写到 w，直到 r 结束。
// 由隐藏子命令 aint __tmpl-worker 调用（cmd/aint）。
func ServeWorker(r io.Reader, w io.Writer) error {
	rd := newRenderer()
	in := bufio.NewReaderSize(r, 64<<10)
	out := bufio.NewWriter(w)
	for {
		line, err := readLine(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var req workerRequest
		if err := jsonx.Unmarshal(line, &req); err != nil {
			return fmt.Errorf("无法解析渲染请求: %w", err)
		}
		resp := workerResponse{ID: req.ID}
		text, rerr := rd.render(req.Name, req.Src, req.Vars, req.Partials)
		if rerr != nil {
			resp.Err = asError(req.Name, rerr)
		} else {
			resp.Out = text
		}
		if err := jsonx.MarshalWrite(out, resp); err != nil {
			return err
		}
		if err := out.WriteByte('\n'); err != nil {
			return err
		}
		if err := out.Flush(); err != nil {
			return err
		}
	}
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > maxLine {
			return nil, errors.New("渲染请求过大")
		}
		if err == nil {
			return bytes.TrimRight(buf, "\r\n"), nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(buf) > 0 {
			return bytes.TrimRight(buf, "\r\n"), nil
		}
		return nil, err
	}
}

func asError(name string, err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Name: name, Msg: err.Error(), Kind: ErrTemplate}
}

// ---------------------------------------------------------------------------
// 父进程：工作进程池

// NewIsolated 创建在工作进程中渲染的渲染器。newCmd 返回启动工作进程的命令（未启动），
// 通常是 exec.Command(os.Executable(), "__tmpl-worker")。
func NewIsolated(newCmd func() *exec.Cmd) *Engine {
	p := &pool{newCmd: newCmd, idle: make(chan *worker, PoolSize), timeout: MaxRenderTime}
	for range PoolSize {
		p.idle <- nil // 空位：用到时再启动
	}
	return &Engine{r: newRenderer(), pool: p}
}

type pool struct {
	newCmd  func() *exec.Cmd
	idle    chan *worker // 空闲的工作进程；nil 表示该位置的进程尚未启动或已终止
	timeout time.Duration

	mu     sync.Mutex
	live   map[*worker]struct{}
	closed bool
	seq    uint64
}

type worker struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	resp  chan workerResponse
	dead  chan struct{} // 进程的标准输出结束（退出或被杀）
	errs  *tailBuffer   // 标准错误的最后一段（排查启动失败）
}

func (p *pool) render(name, src string, vars []byte, partials map[string]string) (string, error) {
	w := <-p.idle
	ok := false
	defer func() {
		if !ok && w != nil {
			p.kill(w)
			w = nil
		}
		p.idle <- w
	}()
	if w == nil || !w.alive() {
		var err error
		if w, err = p.start(); err != nil {
			slog.Error("模板渲染工作进程启动失败", "err", err)
			return "", &Error{Name: name, Msg: "模板渲染进程启动失败（详见日志）", Kind: ErrWorker}
		}
	}

	p.mu.Lock()
	p.seq++
	id := p.seq
	p.mu.Unlock()
	line, err := jsonx.Marshal(workerRequest{ID: id, Name: name, Src: src, Vars: vars, Partials: partials})
	if err != nil {
		ok = true // 进程没有收到请求，仍然可用
		return "", &Error{Name: name, Msg: "渲染请求无法序列化：" + err.Error(), Kind: ErrTemplate}
	}
	if _, err := w.stdin.Write(append(line, '\n')); err != nil {
		return "", &Error{Name: name, Msg: "模板渲染进程不可用", Kind: ErrWorker}
	}

	timer := time.NewTimer(p.timeout)
	defer timer.Stop()
	for {
		select {
		case resp := <-w.resp:
			if resp.ID != id {
				continue // 不应发生：一个进程同一时刻只有一个请求
			}
			ok = true
			if resp.Err != nil {
				return "", resp.Err
			}
			return resp.Out, nil
		case <-w.dead:
			slog.Warn("模板渲染工作进程异常退出", "template", name, "stderr", w.errs.String())
			return "", &Error{Name: name, Msg: "模板渲染失败（渲染进程异常退出）", Kind: ErrWorker}
		case <-timer.C:
			slog.Warn("模板渲染超时，已终止工作进程", "template", name, "timeout", p.timeout)
			return "", &Error{Name: name, Msg: fmt.Sprintf("渲染超时（超过 %s），模板中可能有过大的循环", p.timeout), Kind: ErrTimeout}
		}
	}
}

func (p *pool) start() (*worker, error) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return nil, errors.New("渲染器已关闭")
	}
	cmd := p.newCmd()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	w := &worker{cmd: cmd, stdin: stdin, resp: make(chan workerResponse, 1), dead: make(chan struct{}), errs: &tailBuffer{max: 4 << 10}}
	cmd.Stderr = w.errs
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		defer close(w.dead)
		rd := bufio.NewReaderSize(stdout, 64<<10)
		for {
			line, err := readLine(rd)
			if err != nil {
				return
			}
			var resp workerResponse
			if jsonx.Unmarshal(line, &resp) != nil {
				return
			}
			w.resp <- resp
		}
	}()
	go func() { _ = cmd.Wait() }() // 回收进程；退出原因体现在 dead 与 stderr 中
	p.mu.Lock()
	if p.live == nil {
		p.live = map[*worker]struct{}{}
	}
	p.live[w] = struct{}{}
	p.mu.Unlock()
	return w, nil
}

func (w *worker) alive() bool {
	select {
	case <-w.dead:
		return false
	default:
		return true
	}
}

func (p *pool) kill(w *worker) {
	_ = w.cmd.Process.Kill() // 进程可能已经退出；错误无需处理
	_ = w.stdin.Close()
	p.mu.Lock()
	delete(p.live, w)
	p.mu.Unlock()
}

func (p *pool) close() {
	p.mu.Lock()
	p.closed = true
	live := p.live
	p.live = nil
	p.mu.Unlock()
	for w := range live {
		_ = w.stdin.Close() // 工作进程读到 EOF 后自行退出
		select {
		case <-w.dead:
		case <-time.After(time.Second):
			_ = w.cmd.Process.Kill()
		}
	}
}

// tailBuffer 只保留最后 max 字节（工作进程的标准错误）。并发安全。
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
