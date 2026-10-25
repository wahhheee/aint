package runtime

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/ids"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/tools"
	"github.com/wahhheee/yanyi/internal/vfs"
)

// 调试只在进程内保存：总计至多 100 次调用、64 MiB，单次至多 8 MiB。
// 限额包括报文、结构化内容与 VFS 快照；淘汰或截断都会显式报告。
const (
	debugMaxCalls  = 100
	debugMaxBytes  = 64 << 20
	debugCallBytes = 8 << 20
)

type DebugSummary struct {
	ID         string       `json:"id"`
	TurnID     string       `json:"turn_id"`
	TurnNo     int          `json:"turn_no"`
	Model      string       `json:"model"`
	Protocol   llm.Protocol `json:"protocol"`
	Purpose    string       `json:"purpose"`
	Started    time.Time    `json:"started"`
	DurationMS int64        `json:"duration_ms"`
	Status     string       `json:"status"`
	Stop       string       `json:"stop"`
	Error      string       `json:"error"`
	Usage      llm.Usage    `json:"usage"`
	ToolCount  int          `json:"tool_count"`
	HTTPCount  int          `json:"http_count"`
	Revision   int          `json:"revision"`
	Truncated  bool         `json:"truncated"`
}

type DebugStatus struct {
	Enabled bool           `json:"enabled"`
	Calls   []DebugSummary `json:"calls"`
	Dropped int            `json:"dropped"`
}

type DebugStep struct {
	Kind string        `json:"kind"`
	AtMS int64         `json:"at_ms"`
	Text string        `json:"text"`
	Call *llm.ToolCall `json:"call,omitzero"`
}

type DebugHTTP struct {
	Method          string      `json:"method"`
	URL             string      `json:"url"`
	RequestHeaders  http.Header `json:"request_headers"`
	RequestBody     string      `json:"request_body"`
	Status          int         `json:"status"`
	ResponseHeaders http.Header `json:"response_headers"`
	ResponseBody    string      `json:"response_body"`
	HeaderMS        int64       `json:"header_ms"`
	DurationMS      int64       `json:"duration_ms"`
	Error           string      `json:"error"`
	Complete        bool        `json:"complete"`
	Truncated       bool        `json:"truncated"`
	body            *bytes.Buffer
}

type DebugFile struct {
	Path    string `json:"path"`
	Size    int    `json:"size"`
	Summary string `json:"summary"`
	Blob    string `json:"blob"`
}

type DebugSnapshot struct {
	State jsonx.Value `json:"state"`
	Files []DebugFile `json:"files"`
	Roots []string    `json:"roots"`
	Error string      `json:"error"`
	fs    *vfs.Table
}

type DebugCall struct {
	DebugSummary
	Request jsonx.Value        `json:"request"`
	Steps   []DebugStep        `json:"steps"`
	Tools   []tools.CallRecord `json:"tools"`
	Results []llm.ToolResult   `json:"results"`
	HTTP    []*DebugHTTP       `json:"http"`
	Before  *DebugSnapshot     `json:"before"`
	After   *DebugSnapshot     `json:"after"`
}

type debugStore struct {
	mu      sync.Mutex
	enabled map[string]bool
	dropped map[string]int
	calls   []*debugRecorder
	bytes   int
}

type debugRecorder struct {
	store    *debugStore
	saveID   string
	data     DebugCall
	size     int
	detached bool
	images   map[string]bool
}

// Debug 返回存档的调试开关与轻量调用列表。详情单独获取，避免反复传输整个上下文。
func (m *Manager) Debug(saveID string) DebugStatus {
	s := &m.debug
	s.mu.Lock()
	defer s.mu.Unlock()
	out := DebugStatus{Enabled: s.enabled[saveID], Dropped: s.dropped[saveID], Calls: []DebugSummary{}}
	for _, r := range s.calls {
		if r.saveID == saveID {
			out.Calls = append(out.Calls, r.data.DebugSummary)
		}
	}
	return out
}

// SetDebug 决定之后开始的调用是否记录；已开始的调用继续完成记录。
func (m *Manager) SetDebug(saveID string, enabled bool) {
	s := &m.debug
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled == nil {
		s.enabled = map[string]bool{}
	}
	if enabled {
		s.enabled[saveID] = true
	} else {
		delete(s.enabled, saveID)
	}
}

func (m *Manager) ClearDebug(saveID string) {
	s := &m.debug
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = slices.DeleteFunc(s.calls, func(r *debugRecorder) bool {
		if r.saveID != saveID {
			return false
		}
		r.detached = true
		s.bytes -= r.size
		return true
	})
	delete(s.dropped, saveID)
}

