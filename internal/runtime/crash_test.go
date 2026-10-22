package runtime_test

// 杀进程测试（M1-12，不变量 2"原子提交"：一个回合要么完整提交，要么不留痕迹）。
//
// 做法：测试重新执行测试二进制自身作为子进程（TestMain 见到 envCrashPhase 后不跑测试，改跑 crashChild：
// 在父进程准备好的数据目录上装配服务、提交一个回合），子进程跑到指定阶段时在标准输出打印一行标记，
// 父进程见到标记后杀掉它（Windows 上是 TerminateProcess，其他平台是 SIGKILL，都不给进程收尾的机会），
// 然后重新打开同一个数据目录检查：库完整、没有半个节点、之后能正常提交新回合。
//
// 故障注入点：提交事务内部的两个阶段用 SQLite 驱动的钩子注入——只在子进程里通过
// sqlite.RegisterConnectionHook 给连接装上 pre-update / commit 钩子，钩子输出标记后原地阻塞等父进程来杀。
// 生产代码里没有任何注入点，正常运行不受影响；测试也不依赖构建标签或环境变量开关之外的东西。
//
//   - first-token：模型调用已发出、首字之前（假模型首字延迟 60 秒）
//   - streaming：正文流式输出中（已收到 3 个增量）
//   - tool：工具往返中（turn.tool start 之后；第二次模型调用被延迟，回合一定还没提交）
//   - tx-write：提交事务写入中（blob 已写入事务、正在插入节点行）
//   - tx-commit：提交事务 COMMIT 中（全部语句已执行，SQLite 正要提交）
//   - committed：提交之后（turn.committed 之后）：回合必须完整保留

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/llm/fake"
	"github.com/wahhheee/yanyi/internal/markup"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/store"
)

// 子进程模式的环境变量。
const (
	envCrashPhase = "YANYI_TEST_CRASH_PHASE"
	envCrashDir   = "YANYI_TEST_CRASH_DIR"
	envCrashSave  = "YANYI_TEST_CRASH_SAVE"
)

func TestMain(m *testing.M) {
	if phase := os.Getenv(envCrashPhase); phase != "" {
		os.Exit(crashChild(phase, os.Getenv(envCrashDir), os.Getenv(envCrashSave)))
	}
	os.Exit(m.Run())
}

// 子进程在提交事务中阻塞的位置。
const (
	blockTxWrite  = "tx-write"  // 插入节点行之前（pre-update 钩子）
	blockTxCommit = "tx-commit" // COMMIT 开始时（commit 钩子）
)

// crashPhase 是杀进程的时机：子进程跑到这里时输出 marker，父进程见到后杀掉它。
type crashPhase struct {
	name      string
	marker    string // 子进程输出的标记行（或其前缀，后跟空格与参数）
	block     string // 子进程在提交事务中原地阻塞的位置；空表示不阻塞
	committed bool   // 杀进程时回合是否已经提交
	script    *fake.Script
}

// crashBigText 超过 store.InlineMax：消息与显示项 blob 落成文件（提交事务中、插入记录之前写入），
// 覆盖"文件先写、记录在事务里"的路径。
var crashBigText = strings.Repeat("海雾漫过码头，", 3700)

func crashBigScript() *fake.Script {
	return &fake.Script{Responses: []fake.Response{{Text: crashBigText, ChunkRunes: 5000, DelayMS: 1}}}
}

