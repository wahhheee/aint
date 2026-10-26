package wire

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wahhheee/aint/internal/pkg/fsx"
)

// 原始报文追踪（--trace-llm）：每次调用写两个文件
//
//	<时间>-<序号>-<协议>-<用途>-<模型>.req.json   请求体原样字节（确定性 JSON，可直接用于前缀比对）
//	<时间>-<序号>-<协议>-<用途>-<模型>.resp.txt   请求行、脱敏请求头、响应状态与响应头、空行、响应体原样字节
//
// 响应体是边读边写的：流中途断开时文件里就是实际收到的部分，便于复现与制作回放夹具。

var (
	traceSeq   atomic.Int64
	unsafeName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	// secretHeaders 是需要脱敏的请求头（小写）。
	secretHeaders = map[string]bool{"authorization": true, "x-api-key": true, "x-goog-api-key": true, "api-key": true, "cookie": true}
)

type traceFile struct {
	f      *os.File
	path   string
	closed bool
}

func (e *Endpoint) startTrace(req *http.Request, body []byte, call Call) *traceFile {
	if e.TraceDir == "" {
		return nil
	}
	base := fmt.Sprintf("%s-%03d-%s-%s-%s", time.Now().Format("20060102-150405.000"), traceSeq.Add(1)%1000,
		e.Protocol, unsafeName.ReplaceAllString(call.Purpose, "_"), unsafeName.ReplaceAllString(call.Model, "_"))
	if err := os.MkdirAll(e.TraceDir, fsx.DirPerm); err != nil {
		slog.Warn("创建原始报文追踪目录失败", "err", err)
		return nil
	}
	if err := os.WriteFile(filepath.Join(e.TraceDir, base+".req.json"), body, fsx.PrivateFilePerm); err != nil {
		slog.Warn("写入原始请求追踪失败", "err", err)
		return nil
	}
	path := filepath.Join(e.TraceDir, base+".resp.txt")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fsx.PrivateFilePerm)
	if err != nil {
		slog.Warn("写入原始响应追踪失败", "err", err)
		return nil
	}
	t := &traceFile{f: f, path: path}
	t.printf("%s %s\n", req.Method, safeURL(req.URL))
	writeHeaders(f, req.Header)
	return t
}

func writeHeaders(w io.Writer, h http.Header) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := strings.Join(h[k], ", ")
		if sensitiveHeader(k) {
			v = "***"
		}
		_, _ = fmt.Fprintf(w, "%s: %s\n", k, v)
	}
}

func (t *traceFile) printf(format string, args ...any) {
	if t == nil {
		return
	}
	_, _ = fmt.Fprintf(t.f, format, args...)
}

func (t *traceFile) response(resp *http.Response, elapsed time.Duration) {
	if t == nil {
		return
	}
	t.printf("\n--- %s（首包 %s）\n", resp.Status, elapsed.Round(time.Millisecond))
	writeHeaders(t.f, resp.Header)
	t.printf("\n")
}

func (t *traceFile) fail(err error) {
	if t == nil {
		return
	}
	t.printf("\n--- 请求失败：%v\n", err)
	t.close()
}

func (t *traceFile) body(b []byte) {
	if t == nil {
		return
	}
	_, _ = t.f.Write(b)
}

func (t *traceFile) close() {
	if t == nil || t.closed {
		return
	}
	t.closed = true
	if err := t.f.Close(); err != nil {
		slog.Warn("关闭原始报文追踪文件失败", "file", t.path, "err", err)
	}
}

// wrap 返回边读边写追踪文件的响应体。
func (t *traceFile) wrap(rc io.ReadCloser) io.ReadCloser {
	if t == nil {
		return rc
	}
	return &teeBody{rc: rc, t: t}
}

type teeBody struct {
	rc io.ReadCloser
	t  *traceFile
}

func (b *teeBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		b.t.body(p[:n])
	}
	return n, err
}

func (b *teeBody) Close() error {
	err := b.rc.Close()
	b.t.close()
	return err
}
