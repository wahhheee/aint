// Package app 负责启动：配置、数据目录、日志、单实例检测、依赖装配、HTTP 服务生命周期。
//
// 这里是唯一知道"所有服务如何组装在一起"的地方（组合根）。各业务包只通过构造参数拿到依赖，
// 不读取全局变量、不自行创建其他服务——想知道某个服务依赖什么，看 wire() 即可。
package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wahhheee/yanyi/examples"
	"github.com/wahhheee/yanyi/internal/buildinfo"
	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/pkg/ids"
	"github.com/wahhheee/yanyi/internal/pkg/logx"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/provider"
	"github.com/wahhheee/yanyi/internal/runtime"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/server"
	"github.com/wahhheee/yanyi/internal/settings"
	"github.com/wahhheee/yanyi/internal/store"
	"github.com/wahhheee/yanyi/internal/tmpl"
	"github.com/wahhheee/yanyi/internal/tools"
	"github.com/wahhheee/yanyi/internal/work"
	"github.com/wahhheee/yanyi/web"
)

// Options 是 `yanyi serve` 的启动参数。
type Options struct {
	DataDir   string // --data：数据目录（默认程序旁的 yanyi-data/）
	Listen    string // --listen：监听地址（默认 127.0.0.1）
	Port      int    // --port：端口（0 = 从 7860 起自动选择空闲端口）
	NoBrowser bool   // --no-browser：不自动打开浏览器
	FakeLLM   string // --fake-llm：使用假模型（"demo" 为内置脚本，或脚本文件路径）
	TraceLLM  bool   // --trace-llm：把每次模型调用写入 logs/llm-trace/
	LogLevel  string // --log-level：debug | info | warn | error
}

// DefaultPort 是自动选择端口的起点。
const DefaultPort = 7860

// Run 启动服务并阻塞直到 ctx 被取消（Ctrl+C），然后优雅关闭。
func Run(ctx context.Context, opts Options) error {
	dirs, err := ResolveDirs(opts.DataDir)
	if err != nil {
		return err
	}
	_, closeLog, err := logx.Setup(logx.Options{Level: logx.ParseLevel(opts.LogLevel), Dir: dirs.Logs, Console: true})
	if err != nil {
		return fmt.Errorf("初始化日志失败: %w", err)
	}
	defer func() {
		if err := closeLog(); err != nil {
			fmt.Fprintln(os.Stderr, "关闭日志文件失败:", err) // 日志已关闭，只能写到标准错误
		}
	}()

	if url := runningInstance(ctx, dirs.Instance); url != "" {
		slog.Info("该数据目录已有实例在运行，直接打开它的页面", "url", url, "data", dirs.Root)
		if !opts.NoBrowser {
			openBrowser(ctx, url)
		}
		return nil
	}

	bi := buildinfo.Get()
	slog.Info("演义启动中", "version", bi.Version, "commit", bi.Commit, "data", dirs.Root, "fake_llm", opts.FakeLLM != "")

	db, err := store.Open(ctx, dirs.DB)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("关闭数据库失败", "err", err)
		}
	}()
	if err := db.Migrate(ctx, dirs.Backup); err != nil {
		return err
	}

	ln, err := listen(ctx, opts)
	if err != nil {
		return err
	}
	url := "http://" + displayAddr(ln.Addr().String())
	info := server.SystemInfo{
		Build: bi, InstanceID: ids.New(), DataDir: dirs.Root, LogDir: dirs.Logs, Listen: ln.Addr().String(),
		FakeLLM: opts.FakeLLM != "", TraceLLM: opts.TraceLLM, PromptVersion: prompts.Version, StartedAt: time.Now().UnixMilli(),
	}

	svc, err := wire(ctx, db, dirs, opts, info)
	if err != nil {
		_ = ln.Close()
		return err
	}

	srv := &http.Server{
		Handler:           svc.server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// 不设 WriteTimeout：SSE 是长连接。
		ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	if err := writeInstance(dirs.Instance, instanceInfo{InstanceID: info.InstanceID, PID: os.Getpid(), URL: url, StartedAt: info.StartedAt}); err != nil {
		slog.Warn("写入 instance.json 失败（单实例检测将不可用）", "err", err)
	}
	defer func() {
		if err := os.Remove(dirs.Instance); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("删除 instance.json 失败（下次启动会识别并覆盖）", "err", err)
		}
	}()

	slog.Info("演义已就绪", "url", url)
	if ip := net.ParseIP(hostOf(ln.Addr().String())); ip != nil && !ip.IsLoopback() {
		slog.Warn("正在监听非本机地址：当前网络中的其他人可以访问你的存档与模型额度（访问密码功能尚未实现，TODO(M6-9)）")
	}
	if !opts.NoBrowser {
		openBrowser(ctx, url)
	}

	select {
	case <-ctx.Done():
		slog.Info("正在关闭……")
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP 服务异常退出: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 先停回合（已生成的内容以 interrupted 状态提交），再关 HTTP（SSE 连接随之断开）。
	if err := svc.runtime.Shutdown(shutdownCtx); err != nil {
		slog.Warn("等待回合结束超时", "err", err)
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("HTTP 服务关闭超时", "err", err)
	}
	slog.Info("已退出")
	return nil
}