// DebugCall 返回与运行中数据分离的副本。
func (m *Manager) DebugCall(saveID, id string) (*DebugCall, error) {
	s := &m.debug
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.calls {
		if r.saveID == saveID && r.data.ID == id {
			data := r.data
			data.HTTP = make([]*DebugHTTP, len(r.data.HTTP))
			for i, h := range r.data.HTTP {
				copyHTTP := *h
				copyHTTP.ResponseBody = strings.ToValidUTF8(h.body.String(), "�")
				data.HTTP[i] = &copyHTTP
			}
			var out DebugCall
			if err := jsonx.Unmarshal(jsonx.MustMarshal(data), &out); err != nil {
				return nil, err
			}
			return &out, nil
		}
	}
	return nil, errs.NotFound("调试记录已清除或超出保留范围")
}

type DebugFileContent struct {
	Path       string        `json:"path"`
	Text       string        `json:"text"`
	FromChar   int           `json:"from_char"`
	ToChar     int           `json:"to_char"`
	TotalChars int           `json:"total_chars"`
	Image      *llm.ImageRef `json:"image,omitzero"`
}

func (m *Manager) DebugFile(ctx context.Context, saveID, id, phase, path string, offset int) (*DebugFileContent, error) {
	if phase != "before" && phase != "after" {
		return nil, errs.Invalid("快照必须是 before 或 after")
	}
	var fs *vfs.Table
	m.debug.mu.Lock()
	for _, r := range m.debug.calls {
		if r.saveID != saveID || r.data.ID != id {
			continue
		}
		snap := r.data.Before
		if phase == "after" {
			snap = r.data.After
		}
		if snap != nil {
			fs = snap.fs
		}
	}
	m.debug.mu.Unlock()
	if fs == nil {
		return nil, errs.NotFound("这次调用没有可用的 VFS 快照")
	}
	res, err := fs.Read(ctx, path, 1, 1)
	if err != nil {
		return nil, errs.Invalid("无法读取虚拟文件：%v", err)
	}
	out := &DebugFileContent{Path: res.Path}
	if res.Image != nil {
		out.Image = &llm.ImageRef{Blob: res.Image.Blob, MIME: res.Image.MIME}
		return out, nil
	}
	b, err := fs.ReadFile(ctx, path)
	if err != nil {
		return nil, err
	}
	runes := []rune(string(b))
	if offset < 1 || offset > max(1, len(runes)) {
		return nil, errs.Invalid("超出文件内容范围")
	}
	end := min(len(runes), offset-1+20000)
	out.FromChar, out.ToChar, out.TotalChars = min(offset, len(runes)), end, len(runes)
	out.Text = string(runes[offset-1 : end])
	return out, nil
}

// DebugImage 仅提供这次调用上下文或工具结果实际引用过的图片，包括模型使用的派生图。
func (m *Manager) DebugImage(ctx context.Context, saveID, id, hash string) ([]byte, error) {
	allowed := false
	m.debug.mu.Lock()
	for _, r := range m.debug.calls {
		if r.saveID == saveID && r.data.ID == id {
			allowed = r.images[hash]
			break
		}
	}
	m.debug.mu.Unlock()
	if !allowed {
		return nil, errs.NotFound("这次调用没有引用该图片")
	}
	return m.d.Saves.Blob(ctx, hash)
}

func (m *Manager) debugStart(ctx context.Context, saveID, turnID string, turnNo int, p llm.Provider, model *llm.Model, req *llm.Request, fs *vfs.Table, state map[string]any) *debugRecorder {
	s := &m.debug
	s.mu.Lock()
	if !s.enabled[saveID] {
		s.mu.Unlock()
		return nil
	}
	r := &debugRecorder{store: s, saveID: saveID, data: DebugCall{DebugSummary: DebugSummary{
		ID: ids.New(), TurnID: turnID, TurnNo: turnNo, Model: model.Name, Protocol: p.Protocol(), Purpose: req.Purpose, Started: time.Now(), Status: "running",
	}, Steps: []DebugStep{}, Tools: []tools.CallRecord{}, Results: []llm.ToolResult{}, HTTP: []*DebugHTTP{}}}
	s.calls = append(s.calls, r)
	r.trim()
	b := jsonx.MustMarshal(req)
	if r.reserve(len(b)) {
		r.data.Request = b
		for _, msg := range req.Messages {
			r.rememberImages(msg.Parts)
		}
	}
	s.mu.Unlock()
	r.snapshot(ctx, fs, state, false)
	return r
}

