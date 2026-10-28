# 架构

本文描述代码的组织方式：分层、依赖方向、关键数据流、前后端契约，以及"想加一个 X 应该改哪里"。
作品文件契约见[格式参考](../skills/aint-work/references/format.md)，任务状态见[项目进度](progress.md)。

## 1. 总览

```
浏览器（React SPA）                                 aint 单进程
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

| 层     | 包                                                                                                                 | 职责                                                                                                                                         |
| ------ | ------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------- |
| 组合根 | `cmd/aint`、`app`                                                                                                  | 命令行；数据目录、日志、单实例、**唯一装配所有服务的地方**（`app.wire`）                                                                     |
| 接口   | `server`                                                                                                           | HTTP 路由、中间件、错误格式、SSE、静态资源。只做协议转换，不含业务规则                                                                       |
| 编排   | `runtime`                                                                                                          | 存档会话（单一写者）、回合循环、预算、取消、提交、上下文整理的执行、事件发布                                                                 |
| 引擎   | `contextx` `tools` `panel` `compact` `helper`                                                                      | 上下文分层构建与按需组装；工具注册与执行；面板渲染；整理的参数与规则；后台助手的有界隔离工具循环                                             |
| 存档   | `save`                                                                                                             | 存档、节点树、进度线、建档、回合与整理的提交事务、分支操作、玩家编辑、原文、时间线与分支结构                                                 |
| 领域   | `work` `settings` `provider` `usage` `access`                                                                               | 作品编译与导入；全局设置；供应商与模型配置、密钥、内置模型预设、连接测试；用量记录与统计（按存档、作品、节点，分用途与模型，按货币分别求和） |
| 基础   | `llm`（含 `wire` 与适配器子包）`event` `store` `prompts` `tmpl` `state` `markup` `vfs` `media` `pkg/*` `buildinfo` | 中立模型层与三协议适配器；事件总线；SQLite 与 blob；平台提示词；作者模板（隔离工作进程）；状态类型；叙事标记；虚拟文件系统；图片             |

几条贯穿性的约定：

- **SQL 写在各业务包的 `repo.go`**，表结构只在 `store/migrations/*.sql`；repo 函数接受 `store.Querier`，调用方决定是否在事务中。
- **业务错误用 `pkg/errs`**：`Message` 是面向玩家的中文，`Details` 放排查信息；`server/respond.go` 统一映射 HTTP 状态码。
- **JSON 一律经 `pkg/jsonx`**（`encoding/json/v2`，确定性序列化）——前缀缓存稳定、内容寻址去重都依赖于此。
- **ID 一律 UUID v7**（`pkg/ids`，标准库 `uuid`），时间有序。
- **作者内容不可信**：作者模板走 `tmpl`（Liquid，服务端在可终止的隔离工作进程中渲染，D-46）；平台提示词走 `text/template`（`prompts` + `contextx`）。两者刻意分开。

## 3. 一个回合的生命周期

```
POST /saves/{id}/turns
  └─ runtime.Manager.Submit            会话加锁：同一存档同时只有一个回合（409）；回合 ID = 提交后的节点 ID
       └─ goroutine: runOperation           与 HTTP 请求脱钩；ctx 带 save_id / turn_id 日志字段
            ├─ prepare                 读进度线 → 作品模型 → 解析模型 → 能力画像 → 封装玩家消息 → contextx.Build → 挂载 VFS
            ├─ loop                    llm.Stream → 正文增量（turn.text）→ 工具（tools.Execute：read / list 经 VFS）
            │                          → 终止型 offer_choices / 截断续写 / 预算 / 取消 / 错误
            └─ commit                  save.CommitTurn：单事务写节点、消息与显示项 blob、原文索引、用量、head
                                       → 让出写者槽位 → turn.committed → 面板重新渲染（panels）
```

- 生成过程只在内存草稿（`runtime/draft.go`）中，进程崩溃不会留下半提交（不变量 2）。
- 记忆写入（write / edit / remove）进回合的覆盖层（`vfs.Overlay`），提交时有改动才生成新清单；`/transcript` 与检索只看进度线（不变量 3）。
- 写者槽位在发布任何终态事件（`turn.committed` / `turn.failed`）**之前**让出：客户端收到终态事件后立即提交下一回合不会得到 409。
- 玩家停止时以 `interrupted` 状态提交已生成的内容；什么都没生成则不产生节点，只发 `turn.failed`。
- 回合从准备阶段起受墙钟时限约束（默认 10 分钟，排队不计入），超时取消模型请求与流读取，保留已有正文并释放写者；后台整理也有同等时限。中断回合不触发自动整理。
- 上一回合以 `offer_choices` 结束时，玩家的选择作为该工具调用的结果回填到下一回合的第一条消息（`contextx/envelope.go`）。选项附带命令时，输入同时保留 `choice_id` 与命令信息；服务端校验选项、补全选项文字，并在选择结果之后发送命令展开后的输入。独立使用命令、场外或继续时，工具结果说明玩家另有输入。
- 静态开场的助手历史包含正文与 `offer_choices` 调用，选项参数取自开场文件；首轮玩家输入沿用工具结果回填。开场正文不包含平台拼接的文字选项列表。
- 作品以 `openings` 声明开场列表与各自初始状态覆盖；存档保存所选 ID、有效开局表单和永久锁定标记。开场后的首个成功玩家回合（含场外）在提交事务中锁定开场；开场及其中断续写不锁定。切换经现有写者槽位重建根节点与开场，清除试读分支、保留用量，推送 `timeline.changed`。格式、状态优先级与接口见 [多开场](../skills/aint-work/references/format.md#开场-openings)。
- 作品可开启 `features.force_choices`：正文自然结束而没有选项时，追加一次提示并继续当前工具循环；补调用沿用回合预算、取消与用量记录。纯场外回应不触发，再次自然结束仍缺少选项时以中断状态保留正文。配置与边界见 [正文结束后交还选项](../skills/aint-work/references/format.md#正文结束后交还选项)。
- 正文中的可用工具名 XML 标签由 `runtime/tool_text.go` 在流式显示前隔离；运行时拒绝同条回复的全部工具操作，回填错误结果并最多纠正两次，由模型重新发起原生调用。`tool_history.go` 在后续请求的历史副本中去掉伪标签、退回的调用与纠正提示，保留成功调用及其结果；清理位置起的 Native 与推理一并移除。原始消息仍保留在存储与检查器中；普通叙事标签、代码引用与作品同名样式不受影响。失败保留正文并中断，边界与验证见 [正文伪工具调用](notes/tool-text-recovery.md)。

**上下文整理**（`runtime/compact.go`、`compact_job.go`，参数与规则在 `compact`）：

```
回合提交 ── context_tokens ≥ 阈值 ──► 写者槽位交给后台整理（期间的输入排队，至多一个）
回合进行中 ── 估算 ≥ 硬上限 / 模型报超限 ──► 回合内整理（整理节点成为本回合的父节点，每回合至多一次）
POST /saves/{id}/compact、/compact ──► 后台整理
  compactLine：当前上下文 + 整理指令 → 落盘循环（只能读资料、读写记忆）→ 交接摘要 → save.CommitCompaction
               （阶段块 + keep_from + 落盘后的记忆清单，成为 head）→ compaction.* 事件
```

之后的请求由 `runtime/history.go` 组装：最近一次整理的阶段块 + 保留的回合（去掉推理与协议原生内容）+ 之后的回合。

**分支与编辑**（`save/branch.go`、`edit.go`、`tree.go`）：重新生成、修改玩家输入经 `runtime.Regenerate` 以原回合的父节点提交新回合；
移动 head、修改正文、丢弃、删除分支、编辑记忆或状态经 `runtime.Exclusive`（有任务在运行时 409），完成后推送 `timeline.changed`。

### 开场切换

`saves.opening_id` 保存所选开场，`opening_locked` 保存不可逆的锁定状态，`initial_form` 保存有效开局表单。锁定与成功的玩家回合在同一事务提交；开场及其续写通过节点 `meta.opening` 标记。手动修改正文不产生首次模型交互。

创建存档可提交 `opening_id`，省略时取第一项。作品详情返回 `openings` 摘要列表；阅读页呈现信息返回 `opening.options`、`can_switch`、`has_unfinished_turn`，由服务端判断是否允许切换。

`PUT /api/v1/saves/{id}/opening` 接收 `opening_id` 与 `expected_head_id`。运行器占用现有单写者槽位，存档层先校验并渲染，再在事务中重建初始节点与开场；锁定、任务忙碌或页面的 head 过期时返回冲突。切换完成发送 `timeline.changed`（`reason: opening`）及最新面板，前端清除旧草稿并重置时间线查询。

生成式开场按当前起点自动提交 `kind: opening` 和 `expected_head_id`。多个阅读页同时提交时，冲突的页面重新同步会话，不展示自动开场失败。原文、状态、记忆和背景只使用新开场的进度线。

### 重写的上下文与提交

`runtime.Manager.Operate` 读取原输入，使用原回合的父节点准备上下文、状态、记忆与原文检索。旧版正文和选项通过临时 `rewrite_request` 传给执笔者，只供理解修改要求。原回合之后的编辑和整理仍属于原分支，不带入新版。

执笔者先维护新版事实对应的状态和记忆，再输出正文。若本轮写了记忆，提交前还会以实际成稿核对这些记忆；核对不携带旧版、修改要求或生成计划，只能查资料和修订记忆，不能修改状态或推进故事。需要修改的文件由模型批量修订，成功后结束；最多四次调用，仍受整个操作的调用数、费用和时长预算限制。核对出错或取消，整次重写回滚。工具执行后的提示说明正文会依次拼接，避免重复全文或输出完成汇报。模型的普通正文不做关键词删改；协议推理仅用于本次调用和调试。

新节点保存原玩家输入、生成的正文、成功的工具往返与新快照。旧文、重写要求和临时工具返回提示不进入后续故事历史；修改过上下文的助手消息去掉推理和协议 Native。操作元数据保留在节点与检查器中供排查。

所有写入在草稿覆盖层中执行，成功时单事务提交。回合内紧急整理先暂存不可变 blob，不移动进度；整理节点与新版一起提交，失败时不留下分支节点。逻辑版本父节点用于把经过整理的新版仍归到同一组版本。未采用操作的实际模型用量仍计入存档。

### 补选项的只读边界

补选项使用当前进度的故事上下文与状态。请求沿用已有工具定义以保持前缀稳定，执行器只放行 `read`、`list`、`search`、`get_state` 与 `offer_choices`。状态修改、记忆写入、骰子、后台助手等调用在执行前拒绝。

普通文字保留在调试记录中，不推送到正文，也不存入故事历史。成功提交一个 `choices` 附属节点：回合号、状态哈希、记忆哈希均沿用当前进度，正文时间线不增加回合。时间线按当前分支把选项附到目标回合；回退到补选项前不会带出其他分支的选项。

后续上下文把有效 `offer_choices` 调用并入目标回合，玩家的选择作为工具结果回填。复制存档重映射目标 ID；手动编辑正文保留当前分支的附属选项及对应工具调用。整理按故事回合分组保留或摘要这段历史。

### 契约与代码入口

| 入口 | 行为 |
| --- | --- |
| `POST /saves/{id}/nodes/{nid}/revise` | `{instructions, expected_head}`，返回异步回合 ID |
| `POST /saves/{id}/nodes/{nid}/choices` | `{expected_head}`，返回异步操作 ID |
| `turn.pending` / `turn.started` / `session.snapshot` | 可选 `operation` 携带类型、目标、要求与期望进度，刷新可恢复草稿与取消操作 |
| `turn.committed` / `turn.failed` | 采用新版或丢弃操作；补选项成功同时发 `timeline.changed` |

路由省略共同前缀 `/api/v1`。运行入口在 `internal/runtime/operation.go`，提交与时间线在 `internal/save/`，前端入口在 `useTurnActions.ts`、`TurnToolbar.tsx` 与 `Stream.tsx`。

## 4. 上下文工程（`contextx` + `prompts` + `tools`）

```
system : [L0 平台核心（prompts/l0.go 片段）][L1 作品指令（contextx/l1.go）]    tools: [L2（tools/builtins.go）]
user   : [L3 常驻设定] [L4 阶段块（contextx/epoch.go）]  …历史回合（只追加）…  [本回合玩家消息]
```

- **按需组装**：`contextx.Profile` 计算能力画像（作品用到了什么、模型能做什么、存档开了什么）；每个提示词片段、
  每节 L1、每个工具及其参数都声明启用条件，作品没用到的功能不留任何痕迹。节内无片段则整节连同标题省略。
- **工具用法**：L0 说明原生工具调用与正文输出的边界，并由本次请求的工具定义生成逐项自然语言用法（`contextx.RenderToolGuide`）；名称、用途与主要参数说明共用工具注册表，完整参数约束仍通过 API schema 发送。独立整理只列出其可用的资料与记忆工具。说明文字计入提示词 token 估算。
- **前缀稳定**：组装只依赖稳定输入，按 `(提示词版本, 作品版本, 能力签名, 初始状态, 图片尺寸上限, 工具定义哈希)` 缓存（64 条 LRU）；`Built.PrefixHash` 记录在节点 meta。历史伪调用的清理是确定性的，但修改位置起的历史缓存不能沿用；它不改变稳定层的 `PrefixHash`。
- **阶段块（L4）**：第 0 阶段由初始状态与初始记忆即时渲染；整理后的阶段块在整理时渲染、存为整理节点的 blob，阶段内原样复用（`contextx/epoch.go`）。
- **缓存断点**：`Part.CacheMark` / `SystemBlock.CacheMark` 只标稳定层的边界（L1、L3、L4 末尾）；"最新消息末尾"的滚动断点由适配器按协议放置
  （Anthropic 显式 `cache_control`、OpenAI GPT-5.6 起的隐式断点、Gemini 隐式缓存）。Anthropic 新增内容达到 20 个回溯位置时，
  增加前一次 user 消息末尾的历史锚点；四个断点优先保留系统、历史锚点与最新末尾。thinking 块不能显式加断点。
- **可观察**：`Built.Fragments` 列出每个片段的启用情况与原因；`aint work prompt <目录>` 在终端打印组装结果。
- 测试：`contextx/builder_test.go`（最小作品的裁切、文本提到的工具都已注册、没有空标题、前缀哈希稳定）。

## 5. 模型适配层（`llm`）

```
runtime ──► llm.Provider（中立类型）
              WithTrace(WithRetry(适配器))          provider.Service.decorate
                 ├─ llm/anthropic   Messages        ┐
                 ├─ llm/openai      Responses       ├─ 只做"中立类型 ⇄ 协议 JSON"的转换（D-44：标准库手写，不用 SDK）
                 ├─ llm/gemini      generateContent ┘
                 │     └─ llm/wire  HTTP 端点、SSE 读取、错误归一、原始报文追踪
                 └─ llm/fake        脚本化假模型
```

- 上层只认识中立类型（`Request`、`Message`、`Part`、`Event`、`StopReason`、`Usage`）；协议 JSON 只出现在 `llm/<协议>` 包内。
- **Native**：助手消息的协议原生内容（Anthropic 内容块含 thinking 签名、Responses 输出项含加密推理、Gemini 部件含 thoughtSignature）
  按 `"<协议>/<模型>"` 存在 `Message.Native`；同协议同模型且历史未改写时原样回传，否则按中立内容重建（推理丢弃）。
- **工具历史**：适配器构建请求前通过 `llm.RepairToolHistory` 修复发送副本：没有结果的调用丢弃，孤儿结果展开为文本与图片；
  修改位置及其后的原生内容与推理丢弃。完整历史保持不变，存储消息不被修改。Gemini 缺少调用 ID 与响应 ID 时为本次响应分配独立命名空间。
  Gemini 在合并相邻用户消息后，将普通文字与独立图片放在 `functionResponse` 之前，各组内部顺序不变；工具结果内嵌图片与助手原生签名保持不变。
- **暂停**：Anthropic `pause_turn` 把原生响应随历史再次发送，不追加续写提示，仍受回合调用次数与时长预算限制。
- **断流用量**：Anthropic / Gemini 流式错误可携带供应商已报告的累计用量，正文续写的回合会记录这部分用量，检查器也显示已知值。
  没有最终用量时不估算缺失 token；费用只能反映已报告的部分，不能当作完整账单。
- **模型预设**：`provider/presets.go` 随应用维护，按模型前缀与接入协议匹配；`GET /models/{id}/preset` 返回预设版本、能力参数与官方价格，
  无匹配时返回 `null`。模型设置分别将能力或价格填入表单，经现有 `PATCH /models/{id}` 保存。已配置模型不会自动被覆盖。
- **能力矩阵**（`llm.Caps`）决定请求怎么写：`Reasoning` + `ReasoningMap`（推理档位 → 协议参数，如 Claude Sonnet 5.5 的 off 写作
  `between_tools`）、`Cache`（breakpoints / implicit / none）、看图与工具结果含图。内置预设在 `provider/presets.go`，添加模型时按名字预填。
- 请求体由 `jsonx` 确定性序列化，每个适配器导出 `Body()` 便于测试与前缀比对。
- 调试：`--trace-llm` 写中立追踪（`logs/llm-trace/`）与协议原始报文（`logs/llm-trace/raw/`，鉴权头脱敏，可直接做回放夹具）。
- 游玩检查器：`runtime/inspect.go` 按存档控制内存记录；`llm.ObserveStream` 包住供应商与重试层，`wire` 通过 context 中的观察器上报各次 HTTP 尝试。
  记录结构化上下文、明文推理、输出、工具参数与结果、状态前后值；`vfs.Table.Snapshot` 固定文件清单并复制可写记忆，之后按需读取文件。
  `GET/PATCH/DELETE /saves/{id}/debug` 管理开关与记录列表；`GET .../debug/calls/{cid}` 获取详情，`/file` 读取快照，`/images/{hash}` 只提供该调用实际引用的图片。
  调试响应不缓存，记录最多 100 次 / 64 MiB，单次 8 MiB；HTTP 头与 URL 的鉴权字段进入观察器前脱敏，正文内容不改写。
  前端只轮询轻量列表，打开面板后按修订号读取当前详情；调试内容不进入游玩 SSE 事件总线，也不写入存档。
- 测试：真实录制的 SSE 夹具回放（`llm/*/testdata/`）；真实调用冒烟测试 `provider/live_test.go`（默认跳过，D-45）；实测结论见
  [api-verification.md](notes/api-verification.md)。
- 供应商与模型：`provider` 管理增删改、密钥（`secrets.json`）、价格倍率（运行时解析模型时乘好，D-48）、连接测试（模型列表 → 文本 →
  工具往返 → 看图）。

## 6. 前端结构（`web/src`）

```
app/        入口、Provider、路由表（router.tsx 一处看全）、外壳布局、全局事件流
api/        与后端契约一一对应：types.ts、events.ts、errors.ts；client.ts（错误归一）、sse.ts（续传与重连）、queries.ts（查询键工厂）
stores/     Zustand：reader.ts（草稿回合，事件处理为纯函数 reduceEvent）、ui.ts
markup/     叙事标记增量解析（parse.ts）与渲染（render.tsx）、作品样式标签 CSS（workStyles.ts）
features/   library（书架、线装书封面、导入）、work（作品详情）、save（开局）、reader（阅读页）、settings
components/ ui/（shadcn 生成，已按设计语言定制尺寸与配色）、common/（PageHeader 页面骨架、ThemeSwitch 外观切换等）、brand/
styles/     globals.css（设计令牌）、reading.css（正文排版）、markup.css（标签样式）
i18n/       zh-CN.ts（全部文案；键名有类型检查）
```

阅读页的数据分工：**已提交的回合**来自时间线接口（权威）；**进行中的回合**是 SSE 驱动的草稿（`stores/reader.ts`）。
提交后时间线刷新，打字机显示完毕再用时间线节点替换草稿，避免内容跳变。

阅读页的提示、正文、选项与输入栏共用 `.reader-rail` 内的 `.reader-column`，在两侧栏之间的可用区域居中；
版心宽度以阅读容器计算，正文滚动条双侧预留位置。1200 px 起目录与作品侧栏参与布局并使用相同的宽度过渡；
目录桌面开关保存在本机 `stores/ui.ts`，窄屏使用独立状态的模态侧滑抽屉（`chrome/TurnIndex.tsx`），收起后恢复入口焦点。

存档事件流首次连接或续传缓冲不足时发送 `session.snapshot`，包含进行中的正文、父节点、输入、文本序号、工具、用量、助手和整理状态。
`runtime/session_snapshot.go` 从已发布事件维护会话投影，快照与订阅在同一锁内取得，快照事件 ID 标记增量起点；空闲时投影被回收。
`turn.pending` 表示输入已经接受、正在准备或排队，前端可用回合 ID 停止。前端收到首次快照后才允许提交；409 显示原因并重新取得会话快照，
空闲快照清除遗留草稿并刷新时间线。正常短暂断线沿用事件续传；消费过慢触发 `resync` 时重建连接。

视觉语言的取材与原则写在 `styles/globals.css` 文件头。外壳（`app/ShellLayout.tsx`）宽屏是左侧栏 + 内容区，窄屏是顶栏 + 内容区；
整个外壳不滚动，只有内容区滚动（`.scroll-stable` 预留滚动条位置），新页面以 `PageHeader` 开头，标题行与侧栏品牌同一水平线。
加页面时沿用这两点，版面在切换页面、标签时就不会移动。

## 7. 前后端契约（修改时必须两边同步）

| 契约          | 后端                         | 前端                                                                         |
| ------------- | ---------------------------- | ---------------------------------------------------------------------------- |
| REST 数据结构 | 各包导出结构体的 `json` 标签 | `web/src/api/types.ts`                                                       |
| SSE 事件      | `internal/event/types.go`    | `web/src/api/events.ts`（含 `EVENT_TYPES` 列表）                             |
| 错误码        | `internal/pkg/errs/errs.go`  | `web/src/api/errors.ts`                                                      |
| 路由          | `internal/server/routes.go`  | `web/src/api/endpoints.ts`                                                   |
| 叙事标记语法  | `internal/markup`            | `web/src/markup/parse.ts`（两端运行同一份测试向量 `testdata/markup/*.json`） |

字段名保持 snake_case 不转换：排查问题时浏览器网络面板、日志、Go 代码看到的是同一个名字。

首次发布前契约不做向后兼容（D-43）：改名、删字段、改结构直接改，两边在同一次改动中同步，不保留旧字段、旧路由或别名。

## 8. 扩展指南

| 想要……                   | 改哪里                                                                                                                                                                 |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 新增一个 API             | `server/routes.go` 登记 → `server/api_*.go` 处理函数 → 业务包服务方法 → `web/src/api/endpoints.ts` + `queries.ts`                                                      |
| 新增一个 SSE 事件        | `event/types.go`（常量 + 载荷）→ 发布处 → `web/src/api/events.ts`（类型 + `EVENT_TYPES`）→ `stores/reader.ts` 的 `reduceEvent`                                         |
| 新增一个工具             | `tools/builtins.go`：`Def`（名称、类型、显示策略、可用条件、`Spec` 裁切、`Handle`）；需要存档数据时扩展 `tools.Env` 并在 `runtime/draft.go` 实现                       |
| 新增一段平台提示词       | `prompts/l0.go` 加片段（声明 `When` 条件）；新条件在 `prompts/features.go` 加特征、在 `contextx/profile.go` 计算；**递增 `prompts.Version`**                           |
| 新增作品文件类型或字段   | `work/manifest.go` 或 `work/model.go` → `work/compile.go`（解析 + 校验代码）→ 示例作品 → `work/schemas.go` → `go run ./cmd/schema-gen`                                                     |
| 新增数据库表或列         | 首次发布前直接改 `store/migrations/0001_init.sql`（D-43）；发布后追加 `NNNN_说明.sql`（编号递增，已发布的脚本不改）→ 对应包的 `repo.go`                                |
| 新增模型协议             | `llm/<协议>/` 实现 `llm.Provider`（HTTP 与 SSE 用 `llm/wire`，导出 `Body()`）→ `provider/service.go` 的 `NewAdapter` 中登记 → 录制夹具与 `live_test.go` 子测试         |
| 新增内置模型预设         | `provider/presets.go`（窗口、输出上限、能力与推理映射、官方价格；同一模型经不同协议接入时用 `ReasoningBy`）                                                            |
| 新增 VFS 挂载点          | 实现 `vfs.Source`（`Files` / `Open`）→ `runtime/vfs.go` 的 `buildVFS` 中挂载；可写的实现 `vfs.Writer`，能自行检索的实现 `vfs.Searcher`；暂不可用的挂载点 `Source` 留空 |
| 新增一个直接改存档的操作 | `save` 中实现（单事务）→ `server` 处理函数中经 `Runtime.Exclusive` 调用（保证单一写者，完成后推送 `timeline.changed`）→ 前端 `useTurnActions` 或抽屉                   |
| 新增设置项               | `settings/settings.go`（字段 + 默认值）→ `web/src/api/types.ts` → 设置页                                                                                               |
| 新增页面文案             | `web/src/i18n/zh-CN.ts`（不在组件里写死中文）                                                                                                                          |

## 9. 关键不变量与守护位置

测试的完成情况见 [progress.md](progress.md) 第 6 节。

| 不变量 | 代码位置                                                                                                                                                                                                                                                                               | 测试                                                                                                                          |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| 前缀稳定                   | `contextx`（只依赖稳定输入、确定性序列化、历史只读）；正常回合只追加；伪调用清理与孤儿调用修复仅改发送副本，并使修改位置后的签名失效 | `contextx/builder_test.go`、`runtime/prefix_test.go`（正常回合前缀）、`runtime/tool_history_test.go`（确定性清理与存储不变） |
| 原子提交                   | `save.CommitTurn`（单事务）、`runtime/draft.go`（生成只在内存）                                                                                                                                                                                                                        | `runtime/runtime_test.go`、`runtime/crash_test.go`（回合各阶段杀进程：首字前、流式中、工具中、提交事务中、COMMIT 时、提交后） |
| 分支隔离                   | `save.ProgressLine`（递归 CTE 只取祖先链）；记忆以 head 的清单为底；`/transcript` 与检索按进度线过滤                                                                                                                                                                                   | `runtime/branch_test.go`、`runtime/invariants_test.go`                                                                        |
| 状态类型正确               | `state.Check` / `state.Set`                                                                                                                                                                                                                                                            | `state/schema_test.go`                                                                                                        |
| 单一写者                   | `runtime.Manager`（会话锁：回合与整理共用一个写者槽位，整理期间的输入排队；`Exclusive` 包住直接改存档的操作）+ `app` 单实例检测 + 写连接池单连接                                                                                                                                       | `runtime/runtime_test.go`、`runtime/compact_test.go`                                                                          |
| 作品不执行                 | `tmpl`（隔离工作进程、超时即杀、render 只读作品 ui/ 模板、输入输出上限）、导入安全限制（`work/archive.go`、`fileset.go`）、前端 DOMPurify 白名单与 style 过滤（`features/reader/panels/PanelHTML.tsx`、`sanitize.ts`）、素材只按媒体类型提供并带 CSP sandbox（`server/api_assets.go`） | `tmpl/tmpl_test.go`、`work/archive_test.go`、`server/api_assets_test.go`、`features/reader/panels/PanelHTML.test.tsx`         |
| 密钥不出后端               | `provider/secrets.go`（0600）、`ProviderView.KeyMasked`、额外请求头不允许放密钥、`logx.Secret`；原始报文追踪中鉴权头脱敏（`llm/wire/trace.go`）；数据目录新建目录 0750、日志与追踪文件 0600（`fsx.DirPerm` / `PrivateFilePerm`）；真实调用测试的密钥只放 `.local/secrets/`             | `provider/service_test.go`（掩码、secrets.json）                                                                              |
| 存档不膨胀                 | 游玩只追加节点；存档条目只在 `save.Create` 与"复制为新存档"产生                                                                                                                                                                                                                        | `runtime/invariants_test.go`                                                                                                  |

## 10. 运行参数与后台任务

- 阅读页输入栏分别提供模型 / 推理与预算入口；篇幅直接保存为存档设置，供自由输入与选项回合共用。预算为空时继承全局设置，调用上限的运行时预设通过 `system/info.turn_defaults` 提供。
- `runtime/settings.go` 使用同一存档写者槽位更新设置，拒绝回合 / 整理期间的修改。回合开始冻结预算，优先级为存档覆盖、全局设置、默认值。模型能力与提示词在下一回合重建。
- `turn.failed` 在失败正文已提交时携带 `node_id` 与 `parent_id`；前端以重新生成为重试，不向当前 head 重交已消费的选项。
- `helper.Run` 只负责有界循环；运行时注入模型调用与工具执行。research 只读，curate 的记忆写入当前回合覆盖层；不提供状态、选项与递归委派工具。助手用量进入本回合 `usage_log`，状态通过 `helper.progress` 推送。
- `runtime/title.go` 在进程上下文下生成标题；`save.CommitTitle` 记录用量并比较任务开始时的标题、来源及 head，避免覆盖并发命名或新进度。自动生成按 head 去重，事件为 `save.titled` / `save.title_failed`。
- 前端舞台从显示项推导，音效只播放当前草稿的新标记；资源只取作品素材表。回合目录复用分页时间线；已加载回合的检索与跳转不发起模型调用。

## 11. 分发、开发目录与访问控制

- 远程导入由 `work/remote.go` 下载至暂存目录，继续使用 `Importer` 的预览与确认流程。GitHub 先解析 ref 为提交 SHA，再下载固定提交归档；URL 使用 ETag 和文件树哈希检查更新。HTTPS、逐跳重定向校验、解析后的公网地址直连、流式大小上限与超时共同约束网络请求，GitHub 令牌只发往 API 主机。
- `work/dev.go` 递归监听开发目录，400 ms 去抖后编译，只有校验通过才替换唯一开发版本与缓存。正在运行的回合继续持有旧 Model；后续回合使用新内容。前缀缓存键包含文件树哈希，状态按 D-69 迁移；全局 `work.reloaded` 刷新作品、呈现信息与面板。校验错误保存报告并保留上次可用文件。
- `runtime.Upgrade` 占用存档写者槽位，按旧版本整理后构建新阶段块；`CommitCompaction` 在同一事务中提交状态、版本、迁移记录和 head。没有待整理回合时复用摘要或迁移初始状态，不调用模型。`setHead` 同步存档的版本绑定；单独删除版本时，存档和历史节点保护它们引用的版本。
- `work.Delete` 删除整部作品时，在同一事务内清除全部存档的原文索引、存档和作品，节点、版本及作品文件级联删除；前端说明删除范围、当前存档数与不可撤销性并确认。作品删除由 `runtime.WhenIdle` 串行执行，原始文件夹保留。
- `save.CollectStorage` 在写事务内从作品文件、所有历史节点、近期上传开始递归标记 blob，保护最近一小时的新内容；磁盘回收在数据库删除提交后再次核对引用。手动清理和删除后清理由 `runtime.WhenIdle` 防止与活跃生成竞争，并清空含派生图引用的缓存。启动时至多每周扫描一次。`media.CollectCache` 清除孤立缩略图。
- `access` 通过注入的 Secrets 接口保存 PBKDF2 密码哈希与会话令牌哈希，不依赖 provider。HTTP 业务 API（包括素材、上传、调试和事件流）统一检查会话；登录页静态资源、登录状态和单实例健康端点公开。密码变化或会话撤销关闭已打开的事件流，会话到期也会结束连接。Host 与跨站检查先于鉴权。监听地址在 settings，密码和 GitHub 令牌在 secrets.json。
- 发布与离线 Schema 见[发布构建](development.md#5-构建与发布)与[编辑器配置](../schemas/README.md)。
