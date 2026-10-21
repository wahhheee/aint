package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/wahhheee/yanyi/internal/pkg/fsx"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// WithTrace 包装 Provider：把每次调用的"中立请求 + 全部流事件 + 错误"写成一个 JSON 文件。
//
// 用于排查"模型为什么这样写""适配器转换是否正确""缓存为什么没命中"等问题。
// 启用方式：yanyi --trace-llm（文件写到 <数据目录>/logs/llm-trace/）。
// 文件只包含中立格式，不含 API Key；但包含完整正文，分享前请注意内容。
//
// TODO(M4)：适配器层另加"协议原始请求/响应"追踪（检查器"原始"页，脱敏）。
func WithTrace(p Provider, dir string) Provider {
	return &traceProvider{Provider: p, dir: dir}
}

type traceProvider struct {
	Provider
	dir string
	seq sync.Mutex
}

type traceRecord struct {
	Time     time.Time    `json:"time"`
	Protocol Protocol     `json:"protocol"`
	Model    *Model       `json:"model"`
	Request  *Request     `json:"request"`
	Events   []traceEvent `json:"events"`
	Error    string       `json:"error,omitzero"`
	Duration string       `json:"duration"`
}

type traceEvent struct {
	AtMS int64  `json:"at_ms"` // 相对调用开始的毫秒数（看首字延迟、卡顿）
	Type string `json:"type"`
	Data Event  `json:"data,omitzero"`
}

func (t *traceProvider) Stream(ctx context.Context, m *Model, req *Request) (Stream, error) {
	rec := &traceRecord{Time: time.Now(), Protocol: t.Protocol(), Model: m, Request: req}
	s, err := t.Provider.Stream(ctx, m, req)
	if err != nil {
		rec.Error = err.Error()
		t.write(ctx, rec)
		return nil, err
	}
	return &traceStream{Stream: s, rec: rec, owner: t, ctx: ctx}, nil
}

type traceStream struct {
	Stream
	rec   *traceRecord
	owner *traceProvider
	ctx   context.Context
	once  sync.Once
}

func (s *traceStream) Next() (Event, error) {
	ev, err := s.Stream.Next()
	at := time.Since(s.rec.Time).Milliseconds()
	if err != nil {
		if !errors.Is(err, io.EOF) {
			s.rec.Error = err.Error()
		}
		return ev, err
	}
	s.rec.Events = append(s.rec.Events, traceEvent{AtMS: at, Type: EventName(ev), Data: ev})
	return ev, nil
}

func (s *traceStream) Close() error {
	err := s.Stream.Close()
	s.once.Do(func() { s.owner.write(s.ctx, s.rec) })
	return err
}

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func (t *traceProvider) write(ctx context.Context, rec *traceRecord) {
	rec.Duration = time.Since(rec.Time).String()
	t.seq.Lock()
	defer t.seq.Unlock()
	model := "unknown"
	if rec.Model != nil {
		model = rec.Model.Name
	}
	name := fmt.Sprintf("%s-%s-%s.json",
		rec.Time.Format("20060102-150405.000"), rec.Request.Purpose, unsafeName.ReplaceAllString(model, "_"))
	b, err := jsonx.MarshalIndent(rec)
	if err == nil {
		err = os.MkdirAll(t.dir, fsx.DirPerm)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(t.dir, name), b, fsx.PrivateFilePerm)
	}
	if err != nil {
		slog.WarnContext(ctx, "写入 LLM 追踪文件失败", "err", err)
		return
	}
	slog.DebugContext(ctx, "已写入 LLM 追踪文件", "file", name)
}