// reserve 与 trim 由持有 store.mu 的调用方使用。
func (r *debugRecorder) reserve(n int) bool {
	if r.detached {
		return false
	}
	if r.size+n > debugCallBytes {
		r.data.Truncated = true
		return false
	}
	r.size += n
	r.store.bytes += n
	r.trim()
	return true
}

func (r *debugRecorder) trim() {
	s := r.store
	for len(s.calls) > debugMaxCalls || s.bytes > debugMaxBytes {
		i := 0
		if s.calls[i] == r {
			i++
		}
		old := s.calls[i]
		old.detached = true
		s.bytes -= old.size
		if s.dropped == nil {
			s.dropped = map[string]int{}
		}
		s.dropped[old.saveID]++
		s.calls = slices.Delete(s.calls, i, i+1)
	}
}

func (r *debugRecorder) text(text string) string {
	n := min(len(text), max(0, debugCallBytes-r.size))
	if n < len(text) {
		r.data.Truncated = true
	}
	if !r.reserve(n) {
		return ""
	}
	return strings.ToValidUTF8(strings.Clone(text[:n]), "�")
}

func (r *debugRecorder) update(fn func()) {
	if r == nil {
		return
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if r.detached {
		return
	}
	fn()
	r.data.Revision++
}

func (r *debugRecorder) Event(ev llm.Event) {
	r.update(func() {
		kind, text := "", ""
		switch e := ev.(type) {
		case llm.TextDelta:
			kind, text = "text", e.Text
		case llm.ReasoningSummaryDelta:
			kind, text = "reasoning", e.Text
		case llm.ToolCallStart:
			if r.reserve(len(e.ID) + len(e.Name) + 128) {
				r.data.Steps = append(r.data.Steps, DebugStep{Kind: "tool", AtMS: time.Since(r.data.Started).Milliseconds(), Call: &llm.ToolCall{ID: e.ID, Name: e.Name}})
			}
		case llm.ToolCallDone:
			r.toolCall(llm.ToolCall(e))
		case llm.Done:
			r.data.Stop, r.data.Usage = string(e.Stop), e.Usage
			if !slices.ContainsFunc(r.data.Steps, func(s DebugStep) bool { return s.Kind == "text" }) {
				r.appendText("text", e.Message.TextOf())
			}
			for _, c := range e.Message.ToolCalls() {
				r.toolCall(c)
			}
			// 某些协议只在完整消息中给出可读推理。
			if !slices.ContainsFunc(r.data.Steps, func(s DebugStep) bool { return s.Kind == "reasoning" }) {
				for _, p := range e.Message.Parts {
					if p.Kind == llm.PartReasoning {
						kind, text = "reasoning", text+p.Text
					}
				}
			}
		}
		r.appendText(kind, text)
	})
}

func (r *debugRecorder) appendText(kind, text string) {
	if text == "" {
		return
	}
	n := len(r.data.Steps)
	if n > 0 && r.data.Steps[n-1].Kind == kind {
		r.data.Steps[n-1].Text += r.text(text)
	} else if r.reserve(128) {
		r.data.Steps = append(r.data.Steps, DebugStep{Kind: kind, Text: r.text(text), AtMS: time.Since(r.data.Started).Milliseconds()})
	}
}

func (r *debugRecorder) toolCall(c llm.ToolCall) {
	for i := range r.data.Steps {
		s := &r.data.Steps[i]
		if s.Call == nil || s.Call.ID != c.ID {
			continue
		}
		if len(s.Call.Args) == 0 && r.reserve(len(c.Args)) {
			s.Call.Args = slices.Clone(c.Args)
		}
		return
	}
	if r.reserve(len(c.ID) + len(c.Name) + len(c.Args) + 128) {
		c.Args = slices.Clone(c.Args)
		r.data.Steps = append(r.data.Steps, DebugStep{Kind: "tool", Call: &c, AtMS: time.Since(r.data.Started).Milliseconds()})
	}
}

func (r *debugRecorder) Finish(err error) {
	r.update(func() {
		r.data.DurationMS = time.Since(r.data.Started).Milliseconds()
		r.data.Status = "complete"
		if err != nil {
			r.data.Status, r.data.Error = "error", r.text(err.Error())
			if u := llm.UsageOf(err); u != (llm.Usage{}) {
				r.data.Usage = u
			}
		}
	})
}

func (r *debugRecorder) snapshot(ctx context.Context, fs *vfs.Table, state map[string]any, after bool) {
	if r == nil {
		return
	}
	snap := &DebugSnapshot{State: jsonx.MustMarshal(state), Files: []DebugFile{}, Roots: []string{}}
	n := len(snap.State)
	if fs != nil {
		frozen, files, size, err := fs.Snapshot(ctx)
		if err != nil {
			snap.Error = err.Error()
		} else {
			snap.fs, snap.Roots = frozen, frozen.Roots()
			n += size
			for _, f := range files {
				if !f.IsDir {
					snap.Files = append(snap.Files, DebugFile{Path: f.Path, Size: f.Size, Summary: f.Summary, Blob: f.Blob})
				}
			}
		}
	}
	r.update(func() {
		if after {
			r.data.After = nil
		}
		if !r.reserve(n) {
			return
		}
		if after {
			r.data.After = snap
		} else {
			r.data.Before, r.data.After = snap, snap
		}
	})
}

func (r *debugRecorder) toolsDone(ctx context.Context, out tools.Outcome, fs *vfs.Table, state map[string]any) {
	if r == nil {
		return
	}
	r.update(func() {
		r.data.ToolCount = len(out.Records)
		b := jsonx.MustMarshal(out.Records)
		if r.reserve(len(b)) {
			r.data.Tools = slices.Clone(out.Records)
		}
		b = jsonx.MustMarshal(out.Results)
		if r.reserve(len(b)) {
			r.data.Results = slices.Clone(out.Results)
			for _, result := range out.Results {
				r.rememberImages(result.Parts)
			}
		}
	})
	r.snapshot(ctx, fs, state, true)
}

func (r *debugRecorder) rememberImages(parts []llm.Part) {
	for _, p := range parts {
		if p.Image != nil && !r.images[p.Image.Blob] && r.reserve(len(p.Image.Blob)+64) {
			if r.images == nil {
				r.images = map[string]bool{}
			}
			r.images[p.Image.Blob] = true
		}
		if p.ToolResult != nil {
			r.rememberImages(p.ToolResult.Parts)
		}
	}
}

func (r *debugRecorder) HTTP(method, url string, headers http.Header, body []byte) llm.HTTPObserver {
	h := &debugHTTPRecorder{r: r, start: time.Now(), data: &DebugHTTP{body: &bytes.Buffer{}}}
	r.update(func() {
		if !r.reserve(len(url) + len(jsonx.MustMarshal(headers)) + 512) {
			return
		}
		h.data.Method, h.data.URL, h.data.RequestHeaders = method, url, headers.Clone()
		h.data.RequestBody = r.text(string(body))
		r.data.HTTP = append(r.data.HTTP, h.data)
		r.data.HTTPCount++
	})
	return h
}

type debugHTTPRecorder struct {
	r     *debugRecorder
	start time.Time
	data  *DebugHTTP
}

func (h *debugHTTPRecorder) RequestHeader(name string, values []string) {
	h.r.update(func() {
		if h.data.RequestHeaders == nil {
			return
		}
		if h.r.reserve(len(name) + len(strings.Join(values, ""))) {
			h.data.RequestHeaders[name] = slices.Clone(values)
		}
	})
}

func (h *debugHTTPRecorder) Response(status int, headers http.Header) {
	h.r.update(func() {
		h.data.Status, h.data.HeaderMS = status, time.Since(h.start).Milliseconds()
		if h.r.reserve(len(jsonx.MustMarshal(headers))) {
			h.data.ResponseHeaders = headers.Clone()
		}
	})
}
func (h *debugHTTPRecorder) Body(b []byte) {
	h.r.update(func() {
		n := min(len(b), max(0, debugCallBytes-h.r.size))
		if n < len(b) {
			h.r.data.Truncated = true
		}
		if h.r.reserve(n) {
			_, _ = h.data.body.Write(b[:n])
		}
	})
}
func (h *debugHTTPRecorder) End(err error) {
	h.r.update(func() {
		h.data.Complete, h.data.DurationMS = true, time.Since(h.start).Milliseconds()
		if err != nil {
			h.data.Error = h.r.text(err.Error())
		}
	})
}

func (h *debugHTTPRecorder) Truncated() {
	h.r.update(func() { h.data.Truncated = true })
}

func debugStream(ctx context.Context, p llm.Provider, model *llm.Model, req *llm.Request, r *debugRecorder) (llm.Stream, error) {
	if r == nil {
		return p.Stream(ctx, model, req)
	}
	return llm.ObserveStream(ctx, p, model, req, r)
}
