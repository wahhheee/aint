// Package runtime 是回合运行器：存档会话、回合循环、预算、取消、提交与事件发布（《02-技术方案》第 9 节）。
//
// 一个回合的生命周期（turn.go）：
//
//	Submit ─► prepare（读进度线、作品、模型，封装玩家消息，构建请求）
//	       ─► loop（流式调用 → 工具执行 → 继续 / 终止型工具 / 截断续写 / 预算 / 取消）
//	       ─► commit（单事务提交：节点、消息、显示项、原文索引、用量、head）
//
// 并发模型：
//   - 每个存档一个 session，同一时刻只允许一个回合（或整理）在运行（不变量 5：单一写者）；
//   - 每个运行中的回合一个 goroutine，持有可取消的 ctx；与 HTTP 请求的生命周期脱钩
//     （请求返回后回合继续，进度通过事件总线推给 SSE）；
//   - 生成过程只在内存中暂存（draft），提交时一次写库：进程崩溃时不存在半提交（不变量 2）。
//
// 调试：所有日志带 save_id / turn_id；节点 meta 记录停止原因、调用次数、耗时、前缀哈希、提示词版本；
// 配合 --trace-llm 可看到每次模型调用的完整请求与流事件。
package runtime

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/wahhheee/yanyi/internal/contextx"
	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/media"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/ids"
	"github.com/wahhheee/yanyi/internal/pkg/logx"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/tmpl"
	"github.com/wahhheee/yanyi/internal/tools"
	"github.com/wahhheee/yanyi/internal/work"
)

// ModelResolver 解析存档使用的模型（由 provider.Service 实现；测试中可替换为假实现）。
type ModelResolver interface {
	Resolve(ctx context.Context, modelID string) (llm.Provider, *llm.Model, error)
}

// Deps 是运行器的依赖。
type Deps struct {
	Saves   *save.Service
	Works   *work.Service
	Models  ModelResolver
	Bus     *event.Bus
	Builder *contextx.Builder
	Tools   *tools.Registry
	Tmpl    *tmpl.Engine
	Images  *media.Store // 发送给模型的图片派生、玩家上传
	Budget  Budget
}

// Manager 管理全部存档会话。
type Manager struct {
	d Deps

	base      context.Context // 进程级 ctx：Shutdown 时取消全部回合
	cancelAll context.CancelFunc
	wg        sync.WaitGroup

	mu sync.Mutex
	// sessions 只保存有回合在运行的存档：回合结束（让出写者槽位）时删除，空闲存档不占内存。
	// 加锁顺序固定为 m.mu → session.mu。
	sessions map[string]*session
}

// session 是一个存档的运行状态。
type session struct {
	mu      sync.Mutex
	running *turnHandle
}

type turnHandle struct {
	id      string
	cancel  context.CancelFunc
	started time.Time
}

// NewManager 创建运行器。
func NewManager(d Deps) *Manager {
	if d.Budget == (Budget{}) {
		d.Budget = DefaultBudget
	}
	base, cancel := context.WithCancel(context.Background())
	return &Manager{d: d, base: base, cancelAll: cancel, sessions: map[string]*session{}}
}

// lockSession 返回存档的会话（没有则创建）并锁住它；调用方负责 s.mu.Unlock()。
func (m *Manager) lockSession(saveID string) *session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[saveID]
	if !ok {
		s = &session{}
		m.sessions[saveID] = s
	}
	s.mu.Lock()
	return s
}

// runningSession 返回有回合在运行的会话并锁住它；没有时返回 nil。
func (m *Manager) runningSession(saveID string) *session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[saveID]
	if !ok {
		return nil
	}
	s.mu.Lock()
	return s
}

