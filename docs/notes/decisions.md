# 决策记录：脚手架评审（2026-10-20）

本文记录当前决策及其代码位置。
编号与 `.local/docs/README.md` 的决策记录一致（D-23～D-52）。

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-23 | `presentation.text_effects` 默认关闭；`features.ooc` 默认开启 | `internal/work/manifest.go` |
| D-24 | 平台默认的记忆目录约定写在 L0"记忆"节；作品 `memory.md` 在 L1 补充，作品写明替换时以作品为准 | `internal/prompts/l0.go` |
| D-25 | 有 `opening.md` 未写 `mode` 时为 `static`；命令未写 `present` 时为 `[slash]` | `internal/work/compile.go` |
| D-26 | 叙事标记每个非空行为一个段落 | `web/src/markup/parse.ts` |
| D-27 | 选项 `style` 只取作品样式标签，不内置 `danger`/`subtle`；作品有样式标签时才有该参数 | `internal/tools/builtins.go` |
| D-28 | `<cg>` 只在有非背景图片素材时启用；背景图只用于 `<scene bg>` | `internal/contextx/profile.go` |
| D-29 | 选项待答时玩家另有输入（命令 / 场外 / 继续）：工具结果如实写"没有从选项中作答"，输入照常跟在后面 | `internal/contextx/envelope.go` |
| D-30 | 内置标签 `<ooc>`（随 `features.ooc`）：AI 的场外回应写在其中，浅色块显示，不进入原文检索 | `prompts/l0.go`、`markup/`、`web/src/markup/` |
| D-31 | **开发目录**用于作品链接本地目录；**开发者选项**是设置开关，默认关，开启后显示检查器、回合检查、完整状态与状态编辑，开发目录作品默认显示。状态编辑属于开发者工具，只在显示开发者工具时可用 | `internal/settings`、阅读页 |
| D-32 | `engine` 比较独立的**作品能力版本**（程序内置，目前 `1.0`），与程序发布版本解耦 | `internal/work/engine.go` |
| D-33 | 静态开场节点存两条消息：合成的 user `<input kind="opening">（开场）</input>` + 助手开场正文，历史始终以 user 开头 | `internal/save/service.go` |
| D-34 | 模型价格带货币；费用按货币分别汇总，不做汇率换算 | `internal/llm/usage.go`、`usage_log.currency` |
| D-35 | AI 读取设定文档时把 `[[id]]` 补全为路径（不改作品文件）；校验器检查链接目标 | `internal/work/links.go` |
| D-36 | 作者模板中的 `turn` 是对象，`turn.no` 为回合序号 | `internal/panel`、`internal/runtime/turn.go` |
| D-37 | 开发版本是"树哈希即版本身份"的例外：每部作品至多一个，原地更新，不参与去重 | `store/migrations/0001_init.sql`（部分唯一索引） |
| D-38 | 视觉语言取材线装书：宣纸白、墨、磁青、泥金；书架以线装书封面呈现 | `web/src/styles/globals.css`、`features/library/WorkCover.tsx` |
| D-39 | `GET /events`、`GET /saves?limit=`、`GET /saves/{id}` 返回呈现信息；回合 ID 即节点 ID；前端自动提交生成式开场；第 0 阶段块与执笔指令即时渲染；内置常用图标子集；TypeScript 6.0 | 见 `docs/architecture.md` |
| D-40 | 运行时代码 Apache-2.0；示例作品单独许可（《雾港来信》CC-BY-NC-4.0） | `LICENSE` |

## 界面设计（2026-10-21）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-41 | 夜读（深色）使用**纯黑底**（`#000`），面与浮层用略亮一两级的黑加发丝线区分；阅读页有舞台背景时铺背景图并压暗四周。外壳为宽屏左侧栏（品牌、导航、作品目录）+ 唯一滚动的内容区，内容区始终预留滚动条位置；页面标题固定在同一位置。根字号 16px，尺寸落在 4px 网格上，字号阶梯 12 / 13 / 14 / 16 / 20 / 28 / 36。主操作用墨色（深色时为暖白），泥金只标记"当前选中"；选中态只改颜色与底色、不改字重 | `web/src/styles/globals.css`、`app/ShellLayout.tsx`、`components/common/PageHeader.tsx` |
| D-42 | 标志为**月洞门**：纯黑圆角方块是墙，墙上一扇被门槛截平的圆门，门里是黄昏（深靛的天、地平线上一线金光、一轮小月、山脚隐入雾中的两层远山），门里的光洒在门槛上——走进一个故事，就像穿过园林里的一扇月洞门。不用文字。图形只有一份 `web/public/favicon.svg`，界面里的品牌标识直接引用它；另有 180px 满版的 `apple-touch-icon.png` | `web/public/favicon.svg`、`components/brand/BrandMark.tsx` |

