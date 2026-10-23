# 开发与调试

## 1. 环境

| 工具 | 版本 | 说明 |
| --- | --- | --- |
| Go | ≥ 1.27 | 1.27 起 `encoding/json/v2` 默认可用，无需设置 `GOEXPERIMENT` |
| Node | ≥ 22 | 前端构建 |
| pnpm | 11 | 版本写在 `web/package.json` 的 `packageManager` |
| Task | 3 | <https://taskfile.dev>，任务编排（跨平台） |
| golangci-lint | 2.15.0 | `task lint` 需要；与 CI 使用同一版本（`Taskfile.yml` 的 `GOLANGCI_VERSION`），版本不同检查结果可能不同。配置见 `.golangci.yml` |

> TypeScript 固定在 6.0.x：`typescript-eslint` 目前只支持 `<6.1`。等生态支持 TypeScript 7 后再升级。

## 2. 日常开发

```bash
task setup
```

```bash
task dev
```

- 访问 <http://localhost:5173>（Vite，热更新）；后端在 7860 端口，`/api` 由 Vite 代理过去。
- 后端默认使用**假模型**（`--fake-llm demo`），不需要 API Key；数据在 `.devdata/`，删掉即可从头开始。
- 改了 Go 代码需要重启后端（`task dev:server`）；VS Code 用户可以直接用 `.vscode/launch.json` 里的配置断点调试。
- 只想看构建后的效果：`task build`，然后运行 `bin/yanyi --data .devdata --fake-llm demo`。

### 2.1 使用真实模型

```bash
task dev:server:real
```

- 在设置 → 模型中添加供应商（协议、Base URL、API Key），勾选或手动添加模型，向导最后会做一次连接测试（真实调用，会产生少量费用）。
  第一个添加的模型自动设为主笔。密钥只写入 `.devdata/secrets.json`，不进数据库、不发给浏览器。
- 中转站：Base URL 按站点文档填写（OpenAI 协议一般以 `/v1` 结尾）；有分组倍率时填在供应商的"价格倍率"里，模型单价照抄站点标价。
- DeepSeek 建议用 Responses 协议接入（Anthropic 兼容接口在多轮工具调用上不可靠，见 [api-verification.md](notes/api-verification.md)）。
- 轻量模型（可选）：模型列表中某个模型的"⋯" → 设为轻量模型；之后可以在设置 → 上下文中改用轻量模型独立整理（不复用主笔的缓存）。
- `--trace-llm` 除中立追踪外，还会把**协议原始请求与响应**写到 `<数据目录>/logs/llm-trace/raw/`（鉴权头脱敏），
  排查"适配器发出去的到底是什么"、制作回放夹具都用它。

## 3. 调试手册

### 3.1 日志

- 终端是人类可读的文本；`<数据目录>/logs/yanyi-YYYY-MM-DD.log` 是 JSON 行，便于 `grep` / `jq`。
- `--log-level debug` 打开调试日志（`task dev` 默认开启）。
- **关联 ID**：每个请求有 `request_id`（响应头 `X-Request-ID`、错误响应体里都有），每个回合的日志都带 `save_id` 与 `turn_id`：

  ```bash
  grep 01a159a8-cfad-770a .devdata/logs/yanyi-*.log
  ```

- 新代码写日志请用 `slog.InfoContext(ctx, …)`，并在入口处用 `logx.With(ctx, "key", value)` 放入关联字段。

### 3.2 模型为什么这样写？

| 手段 | 说明 |
| --- | --- |
| `task prompt -- <作品目录> [--vision] [--helpers] [--json]` | 打印 AI 实际收到的系统提示词、首条消息、工具定义，以及每个片段的启用情况与原因 |
| `--trace-llm` | 每次模型调用的**完整中立请求 + 全部流事件（带毫秒时间戳）+ 错误**写成一个 JSON：`<数据目录>/logs/llm-trace/` |
| 节点 meta | 数据库 `nodes.meta`：模型、停止原因、调用次数、续写次数、耗时、错误、前缀哈希、提示词版本、`context_tokens`（这个节点之后的上下文规模）；整理节点另有 `compact_reason`、`summary_from` / `summary_to`、`keep_from_turn` |
| 检查器 | TODO(M5-9)：阅读页内查看每回合的上下文分层、工具调用、状态与记忆差异 |

前缀哈希（`meta.prefix_hash`）在相邻回合间变化，说明缓存被重建了——看 `task prompt` 的片段列表找原因。

### 3.3 复现问题：假模型脚本

把出问题的模型输出写成脚本（格式见 `internal/llm/fake/script.go`），稳定重放：

```bash
go run ./cmd/yanyi serve --data .devdata --fake-llm testdata/fake/my-case.yaml
```