// Submit 提交玩家输入并在后台开始一个回合，立即返回回合 ID（也是提交后的节点 ID）。
// 存档已有回合在运行时返回 Conflict（HTTP 409）。
func (m *Manager) Submit(ctx context.Context, saveID string, in save.TurnInput) (string, error) {
	if err := validateInput(in); err != nil {
		return "", err
	}
	if _, err := m.d.Saves.Get(ctx, saveID); err != nil {
		return "", err
	}
	s := m.lockSession(saveID)
	defer s.mu.Unlock()
	if s.running != nil {
		return "", errs.Conflict("上一回合还在进行中").WithDetails(map[string]string{"turn_id": s.running.id})
	}

	turnID := ids.New()
	// 回合与 HTTP 请求脱钩：保留请求 ctx 中的日志字段（request_id），但不随请求结束而取消；
	// 进程关闭（Shutdown）或玩家停止（Cancel）时取消。
	turnCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(m.base, cancel)
	turnCtx = logx.With(turnCtx, "save_id", saveID, "turn_id", turnID)
	s.running = &turnHandle{id: turnID, cancel: cancel, started: time.Now()}

	// release 让出单一写者槽位。必须先于终态事件（turn.committed / turn.failed）发布：
	// 客户端收到终态事件后可能立即提交下一回合，此时不应再得到 409。
	release := sync.OnceFunc(func() {
		m.mu.Lock()
		s.mu.Lock()
		s.running = nil
		if m.sessions[saveID] == s {
			delete(m.sessions, saveID)
		}
		s.mu.Unlock()
		m.mu.Unlock()
	})
	m.wg.Go(func() {
		defer func() {
			stop()
			cancel()
			release()
		}()
		defer func() {
			if p := recover(); p != nil {
				slog.ErrorContext(turnCtx, "回合运行 panic", "panic", p)
				release()
				m.publish(saveID, event.TurnFailed, event.TurnFailedData{
					TurnID: turnID, Code: string(errs.CodeInternal), Message: "回合运行出错（详见日志）", Recoverable: true,
				})
			}
		}()
		m.runTurn(turnCtx, saveID, turnID, in, release)
	})
	return turnID, nil
}

// Cancel 停止正在运行的回合。turnID 不匹配（已结束或是别的回合）时返回 NotFound。
func (m *Manager) Cancel(saveID, turnID string) error {
	s := m.runningSession(saveID)
	if s == nil {
		return errs.NotFound("没有正在进行的该回合")
	}
	defer s.mu.Unlock()
	if s.running == nil || s.running.id != turnID {
		return errs.NotFound("没有正在进行的该回合")
	}
	s.running.cancel()
	return nil
}

// Running 返回存档正在运行的回合 ID（没有则返回空）。
func (m *Manager) Running(saveID string) string {
	s := m.runningSession(saveID)
	if s == nil {
		return ""
	}
	defer s.mu.Unlock()
	if s.running == nil {
		return ""
	}
	return s.running.id
}

// Shutdown 取消全部运行中的回合并等待它们提交完毕（已生成的内容以 interrupted 状态保存）。
func (m *Manager) Shutdown(ctx context.Context) error {
	m.cancelAll()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) publish(saveID, typ string, data any) {
	m.d.Bus.Publish(event.SaveTopic(saveID), typ, data)
}

// MaxAttachments 是一次输入最多附带的图片数。
const MaxAttachments = 6

func validateInput(in save.TurnInput) error {
	if len(in.Attachments) > MaxAttachments {
		return errs.Invalid("一次最多附带 %d 张图片", MaxAttachments)
	}
	switch in.Kind {
	case save.InputFree, save.InputOOC:
		if in.Text == "" && (in.Kind == save.InputOOC || len(in.Attachments) == 0) {
			return errs.Invalid("输入内容为空")
		}
	case save.InputChoice:
		if in.ChoiceID == "" {
			return errs.Invalid("缺少 choice_id")
		}
	case save.InputCommand:
		if in.Command == nil || in.Command.Name == "" {
			return errs.Invalid("缺少命令")
		}
	case save.InputContinue, save.InputOpening:
	default:
		return errs.Invalid("未知的输入类型：%q", in.Kind)
	}
	return nil
}
