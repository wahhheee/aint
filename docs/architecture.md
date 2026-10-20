# 架构

本文描述代码的组织方式：分层、依赖方向、关键数据流、前后端契约，以及"想加一个 X 应该改哪里"。
设计依据是 `.local/docs/02-技术方案.md`（下称"技术方案"）；两者冲突时以代码与本文为准，并在
[open-questions.md](notes/open-questions.md) 中记录原因。

## 1. 总览

```
浏览器（React SPA）                                 yanyi 单进程
┌──────────────────────────┐   REST /api/v1   ┌──────────────────────────────────────────────┐
│ 书架 / 作品 / 开局 / 设置   │ ───────────────► │ server ──► runtime ──► contextx ──► llm ──► 模型 API
│ 阅读页（SSE 驱动的草稿回合） │ ◄─────────────── │   │          │  ├────► tools                  │
└──────────────────────────┘   SSE 事件流     │   │          │  └────► panel                  │
                                               │   ▼          ▼                                 │
                                               │ work / save / settings / provider             │
                                               │   ▼                                            │
                                               │ store（SQLite WAL + 内容寻址 blob）            │
                                               └──────────────────────────────────────────────┘
```

## 2. 后端分层与依赖方向

**只允许向下依赖**（由 `go list` 核对过，无环）。想在下层使用上层的能力时，在下层定义接口、由上层实现
（例如 `runtime.ModelResolver` 由 `provider.Service` 实现，`tools.Env` 由回合草稿实现）。

| 层 | 包 | 职责 |
| --- | --- | --- |
| 组合根 | `cmd/yanyi`、`app` | 命令行；数据目录、日志、单实例、**唯一装配所有服务的地方**（`app.wire`） |
| 接口 | `server` | HTTP 路由、中间件、错误格式、SSE、静态资源。只做协议转换，不含业务规则 |
| 编排 | `runtime` | 存档会话（单一写者）、回合循环、预算、取消、提交、事件发布 |
| 引擎 | `contextx` `tools` `panel` `compact` `helper` | 上下文分层构建与按需组装；工具注册与执行；面板渲染；整理（TODO）；后台助手（TODO） |
| 存档 | `save` | 存档、节点树、进度线、建档、回合提交事务、时间线 |
| 领域 | `work` `settings` `provider` `usage` | 作品编译与导入；全局设置；供应商与模型配置、密钥；用量记录 |
| 基础 | `llm`（含适配器子包）`event` `store` `prompts` `tmpl` `state` `markup` `vfs` `media` `pkg/*` `buildinfo` | 中立模型层；事件总线；SQLite 与 blob；平台提示词；作者模板；状态类型；叙事标记；虚拟文件系统；图片 |

几条贯穿性的约定：

- **SQL 写在各业务包的 `repo.go`**，表结构只在 `store/migrations/*.sql`；repo 函数接受 `store.Querier`，调用方决定是否在事务中。
- **业务错误用 `pkg/errs`**：`Message` 是面向玩家的中文，`Details` 放排查信息；`server/respond.go` 统一映射 HTTP 状态码。
- **JSON 一律经 `pkg/jsonx`**（`encoding/json/v2`，确定性序列化）——前缀缓存稳定、内容寻址去重都依赖于此。
- **ID 一律 UUID v7**（`pkg/ids`，标准库 `uuid`），时间有序。
- **作者内容不可信**：作者模板走 `tmpl`（Liquid，限时限量）；平台提示词走 `text/template`（`prompts` + `contextx`）。两者刻意分开。

## 3. 一个回合的生命周期

```
POST /saves/{id}/turns
  └─ runtime.Manager.Submit            会话加锁：同一存档同时只有一个回合（409）；回合 ID = 提交后的节点 ID
       └─ goroutine: runTurn           与 HTTP 请求脱钩；ctx 带 save_id / turn_id 日志字段
            ├─ prepare                 读进度线 → 作品模型 → 解析模型 → 能力画像 → 封装玩家消息 → contextx.Build
            ├─ loop                    llm.Stream → 正文增量（turn.text）→ 工具（tools.Execute）
            │                          → 终止型 offer_choices / 截断续写 / 预算 / 取消 / 错误
            └─ commit                  save.CommitTurn：单事务写节点、消息与显示项 blob、原文索引、用量、head
                                       → turn.committed → 面板重新渲染（panels）
```

- 生成过程只在内存草稿（`runtime/draft.go`）中，进程崩溃不会留下半提交（不变量 2）。
- 玩家停止时以 `interrupted` 状态提交已生成的内容；什么都没生成则不产生节点，只发 `turn.failed`。
- 上一回合以 `offer_choices` 结束时，玩家的选择作为该工具调用的结果回填到下一回合的第一条消息（`contextx/envelope.go`）。

## 4. 上下文工程（`contextx` + `prompts` + `tools`）

```
system : [L0 平台核心（prompts/l0.go 片段）][L1 作品指令（contextx/l1.go）]    tools: [L2（tools/builtins.go）]
user   : [L3 常驻设定] [L4 阶段块（contextx/epoch.go）]  …历史回合（只追加）…  [本回合玩家消息]
```

- **按需组装**：`contextx.Profile` 计算能力画像（作品用到了什么、模型能做什么、存档开了什么）；每个提示词片段、
  每节 L1、每个工具及其参数都声明启用条件，作品没用到的功能不留任何痕迹。节内无片段则整节连同标题省略。
- **前缀稳定**：组装只依赖稳定输入，按 `(提示词版本, 作品版本, 能力签名, 初始状态)` 缓存；`Built.PrefixHash` 记录在节点 meta。
- **可观察**：`Built.Fragments` 列出每个片段的启用情况与原因；`yanyi work prompt <目录>` 在终端打印组装结果。
- 测试：`contextx/builder_test.go`（最小作品的裁切、文本提到的工具都已注册、没有空标题、前缀哈希稳定）。