脚本可以模拟：任意正文与叙事标记、工具调用、截断（`stop: max_tokens`）、错误（`error: rate_limited`）、首字延迟；
`ooc:` 段是玩家场外发言时的回应，`compact:` 段是上下文整理时依次使用的回复（先写记忆、再给摘要；内置的 `demo` 脚本都有示例）。
正文与工具参数中的字符串都可以写 `{{input}}`（换成玩家最近一次输入），方便每回合写入不同的记忆。

### 3.4 上下文整理与分支

- **很快触发整理**：新建存档时把预算调低（存档设置覆盖全局设置），例如
  `POST /api/v1/saves {"work_id": "yanyi.misty-harbor", "settings": {"context_budget": 9000, "keep_recent": 2000}}`，
  假模型下十来个回合就会整理；也可以点用量环里的"立即整理"或输入 `/compact`。全局默认值在设置 → 上下文。
- **整理做了什么**：日志 `上下文整理完成`（原因、方式、摘要掉的回合区间、保留起点、落盘写了几个文件、整理前后的上下文规模、调用次数）；
  整理节点的 `epoch_block` 是新阶段块（blob，文本），`messages` 是整理过程的全部消息（不进入之后的上下文）；`--trace-llm` 中用途为 `compact` 的调用。
- **之后的请求长什么样**：`--trace-llm` 中整理之后的第一个 `narrate` 请求，首条消息是新阶段块，后接保留的回合（推理与协议原生内容已去掉）。
- **整理方式**：设置 → 上下文。轻量模型在设置 → 模型的"⋯"菜单中设定；没有轻量模型时 separate 自动改用 inline。
- **分支**：`GET /api/v1/saves/{id}/tree` 看分叉点与各段；`GET /api/v1/saves/{id}/timeline` 的 `has_later` 表示 head 在较早的回合；
  修改正文、移动 head、编辑记忆或状态等操作完成后，存档事件流推送 `timeline.changed`。

### 3.5 前端

- 控制台执行 `localStorage.debugSSE = 1` 后刷新：打印每一条 SSE 事件。
- 失败的请求会在控制台打印 `method path → status code` 与 `request_id`。
- 阅读页的事件处理是纯函数 `reduceEvent`（`web/src/stores/reader.ts`），可以直接写单测复现。
- 渲染出错会进入路由错误页，显示错误信息与请求编号。

### 3.6 数据

- `yanyi.db` 是普通 SQLite（WAL），可以用任意 SQLite 工具只读打开查看；表结构见 `internal/store/migrations/`。
- 大于 64 KiB 的 blob 在 `blobs/ab/cd/<sha256>`，小的内联在 `blobs` 表。
- 结构迁移前会自动备份到 `backup/`；用旧程序打开新数据会拒绝启动（防止破坏数据）。
- `instance.json` 用于单实例检测；进程异常退出后残留的文件会在下次启动时被识别并覆盖。
- 导入作品是两段式的：解析后的文件暂存在 `tmp/imports/<任务 ID>/`（按内容哈希命名），确认、放弃或 30 分钟后删除，启动时整个清空。
  想知道一个 zip 包为什么导入失败，用 `yanyi work validate <zip 包>`：与导入走同一条流水线（作品根定位、忽略规则、安全限制、校验）。

### 3.7 常见问题

| 现象 | 原因与处理 |
| --- | --- |
| `go run` 启动提示"检测到通过 go run 启动" | 数据目录默认在可执行文件旁，`go run` 的可执行文件在临时目录：加 `--data ./.devdata` |
| 打开 7860 端口看到"前端未构建" | 二进制里没有嵌入前端：开发时访问 5173；发布前先 `task build:web` |
| 请求返回 403"请通过 IP 地址或 localhost 访问" | Host 校验（防 DNS 重绑定）：用 `127.0.0.1` 或 `localhost` 访问 |
| 请求返回 403"跨站请求已被拒绝" | 有副作用的请求 Origin 与 Host 不一致；Vite 代理需保持 `changeOrigin: false` |
| 从仓库根执行 `pnpm -C web …` 报"configured to use yarn" | 上层目录的 `package.json` 声明了别的包管理器：进入 `web/` 再执行，或用 `task` |
| 连接测试"模型列表"一项显示跳过 | 不少兼容接口不提供模型列表，不影响使用；手动填写模型 ID 即可 |
| 回合失败"请求被模型服务拒绝" | 看 `<数据目录>/logs/llm-trace/raw/` 中该次请求的 `.resp.txt`：中转站常把上游故障也返回成 400；用 curl 重放 `.req.json` 可以判断是请求问题还是上游问题 |
| 拉取新代码后启动报迁移错误，或数据表缺列 | 首次发布前 `0001_init.sql` 会被直接修改：删除 `.devdata/` 重建 |
| 接口返回 501，或 AI 说"…暂时不可用" | 功能尚未实现。501 的错误信息里带任务编号；工具的编号写在 `tools/builtins.go` 的 `notYet` 调用中。到 [progress.md](progress.md) 查状态 |
| 提交输入返回 409，或操作提示"正在整理上下文" | 同一存档同时只有一个写者：回合进行中、整理中不能再提交或改存档。整理期间提交的输入会排队（至多一个），整理结束后开始 |

