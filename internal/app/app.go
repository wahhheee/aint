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
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wahhheee/aint/examples"
	"github.com/wahhheee/aint/internal/access"
	"github.com/wahhheee/aint/internal/buildinfo"
	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/media"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/pkg/logx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/provider"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/server"
	"github.com/wahhheee/aint/internal/settings"
	"github.com/wahhheee/aint/internal/store"
	"github.com/wahhheee/aint/internal/tmpl"
	"github.com/wahhheee/aint/internal/tools"
	"github.com/wahhheee/aint/internal/usage"
	"github.com/wahhheee/aint/internal/work"
	"github.com/wahhheee/aint/web"
)

// Options 是 `aint serve` 的启动参数。
type Options struct {
	DataDir   string // --data：数据目录（默认程序旁的 aint-data/）
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
	slog.Info("AINT 启动中", "version", bi.Version, "commit", bi.Commit, "data", dirs.Root, "fake_llm", opts.FakeLLM != "")

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

	if opts.Listen == "" {
		configured, err := settings.NewService(db).Get(ctx)
		if err != nil {
			return err
		}
		opts.Listen = configured.Listen
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
	defer svc.closeDev()

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

	slog.Info("AINT 已就绪", "url", url)
	if ip := net.ParseIP(hostOf(ln.Addr().String())); ip != nil && !ip.IsLoopback() && !svc.access.Enabled() {
		slog.Warn("正在监听非本机地址且未设置访问密码：当前网络中的其他人可以访问你的存档与模型额度，建议设置访问密码")
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
	svc.tmpl.Close()
	slog.Info("已退出")
	return nil
}

// services 是装配好的服务集合。
type services struct {
	access   *access.Service
	closeDev func()
	server   *server.Server
	runtime  *runtime.Manager
	tmpl     *tmpl.Engine
}

// wire 组装全部服务（依赖关系一目了然）。
func wire(ctx context.Context, db *store.DB, dirs Dirs, opts Options, info server.SystemInfo) (*services, error) {
	blobs := store.NewBlobs(dirs.Blobs)
	bus := event.NewBus(event.DefaultBufferSize)
	tm, err := isolatedTemplates()
	if err != nil {
		return nil, err
	}

	secrets, err := provider.OpenSecrets(dirs.Secrets)
	if err != nil {
		return nil, err
	}
	accessSvc, err := access.New(secrets)
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
	providers := provider.NewService(db, blobs, secrets, popts)

	settingsSvc := settings.NewService(db)
	works := work.NewService(db, blobs)
	registry := tools.NewRegistry()
	works.SetAnalyzer(func(m *work.Model, r *work.Report) { contextx.Analyze(m, r, registry.Specs) })
	works.EnsureBuiltins(ctx, examples.Works())
	imports, err := work.NewImporter(works, filepath.Join(dirs.Tmp, "imports"), work.DefaultLimits)
	if err != nil {
		return nil, err
	}
	imports.SetGitHubToken(func() string { value, _ := secrets.Resolve("github_token"); return value })
	saves := save.NewService(db, blobs, works, tm)
	if _, err := saves.CollectStorage(ctx, true); err != nil {
		slog.WarnContext(ctx, "启动存储清理失败", "err", err)
	}
	images := media.NewStore(db, blobs, filepath.Join(dirs.Cache, "derived"))

	rt := runtime.NewManager(runtime.Deps{
		Saves: saves, Works: works, Models: providers, Bus: bus,
		Builder: contextx.NewBuilder(tm, images), Tools: registry, Tmpl: tm, Images: images, Settings: settingsSvc,
	})

	srv := server.New(server.Deps{
		Access: accessSvc, Secrets: secrets,
		Info: info, Bus: bus, Settings: settingsSvc, Works: works, Imports: imports, Saves: saves,
		Runtime: rt, Providers: providers, Usage: usage.NewService(db), Images: images, Web: web.Dist(),
	})
	closeDev, err := works.WatchDevelopment(ctx, func(id string, report *work.Report) {
		bus.Publish(event.GlobalTopic, event.WorkReloaded, event.WorkReloadedData{WorkID: id, Report: report})
	})
	if err != nil {
		return nil, err
	}
	return &services{server: srv, runtime: rt, tmpl: tm, closeDev: closeDev, access: accessSvc}, nil
}

// TemplateWorkerCommand 是模板渲染工作进程的隐藏子命令（cmd/aint 中登记）。
const TemplateWorkerCommand = "__tmpl-worker"

// isolatedTemplates 创建在隔离工作进程中渲染作者模板的渲染器（D-46）：工作进程是本程序自身。
func isolatedTemplates() (*tmpl.Engine, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("找不到程序自身的路径（模板渲染进程需要）: %w", err)
	}
	return tmpl.NewIsolated(func() *exec.Cmd {
		return exec.Command(exe, TemplateWorkerCommand) //nolint:noctx // 工作进程的生命周期由渲染器管理（超时即杀），不随单个请求的 ctx 结束
	}), nil
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