## 开发阶段约定（2026-10-21）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-43 | 首次发布前**不考虑向后兼容**。前后端在同一仓库、随同一个可执行文件发布，彼此是唯一的使用方：契约（types / events / errors / routes）直接改名、删字段、改结构，前后端在同一次改动中一起改；不保留旧字段、旧路由或别名，不加 API 版本号，不写"旧格式也接受"的分支与降级逻辑。程序自己的数据同理：表结构直接改 `0001_init.sql`（K1），本地数据删 `.devdata/` 重建；设置、作品格式改定义时同步改示例作品与测试。**不包括**产品功能中的版本机制：作品 `format` / `engine` 检查、存档绑定作品版本、存档升级（M6-6）是需求本身，照常实现。首次发布后再定兼容策略。原因：没有需要兼容的旧版本，兼容代码只会增加分支、测试与理解成本 | `AGENTS.md` |

另：D-08 规定 Liquid 用于开场、执笔指令（只引用初始状态）、命令文本与面板；平台不设条件表达式类字段，模板内部的 `if`/`for` 不受此限。

## M0 / M1 实现（2026-10-22）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-44 | 三协议适配器用**标准库手写**（`net/http` + 自写 SSE 解析），不使用官方 SDK；回合运行器自研，不引入 Agent 框架（D-06）。原因：请求体由 `jsonx` 确定性序列化，前缀字节完全可控；`Native` 原样存取协议内容块（推理签名、带签名的空文本都不丢）；中转站的非标字段与新特性（`between_tools`、`prompt_cache_options` 等）不受 SDK 版本限制；不引入 genai 带来的 grpc / cloud auth 等依赖；录制夹具就是原始 SSE 字节 | `internal/llm/wire`、`internal/llm/{anthropic,openai,gemini}` |
| D-45 | 真实模型验证**以最少的调用完成**：缓存用"同前缀连续两次请求"验证；真实调用测试默认跳过、按需运行；密钥与中转站地址只放 `.local/secrets/`（不入库），文档中中转站只写代号 | `internal/provider/live_test.go`、`docs/notes/api-verification.md` |
| D-46 | 作者模板（Liquid）在**可终止的隔离工作进程**中渲染：yanyi 自身以隐藏子命令 `__tmpl-worker` 运行，超时（1 秒）即杀掉并重启；渲染核心进程内与工作进程共用，语义一致；模板由 Go 服务端渲染 | `internal/tmpl` |
| D-47 | OpenAI Responses 的缓存：GPT-5.6 起（Caps.Cache = breakpoints）用 `prompt_cache_options {mode: implicit, ttl}`（隐式断点自动落在最新消息末尾，相当于滚动断点），并在 L1 / L3 / L4 末尾加显式断点；顶层 `instructions` 不能带断点，所以 L0+L1 放首条 developer 消息。早期模型与兼容接口（如 DeepSeek）用 `instructions` + `prompt_cache_key`（DeepSeek 把 developer 当 user）。缓存保留时长使用中立的 short / long（Anthropic 5m / 1h、OpenAI 30m / 12h） | `internal/llm/openai`、`llm.CacheShort/CacheLong` |
| D-48 | 供应商支持**价格倍率**（中转站的分组倍率）：模型单价照抄站点标价，实际费用 = 单价 × 倍率；运行时解析模型时已乘好，设置页显示单价与倍率 | `providers.price_multiplier`、`llm.Price.Scaled` |

## M0 / M1 待定事项的结论（2026-10-22）

以下决策涵盖模型输出、作品导入、叙事标记与截断续写。

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-49 | 弱模型把工具调用写进正文（progress.md K8）：**暂不在运行时剥离或转换**，正文原样显示与回传。文档中说明：DeepSeek 建议用 Responses 协议接入；带自定义标签的作品宜用工具调用可靠的模型（Claude / GPT / Gemini 实测未出现） | — |
| D-50 | 作品导入：①导入的版本总是成为作品的"最新版本"（不按 semver 比较）；②始终忽略 `__MACOSX`（macOS 压缩生成）；③`DELETE /works/import/{task}` 放弃导入并删除暂存；④并发导入不设上限（本地单用户）；⑤zip 炸弹判定：单个条目解压后超过 1 MiB 且压缩比超过 100 倍；⑥没有 UTF-8 标记的 zip 文件名按 GB18030 解码（中文 Windows 自带压缩工具），`golang.org/x/text` 作为直接依赖 | `internal/work/{importer,archive,ignore,fileset}.go` |
| D-51 | 叙事标记语法细节：①反斜杠转义 `\\ \* \< \> \-` 得到字符本身；②只有**行首**的 `<ooc>` 开始可跨行的场外块，段中的 `<ooc>` 是行内标签、到段末自动闭合；③服务端纯文本（原文索引）保留场景标题、插图说明与场景分隔，去掉所有名称合法的标签（含作品没声明的）；④属性名区分大小写；⑤`scene` `cg` `bgm` `sfx` `pause` 是空元素；找不到开始标签的结束标签忽略；嵌套超过 4 层时忽略更深的开始标签 | `internal/markup`、`web/src/markup/parse.ts`、`testdata/markup/` |
| D-52 | 截断续写去重的重叠下限为 8 字：8 字以内的重合视为巧合，不删正文 | `internal/runtime/continuation.go` |