var crashPhases = []crashPhase{
	{name: "first-token", marker: "stream 1", script: &fake.Script{Responses: []fake.Response{
		{Text: "雾里传来汽笛声。", FirstDelayMS: 60_000},
	}}},
	{name: "streaming", marker: "text 3", script: &fake.Script{Responses: []fake.Response{
		{Text: strings.Repeat("雾越来越浓。", 300), ChunkRunes: 2, DelayMS: 10},
	}}},
	{name: "tool", marker: "tool start read", script: &fake.Script{Responses: []fake.Response{
		{Text: "他翻开柜台上的登记簿。", ChunkRunes: 50, DelayMS: 1,
			ToolCalls: []fake.ToolCall{{Name: "read", Args: map[string]any{"path": "/memory/index.md"}}}},
		{Text: "登记簿上的字迹已经模糊。", FirstDelayMS: 60_000},
	}}},
	{name: "tx-write", marker: "tx nodes", block: blockTxWrite, script: crashBigScript()},
	{name: "tx-commit", marker: "tx commit", block: blockTxCommit, script: crashBigScript()},
	{name: "committed", marker: "committed", committed: true, script: crashBigScript()},
}

// crashInput 是子进程提交的回合（上一回合以 offer_choices 结束，自由输入作为工具结果回填）。
var crashInput = save.TurnInput{Kind: save.InputFree, Text: "我问老陈那封信的事。"}

func TestKillMidTurn(t *testing.T) {
	if len(crashBigText) <= store.InlineMax {
		t.Fatal("测试前提：crashBigText 应超过 blob 内联上限")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, ph := range crashPhases {
		t.Run(ph.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			saveID, before := seedCrashSave(t, dir)
			turnID := killChildAt(t, exe, dir, saveID, ph)

			e := setupAt(t, dir, fake.New(quickScript()))
			after := inspectStore(t, e, saveID)
			if ph.committed {
				if want := append(slices.Clone(before.line), turnID); !reflect.DeepEqual(after.line, want) {
					t.Fatalf("提交之后被杀：进度线应以该回合结尾\n得到 %v\n期望 %v", after.line, want)
				}
				if !slices.Contains(after.fts, turnID) || !slices.Contains(after.usage, turnID) {
					t.Fatalf("提交之后被杀：原文索引与用量应已写入：%+v", after)
				}
				var isFile bool
				if err := e.db.Read().QueryRowContext(context.Background(),
					`SELECT b.inline IS NULL FROM nodes n JOIN blobs b ON b.hash = n.messages WHERE n.id = ?`, turnID).Scan(&isFile); err != nil {
					t.Fatal(err)
				}
				if !isFile {
					t.Fatal("测试前提：大回合的消息 blob 应落成文件")
				}
			} else {
				if slices.Contains(after.nodes, turnID) || slices.Contains(after.fts, turnID) || slices.Contains(after.usage, turnID) {
					t.Fatalf("未提交的回合留下了痕迹：%+v", after)
				}
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("未提交的回合改变了存档\n之前 %+v\n之后 %+v", before, after)
				}
			}

			// 重启后能正常提交新回合，接在杀进程后的 head 之后。
			sub, _, _ := e.bus.Subscribe(event.SaveTopic(saveID), 0)
			defer sub.Close()
			next, err := e.mgr.Submit(context.Background(), saveID, save.TurnInput{Kind: save.InputFree, Text: "继续往前走。"})
			if err != nil {
				t.Fatal(err)
			}
			waitFor(t, sub, event.TurnCommitted)
			final := inspectStore(t, e, saveID)
			if want := append(slices.Clone(after.line), next); !reflect.DeepEqual(final.line, want) {
				t.Fatalf("重启后的新回合应接在 head 之后\n得到 %v\n期望 %v", final.line, want)
			}
		})
	}
}

// setupAt 在已有的数据目录上装配服务。
func setupAt(t *testing.T, dir string, provider llm.Provider) *env {
	t.Helper()
	e, err := openEnv(context.Background(), dir, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })
	return e
}