## 5. 模型适配层（`llm`）

- 上层只认识中立类型（`Request`、`Message`、`Part`、`Event`、`StopReason`、`Usage`）；SDK 类型只出现在 `llm/<协议>` 包内。
- 能力以装饰器叠加（`provider.Service.decorate`）：`WithTrace(WithRetry(适配器))`。
- `llm/fake` 是脚本化假模型：开发、测试、复现问题都用它（`--fake-llm demo` 或脚本文件）。
- 三家真实适配器目前是占位，TODO 中列出了 M0 需要实测确认的要点。

## 6. 前端结构（`web/src`）

```
app/        入口、Provider、路由表（router.tsx 一处看全）、外壳布局、全局事件流
api/        与后端契约一一对应：types.ts、events.ts、errors.ts；client.ts（错误归一）、sse.ts（续传与重连）、queries.ts（查询键工厂）
stores/     Zustand：reader.ts（草稿回合，事件处理为纯函数 reduceEvent）、ui.ts
markup/     叙事标记解析（parse.ts）与渲染（render.tsx）、作品样式标签 CSS（workStyles.ts）
features/   library（书架、线装书封面）、work（作品详情）、save（开局）、reader（阅读页）、settings
components/ ui/（shadcn 生成，可按设计语言定制）、common/、brand/
styles/     globals.css（设计令牌）、reading.css（正文排版）、markup.css（标签样式）
i18n/       zh-CN.ts（全部文案；键名有类型检查）
```

阅读页的数据分工：**已提交的回合**来自时间线接口（权威）；**进行中的回合**是 SSE 驱动的草稿（`stores/reader.ts`）。
提交后时间线刷新，打字机显示完毕再用时间线节点替换草稿，避免内容跳变。

视觉语言的取材与原则写在 `styles/globals.css` 文件头。

## 7. 前后端契约（修改时必须两边同步）

| 契约 | 后端 | 前端 |
| --- | --- | --- |
| REST 数据结构 | 各包导出结构体的 `json` 标签 | `web/src/api/types.ts` |
| SSE 事件 | `internal/event/types.go` | `web/src/api/events.ts`（含 `EVENT_TYPES` 列表） |
| 错误码 | `internal/pkg/errs/errs.go` | `web/src/api/errors.ts` |
| 路由 | `internal/server/routes.go` | `web/src/api/endpoints.ts` |
| 叙事标记语法 | `internal/markup` | `web/src/markup/parse.ts`（TODO：共用 `testdata/markup` 测试向量） |

字段名保持 snake_case 不转换：排查问题时浏览器网络面板、日志、Go 代码看到的是同一个名字。

## 8. 扩展指南

| 想要…… | 改哪里 |
| --- | --- |
| 新增一个 API | `server/routes.go` 登记 → `server/api_*.go` 处理函数 → 业务包服务方法 → `web/src/api/endpoints.ts` + `queries.ts` |
| 新增一个 SSE 事件 | `event/types.go`（常量 + 载荷）→ 发布处 → `web/src/api/events.ts`（类型 + `EVENT_TYPES`）→ `stores/reader.ts` 的 `reduceEvent` |
| 新增一个工具 | `tools/builtins.go`：`Def`（名称、类型、显示策略、可用条件、`Spec` 裁切、`Handle`）；需要存档数据时扩展 `tools.Env` 并在 `runtime/draft.go` 实现 |
| 新增一段平台提示词 | `prompts/l0.go` 加片段（声明 `When` 条件）；新条件在 `prompts/features.go` 加特征、在 `contextx/profile.go` 计算；**递增 `prompts.Version`** |
| 新增作品文件类型或字段 | `work/manifest.go` 或 `work/model.go` → `work/compile.go`（解析 + 校验代码）→ 示例作品 → TODO：JSON Schema |
| 新增数据库表或列 | `store/migrations/NNNN_说明.sql`（编号递增，已发布的脚本不改）→ 对应包的 `repo.go` |
| 新增模型协议 | `llm/<协议>/` 实现 `llm.Provider` → `provider/service.go` 的 `providerFor` 中登记 |
| 新增设置项 | `settings/settings.go`（字段 + 默认值）→ `web/src/api/types.ts` → 设置页 |
| 新增页面文案 | `web/src/i18n/zh-CN.ts`（不在组件里写死中文） |

## 9. 关键不变量与守护位置

| 不变量（技术方案第 25 节） | 代码位置 | 测试 |
| --- | --- | --- |
| 前缀稳定 | `contextx`（只依赖稳定输入、确定性序列化、历史只读） | `contextx/builder_test.go`（TODO：跨回合的字节前缀测试） |
| 原子提交 | `save.CommitTurn`（单事务）、`runtime/draft.go`（生成只在内存） | `runtime/runtime_test.go`（TODO：杀进程测试） |
| 分支隔离 | `save.ProgressLine`（递归 CTE 只取祖先链） | TODO(M3-10) |
| 状态类型正确 | `state.Check` / `state.Set` | `state/schema_test.go` |
| 单一写者 | `runtime.Manager.Submit`（会话锁）+ `app` 单实例检测 + 写连接池单连接 | `runtime/runtime_test.go` |
| 作品不执行 | `tmpl`（Liquid 限时限量）、前端 DOMPurify 白名单（`features/reader/panels/PanelHTML.tsx`） | — |
| 密钥不出后端 | `provider/secrets.go`、`ProviderView.KeyMasked`、`logx.Secret` | — |
| 存档不膨胀 | 游玩只追加节点；存档条目只在 `save.Create` 产生 | — |