## 4. 测试

```bash
task test
```

- Go：单元测试与竖切测试（`internal/runtime/runtime_test.go` 用假模型跑完整回合）；测试作品 T3 的原语回归（`runtime/t3_test.go`）；
  跨回合前缀稳定性（`runtime/prefix_test.go`）；记忆、原文与检索（`runtime/memory_test.go`）；分支操作与分支隔离（`runtime/branch_test.go`）；
  上下文整理（`runtime/compact_test.go`：阈值触发与排队、separate、超限紧急整理、整理后的前缀）；
  200 回合不变量与整理中断（`runtime/invariants_test.go`，`-short` 时跳过）；
  杀进程测试（`runtime/crash_test.go`，重新执行测试二进制作为子进程）；协议适配器用真实录制的 SSE 夹具回放（`internal/llm/*/testdata/`）。
  测试使用 `t.TempDir()` 中的独立数据库。
- 前端：Vitest（叙事标记与共享向量 `testdata/markup/`、事件 reducer、命令解析、导入、供应商设置）。
- 真实模型冒烟测试（会产生费用，默认跳过）：在 `.local/secrets/live.json`（不入库）写好供应商，
  `YANYI_LIVE=<live.json 的绝对路径> go test ./internal/provider -run TestLive -v`；`YANYI_LIVE_ONLY=名称` 只跑某一个，
  `YANYI_LIVE_TRACE=<目录>` 同时录制原始报文。
- 测试作品评测（会产生费用，默认跳过）：按 `testdata/eval/<作品>.yaml` 的行动序列用真实模型游玩一遍，检查每回合提交、工具调用、
  交还选项、状态维护与面板渲染，并以场外提问检查事实召回（答案写进报告，人工核对）：
  `YANYI_EVAL=<live.json 的绝对路径> YANYI_EVAL_MODEL=<名称> go test ./internal/runtime -run TestEval -v -timeout 60m`；
  报告在 `.local/eval/<时间>/story.md`（`YANYI_EVAL_OUT` 可指定），`YANYI_EVAL_TRACE=<目录>` 录制原始报文。
- 尚缺：Playwright 端到端（M5-12）、测试作品 T2 与真实模型 100 回合评测（M3-11）。完整情况见 [progress.md](progress.md) 第 6 节。

## 5. 构建与发布

```bash
task build
```

- `CGO_ENABLED=0`，纯 Go（`modernc.org/sqlite`），六个目标平台可交叉编译：`task build:cross`。
- 版本号由 `git describe` 注入 `internal/buildinfo`。
- TODO(M6-8)：GoReleaser 与发布说明。

## 6. 代码约定

- 开始一个任务前先看 [progress.md](progress.md) 中该任务的"已有 / 缺口"，再用 `git grep -nE "M3-1([^0-9]|$)"` 列出代码中的全部相关位置；
  做完按该文档顶部的"更新规则"更新进度。

- 注释使用简体中文；尚未完成的地方写 `TODO(任务编号)：说明`，编号对应实施计划（如 `TODO(M3-6)`），文档冲突写 `TODO(待确认)`。
- 注释与普通文档描述当前行为和有明确依据的原因；写法见 [AGENTS.md 的“注释与文档写法”](../AGENTS.md#注释与文档写法)。
- 提交信息：Conventional Commits + 简体中文，例如 `feat(runtime): 回合截断续写`、`fix(web): 选项键盘快捷键冲突`。
- Go：`task fmt` 格式化（gofmt + goimports，本项目的包单独成组）；包级注释写清"职责 / 不负责什么 / 调试方法"。
- 前端：ESLint + Prettier；文案进 `i18n/zh-CN.ts`；`components/ui` 是 shadcn 生成的组件，可以按设计语言修改。
- 修改前后端契约时同步两边（见 [architecture.md](architecture.md) 第 7 节）。

### 6.1 静态检查

```bash
task check
```

提交前运行：`task lint`（全部静态检查）+ `task test`（全部测试），与 CI 相同。`task lint` 包括：

| 部分 | 内容 |
| --- | --- |
| `task lint:go` | `go mod tidy -diff`（go.mod / go.sum 整洁）；golangci-lint：go vet（除 fieldalignment、shadow 外的全部分析器）、staticcheck、errcheck、errorlint、gosec、rowserrcheck / sqlclosecheck、noctx、loggercheck、gofmt / goimports 等，清单与理由写在 `.golangci.yml` |
| `task lint:web` | ESLint、`tsc -b`、Prettier 格式检查 |

- 只启用"发现的问题几乎都值得修"的检查。遇到确属误报的，就地写 `//nolint:<linter> // 原因`（必须写明 linter 与原因，nolintlint 会检查），不要整体关闭检查。
- 错误不能静默丢弃：确实无需处理的写 `_ = f()` 并在注释里说明原因；`defer` 中的关闭错误需要时记日志。