// seedCrashSave 准备杀进程前的存档：静态开场 + 一个以 offer_choices 结束的模型回合。返回存档 ID 与库的快照。
func seedCrashSave(t *testing.T, dir string) (string, storeSnapshot) {
	t.Helper()
	ctx := context.Background()
	e, err := openEnv(ctx, dir, fake.New(quickScript()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e.close(); err != nil {
			t.Error(err)
		}
	}()
	sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: "yanyi.misty-harbor", Settings: save.Settings{Pace: "short"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if _, err := e.mgr.Submit(ctx, sv.ID, save.TurnInput{Kind: save.InputChoice, ChoiceID: "a"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)
	return sv.ID, inspectStore(t, e, sv.ID)
}

// killChildAt 启动子进程提交回合，等它输出 ph.marker 后杀掉，返回子进程中的回合 ID。
func killChildAt(t *testing.T, exe, dir, saveID string, ph crashPhase) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), exe)
	cmd.Env = append(os.Environ(), envCrashPhase+"="+ph.name, envCrashDir+"="+dir, envCrashSave+"="+saveID)
	var stderr lockedBuffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var waitOnce sync.Once
	var waitErr error
	wait := func() error {
		waitOnce.Do(func() { waitErr = cmd.Wait() })
		return waitErr
	}
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
		_ = cmd.Process.Kill() // 测试中途失败时兜底；已退出时返回的错误无需处理
		_ = wait()
	})

	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-done:
				return
			}
		}
	}()

	var turnID string
	timeout := time.NewTimer(60 * time.Second)
	defer timeout.Stop()
	for reached := false; !reached; {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("子进程在到达 %q 之前退出（%v）：\n%s", ph.marker, wait(), stderr.String())
			}
			switch {
			case strings.HasPrefix(line, "submitted "):
				turnID = strings.TrimPrefix(line, "submitted ")
			case strings.HasPrefix(line, "failed "):
				t.Fatalf("子进程中的回合失败了：%s\n%s", line, stderr.String())
			case line == ph.marker || strings.HasPrefix(line, ph.marker+" "):
				reached = true
			}
		case <-timeout.C:
			t.Fatalf("等待子进程输出 %q 超时：\n%s", ph.marker, stderr.String())
		}
	}
	if turnID == "" {
		t.Fatal("子进程应先输出回合 ID")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("杀掉子进程失败：%v", err)
	}
	if err := wait(); !errors.As(err, new(*exec.ExitError)) {
		t.Fatalf("子进程应是被杀掉的，得到 %v：\n%s", err, stderr.String())
	}
	return turnID
}

// crashChild 是子进程的入口：在 dir 上装配服务，给 saveID 提交一个回合，按阶段输出标记，等父进程来杀。
// 返回值是退出码（正常情况下不会返回：进程在此之前已被杀掉）。
func crashChild(name, dir, saveID string) int {
	var ph *crashPhase
	for i := range crashPhases {
		if crashPhases[i].name == name {
			ph = &crashPhases[i]
		}
	}
	if ph == nil {
		fmt.Fprintln(os.Stderr, "未知的杀进程阶段：", name)
		return 2
	}

	// 标记行一律在 "submitted <回合 ID>" 之后输出（emit 与 Submit 共用一把锁）。
	var mu sync.Mutex
	emit := func(parts ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Println(parts...)
	}
	hold := func() { time.Sleep(time.Hour) }

	// 故障注入：只在子进程里给 SQLite 连接装钩子；armed 之前（建库、导入内置作品）不触发。
	var armed atomic.Bool
	sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, _ string) error {
		h, ok := conn.(sqlite.HookRegisterer)
		if !ok {
			return errors.New("SQLite 连接不支持钩子")
		}
		switch ph.block {
		case blockTxWrite:
			h.RegisterPreUpdateHook(func(d sqlite.SQLitePreUpdateData) {
				if armed.Load() && d.TableName == "nodes" && d.Op == sqlite3.SQLITE_INSERT {
					emit("tx nodes")
					hold()
				}
			})
		case blockTxCommit:
			h.RegisterCommitHook(func() int32 {
				if armed.Load() {
					emit("tx commit")
					hold()
				}
				return 0
			})
		}
		return nil
	})

	ctx := context.Background()
	e, err := openEnv(ctx, dir, &markingProvider{Provider: fake.New(ph.script), emit: emit})
	if err != nil {
		fmt.Fprintln(os.Stderr, "子进程装配失败：", err)
		return 2
	}
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(saveID), 0)
	go func() {
		for ev := range sub.C {
			switch d := ev.Data.(type) {
			case event.TurnTextData:
				emit("text", d.Seq)
			case event.TurnToolData:
				emit("tool", d.Phase, d.Name)
			case event.TurnCommittedData:
				emit("committed", d.NodeID)
			case event.TurnFailedData:
				emit("failed", d.Code, d.Message)
			}
		}
	}()

	armed.Store(true)
	mu.Lock()
	turnID, err := e.mgr.Submit(ctx, saveID, crashInput)
	if err != nil {
		mu.Unlock()
		fmt.Fprintln(os.Stderr, "提交回合失败：", err)
		return 2
	}
	fmt.Println("submitted", turnID)
	mu.Unlock()

	time.Sleep(2 * time.Minute)
	fmt.Fprintln(os.Stderr, "等待超时：父进程没有杀掉子进程")
	return 3
}

