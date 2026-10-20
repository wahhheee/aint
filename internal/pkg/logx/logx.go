// Package logx 配置全局日志，并提供"上下文携带日志字段"的能力。
//
// 用法约定（排查问题的关键）：
//
//	ctx = logx.With(ctx, "save_id", saveID, "turn_id", turnID)
//	slog.InfoContext(ctx, "回合开始")   // 自动带上 save_id、turn_id
//
// 即：在请求/回合入口处把关联 ID 放进 ctx，之后一律使用 slog.XxxContext(ctx, ...)，
// 同一回合的所有日志都能用 turn_id 串起来。请求入口的 request_id 由 server 中间件放入。
//
// 输出：
//   - 终端（stderr）：人类可读的文本格式；
//   - 文件（<数据目录>/logs/yanyi-YYYY-MM-DD.log）：JSON 行格式，便于 grep / jq 分析。
//
// 默认不记录请求正文与模型输出；需要时用 --trace-llm 单独落盘（见 llm/trace.go）。
package logx

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options 是日志配置。
type Options struct {
	Level   slog.Level
	Dir     string // 日志文件目录；为空则不写文件
	Keep    int    // 保留最近多少天的日志文件，<=0 时取 14
	Console bool   // 是否同时输出到终端
}

// Setup 创建日志器并设为 slog 默认日志器。返回的 close 函数在退出时调用。
func Setup(opts Options) (*slog.Logger, func() error, error) {
	var handlers []slog.Handler
	closeFn := func() error { return nil }

	if opts.Console {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: opts.Level}))
	}
	if opts.Dir != "" {
		keep := opts.Keep
		if keep <= 0 {
			keep = 14
		}
		w, err := newDailyFile(opts.Dir, "yanyi", keep)
		if err != nil {
			return nil, nil, err
		}
		closeFn = w.Close
		// 文件日志至少记录 Info，便于事后排查；Debug 级别跟随配置。
		handlers = append(handlers, slog.NewJSONHandler(w, &slog.HandlerOptions{Level: opts.Level}))
	}
	if len(handlers) == 0 {
		handlers = append(handlers, slog.NewTextHandler(io.Discard, nil))
	}

	logger := slog.New(contextHandler{slog.NewMultiHandler(handlers...)})
	slog.SetDefault(logger)
	return logger, closeFn, nil
}

// ParseLevel 解析 debug/info/warn/error（大小写不敏感），无法识别时返回 info。
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ---------------------------------------------------------------------------
// 上下文字段

type ctxKey struct{}

// With 返回携带额外日志字段的 ctx。args 的写法与 slog 相同（键值对或 slog.Attr）。
func With(ctx context.Context, args ...any) context.Context {
	if len(args) == 0 {
		return ctx
	}
	prev, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	r := slog.NewRecord(timeZero, 0, "", 0)
	r.Add(args...)
	next := make([]slog.Attr, 0, len(prev)+r.NumAttrs())
	next = append(next, prev...)
	r.Attrs(func(a slog.Attr) bool {
		next = append(next, a)
		return true
	})
	return context.WithValue(ctx, ctxKey{}, next)
}

// contextHandler 在输出前把 ctx 中的字段追加到日志记录上。
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(ctxKey{}).([]slog.Attr); ok {
		r.AddAttrs(attrs...)
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

// ---------------------------------------------------------------------------
// 敏感信息

// Mask 把密钥类字符串打码，只保留首尾少量字符，用于日志与前端展示。
func Mask(s string) string {
	r := []rune(s)
	switch {
	case len(r) == 0:
		return ""
	case len(r) <= 8:
		return "••••"
	default:
		return string(r[:3]) + "…" + string(r[len(r)-4:])
	}
}

// Secret 包装敏感字符串，写日志时自动打码：slog.Info("…", "key", logx.Secret(k))。
type Secret string

func (s Secret) LogValue() slog.Value { return slog.StringValue(Mask(string(s))) }