// services 是装配好的服务集合。
type services struct {
	server  *server.Server
	runtime *runtime.Manager
}

// wire 组装全部服务（依赖关系一目了然）。
func wire(ctx context.Context, db *store.DB, dirs Dirs, opts Options, info server.SystemInfo) (*services, error) {
	blobs := store.NewBlobs(dirs.Blobs)
	bus := event.NewBus(event.DefaultBufferSize)
	tm := tmpl.New()

	secrets, err := provider.OpenSecrets(dirs.Secrets)
	if err != nil {
		return nil, err
	}
	popts := provider.Options{}
	if opts.FakeLLM != "" {
		if popts.Fake, err = fake.LoadScript(opts.FakeLLM); err != nil {
			return nil, err
		}
	}
	if opts.TraceLLM {
		popts.TraceDir = dirs.Trace
		slog.Info("已开启模型调用追踪", "dir", dirs.Trace)
	}
	providers := provider.NewService(db, secrets, popts)

	settingsSvc := settings.NewService(db)
	works := work.NewService(db, blobs)
	works.EnsureBuiltins(ctx, examples.Works())
	imports, err := work.NewImporter(works, filepath.Join(dirs.Tmp, "imports"), work.DefaultLimits)
	if err != nil {
		return nil, err
	}
	saves := save.NewService(db, blobs, works, tm)

	rt := runtime.NewManager(runtime.Deps{
		Saves: saves, Works: works, Models: providers, Bus: bus,
		Builder: contextx.NewBuilder(tm), Tools: tools.NewRegistry(), Tmpl: tm,
	})

	srv := server.New(server.Deps{
		Info: info, Bus: bus, Settings: settingsSvc, Works: works, Imports: imports, Saves: saves,
		Runtime: rt, Providers: providers, Web: web.Dist(),
	})
	return &services{server: srv, runtime: rt}, nil
}

// listen 选择监听地址：指定端口时只用该端口；否则从 DefaultPort 起找空闲端口。
func listen(ctx context.Context, opts Options) (net.Listener, error) {
	host := opts.Listen
	if host == "" {
		host = "127.0.0.1"
	}
	var lc net.ListenConfig
	if opts.Port > 0 {
		ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(opts.Port)))
		if err != nil {
			return nil, fmt.Errorf("端口 %d 不可用（可能已被占用，可换一个 --port 或不指定以自动选择）: %w", opts.Port, err)
		}
		return ln, nil
	}
	var lastErr error
	for p := DefaultPort; p < DefaultPort+40; p++ {
		ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(p)))
		if err == nil {
			return ln, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("找不到空闲端口（%d–%d）: %w", DefaultPort, DefaultPort+39, lastErr)
}

func hostOf(addr string) string {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return h
}

// displayAddr 把 0.0.0.0 等通配地址换成 127.0.0.1，便于在本机浏览器打开。
func displayAddr(addr string) string {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsUnspecified() {
		h = "127.0.0.1"
	}
	return net.JoinHostPort(h, p)
}