// markingProvider 在每次模型调用时输出 "stream <n>"。
type markingProvider struct {
	llm.Provider
	emit  func(...any)
	calls atomic.Int32
}

func (p *markingProvider) Stream(ctx context.Context, m *llm.Model, req *llm.Request) (llm.Stream, error) {
	p.emit("stream", p.calls.Add(1))
	return p.Provider.Stream(ctx, m, req)
}

// storeSnapshot 是一个存档在库中的全部痕迹，用于比较杀进程前后。
type storeSnapshot struct {
	head  string
	line  []string // 进度线（root → head）
	nodes []string // 存档的全部节点（按 ID 排序）
	fts   []string // 原文索引行的 node_id（按 ID 排序）
	usage []string // 用量记录的 node_id（按 ID 排序，可重复）
	blobs int      // blob 记录总数
}

// inspectStore 检查库与存档的一致性并返回快照：
// PRAGMA integrity_check / foreign_key_check；进度线从 root 逐级连到 head；时间线可读；
// 每个节点引用的消息、显示项、状态、记忆 blob 都存在且能解析；原文索引与节点正文一致；用量记录都属于存在的节点。
func inspectStore(t *testing.T, e *env, saveID string) storeSnapshot {
	t.Helper()
	ctx := context.Background()
	q := e.db.Read()

	var integrity string
	if err := q.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check：%q %v", integrity, err)
	}
	if bad := queryStrings(t, e, `SELECT "table" FROM pragma_foreign_key_check`); len(bad) > 0 {
		t.Fatalf("外键不一致：%v", bad)
	}

	sv, line, err := e.saves.ProgressLine(ctx, saveID)
	if err != nil {
		t.Fatal(err)
	}
	snap := storeSnapshot{head: sv.HeadNodeID}
	turns := 0
	for i, n := range line {
		snap.line = append(snap.line, n.ID)
		if n.Depth != i || (i == 0) != (n.Kind == save.KindRoot) || (i > 0 && n.ParentID != line[i-1].ID) {
			t.Fatalf("进度线断开：第 %d 个节点 %+v", i, n)
		}
		if n.Kind == save.KindTurn {
			turns++
		}
	}
	if last := line[len(line)-1]; last.ID != sv.HeadNodeID {
		t.Fatalf("进度线应以 head 结尾：%s ≠ %s", last.ID, sv.HeadNodeID)
	}
	tl, err := e.saves.Timeline(ctx, saveID, "", 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(tl.Nodes) != turns || tl.HeadID != sv.HeadNodeID {
		t.Fatalf("时间线应有 %d 个回合、head 为 %s，得到 %d 个、%s", turns, sv.HeadNodeID, len(tl.Nodes), tl.HeadID)
	}

	// 节点引用的 blob 都在；回合节点的原文索引应等于其正文的纯文本。
	wantFTS := map[string]string{}
	for _, n := range queryNodeRows(t, e, saveID) {
		snap.nodes = append(snap.nodes, n.id)
		if n.status != string(save.StatusCommitted) && n.status != string(save.StatusInterrupted) {
			t.Fatalf("节点 %s 的状态不对：%s", n.id, n.status)
		}
		if _, err := e.saves.LoadState(ctx, n.state); err != nil {
			t.Fatalf("节点 %s 的状态快照读不出：%v", n.id, err)
		}
		if _, err := e.saves.MemoryIndex(ctx, n.memory); err != nil {
			t.Fatalf("节点 %s 的记忆清单读不出：%v", n.id, err)
		}
		if n.kind != string(save.KindTurn) {
			continue
		}
		msgs, err := e.saves.LoadMessages(ctx, n.messages)
		if err != nil || len(msgs) == 0 {
			t.Fatalf("回合 %s 的消息读不出：%v", n.id, err)
		}
		display, err := e.saves.LoadDisplay(ctx, n.display)
		if err != nil || len(display) == 0 {
			t.Fatalf("回合 %s 的显示项读不出：%v", n.id, err)
		}
		var text strings.Builder
		for _, it := range display {
			if it.T == "text" {
				text.WriteString(it.Markup + "\n\n")
			}
		}
		if plain := markup.PlainText(text.String()); plain != "" {
			wantFTS[n.id] = plain
		}
	}

	snap.fts = queryStrings(t, e, `SELECT node_id FROM transcript_fts WHERE save_id = ? ORDER BY node_id`, saveID)
	gotFTS := map[string]string{}
	for _, id := range snap.fts {
		var text string
		if err := q.QueryRowContext(ctx, `SELECT text FROM transcript_fts WHERE node_id = ?`, id).Scan(&text); err != nil {
			t.Fatal(err)
		}
		gotFTS[id] = text
	}
	if len(gotFTS) != len(snap.fts) {
		t.Fatalf("同一节点有多行原文索引：%v", snap.fts)
	}
	if !reflect.DeepEqual(gotFTS, wantFTS) {
		for id, want := range wantFTS {
			if gotFTS[id] != want {
				t.Errorf("节点 %s 的原文索引与正文不一致\n索引 %.80q\n正文 %.80q", id, gotFTS[id], want)
			}
		}
		for id := range gotFTS {
			if _, ok := wantFTS[id]; !ok {
				t.Errorf("原文索引中有不存在的节点或没有正文的节点：%s", id)
			}
		}
		t.FailNow()
	}

	snap.usage = queryStrings(t, e, `SELECT COALESCE(node_id, '') FROM usage_log WHERE save_id = ? ORDER BY node_id`, saveID)
	for _, id := range snap.usage {
		if !slices.Contains(snap.nodes, id) {
			t.Fatalf("用量记录指向不存在的节点：%q", id)
		}
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM blobs`).Scan(&snap.blobs); err != nil {
		t.Fatal(err)
	}
	return snap
}

type nodeRow struct{ id, kind, status, messages, display, state, memory string }

// queryNodeRows 直接从表中取存档的全部节点（不经 save 包：要看到进度线之外、可能残留的节点）。
func queryNodeRows(t *testing.T, e *env, saveID string) []nodeRow {
	t.Helper()
	rows, err := e.db.Read().QueryContext(context.Background(), `
		SELECT id, kind, status, COALESCE(messages, ''), COALESCE(display, ''), state_hash, memory_hash
		FROM nodes WHERE save_id = ? ORDER BY id`, saveID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []nodeRow
	for rows.Next() {
		var n nodeRow
		if err := rows.Scan(&n.id, &n.kind, &n.status, &n.messages, &n.display, &n.state, &n.memory); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func queryStrings(t *testing.T, e *env, query string, args ...any) []string {
	t.Helper()
	rows, err := e.db.Read().QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// lockedBuffer 是并发安全的 bytes.Buffer（exec 在后台 goroutine 中写入子进程的标准错误）。
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
