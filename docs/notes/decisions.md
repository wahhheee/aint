# 决策记录：脚手架评审（2026-10-20）

本文记录当前决策及其代码位置。
编号范围 D-23～D-71；本地设计材料在 `.local/docs/`。

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-23 | `presentation.text_effects` 默认关闭；`features.ooc` 默认开启 | `internal/work/manifest.go` |
| D-24 | 平台默认的记忆目录约定写在 L0"记忆"节；作品 `memory.md` 在 L1 补充，作品写明替换时以作品为准 | `internal/prompts/l0.go` |
| D-25 | 命令未写 `present` 时为 `[slash]`；开场按 D-71 显式声明列表与模式 | `internal/work/compile.go`、`internal/work/openings.go` |
| D-26 | 叙事标记每个非空行为一个段落 | `web/src/markup/parse.ts` |
| D-27 | 选项 `style` 只取作品样式标签，不内置 `danger`/`subtle`；作品有样式标签时才有该参数 | `internal/tools/builtins.go` |
| D-28 | `<cg>` 只在有非背景图片素材时启用；背景图只用于 `<scene bg>` | `internal/contextx/profile.go` |
| D-29 | 选项待答时玩家独立输入命令 / 场外 / 继续：工具结果如实写"没有从选项中作答"，输入照常跟在后面；选项附带命令时仍算选项作答，工具结果写所选项，随后附上命令输入 | `internal/contextx/envelope.go` |
| D-30 | 内置标签 `<ooc>`（随 `features.ooc`）：AI 的场外回应写在其中，浅色块显示，不进入原文检索 | `prompts/l0.go`、`markup/`、`web/src/markup/` |
| D-31 | **开发目录**用于作品链接本地目录；**开发者选项**是设置开关，默认关，开启后显示完整状态与状态编辑，开发目录作品默认显示。游玩调试使用阅读页独立开关（D-63），状态编辑仍属于开发者工具 | `internal/settings`、阅读页 |
| D-32 | `engine` 比较独立的**作品能力版本**（程序内置，目前 `2.0`），与程序发布版本解耦；多开场格式属于能力版本 `2.0` | `internal/work/engine.go` |
| D-33 | 静态开场节点存两条消息：合成的 user `<input kind="opening">（开场）</input>` + 助手开场正文与选项工具调用（有选项时）；历史始终以 user 开头，首轮作答回填为工具结果 | `internal/save/opening.go` |
| D-34 | 模型价格带货币；费用按货币分别汇总，不做汇率换算 | `internal/llm/usage.go`、`usage_log.currency`、`internal/usage`（汇总） |
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
| D-46 | 作者模板（Liquid）在**可终止的隔离工作进程**中渲染：aint 自身以隐藏子命令 `__tmpl-worker` 运行，超时（1 秒）即杀掉并重启；渲染核心进程内与工作进程共用，语义一致；模板由 Go 服务端渲染 | `internal/tmpl` |
| D-47 | OpenAI Responses 的缓存：GPT-5.6 起（Caps.Cache = breakpoints）用 `prompt_cache_options {mode: implicit, ttl}`（隐式断点自动落在最新消息末尾，相当于滚动断点），并在 L1 / L3 / L4 末尾加显式断点；顶层 `instructions` 不能带断点，所以 L0+L1 放首条 developer 消息。早期模型与兼容接口（如 DeepSeek）用 `instructions` + `prompt_cache_key`（DeepSeek 把 developer 当 user）。缓存保留时长使用中立的 short / long（Anthropic 5m / 1h、OpenAI 30m / 12h） | `internal/llm/openai`、`llm.CacheShort/CacheLong` |
| D-48 | 供应商支持**价格倍率**（中转站的分组倍率）：模型单价照抄站点标价，实际费用 = 单价 × 倍率；运行时解析模型时已乘好，设置页显示单价与倍率 | `providers.price_multiplier`、`llm.Price.Scaled` |

## M0 / M1 待定事项的结论（2026-10-22）

以下决策涵盖模型输出、作品导入、叙事标记与截断续写。

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-49 | 正文中以当前可用工具名为 XML 标签的伪调用不进入玩家显示；代码引用、转义与作品同名样式标签保留。不解析或执行 XML，混合原生调用整批退回并补齐错误结果，由模型重新发起原生调用，每回合最多纠正两次，沿用预算与取消；仍未完成时中断并保留正文。后续发送历史清理伪标签、退回的调用与结果、纠正提示，并移除修改位置起的 Native 和推理；成功操作与配对结果保留。存储及检查器保留原始消息；规则和真实验证见 [正文伪工具调用](tool-text-recovery.md) | `internal/runtime/tool_text.go`、`tool_history.go`、`turn.go` |
| D-50 | 作品导入：①导入的版本总是成为作品的"最新版本"（不按 semver 比较）；②始终忽略 `__MACOSX`（macOS 压缩生成）；③`DELETE /works/import/{task}` 放弃导入并删除暂存；④并发导入不设上限（本地单用户）；⑤zip 炸弹判定：单个条目解压后超过 1 MiB 且压缩比超过 100 倍；⑥没有 UTF-8 标记的 zip 文件名按 GB18030 解码（中文 Windows 自带压缩工具），`golang.org/x/text` 作为直接依赖 | `internal/work/{importer,archive,ignore,fileset}.go` |
| D-51 | 叙事标记语法细节：①反斜杠转义 `\\ \* \< \> \-` 得到字符本身；②只有**行首**的 `<ooc>` 开始可跨行的场外块，段中的 `<ooc>` 是行内标签、到段末自动闭合；③服务端纯文本（原文索引）保留场景标题、插图说明与场景分隔，去掉所有名称合法的标签（含作品没声明的）；④属性名区分大小写；⑤`scene` `cg` `bgm` `sfx` `pause` 是空元素；找不到开始标签的结束标签忽略；嵌套超过 4 层时忽略更深的开始标签 | `internal/markup`、`web/src/markup/parse.ts`、`testdata/markup/` |
| D-52 | 截断续写去重的重叠下限为 8 字：8 字以内的重合视为巧合，不删正文 | `internal/runtime/continuation.go` |

## M2 实现（2026-10-23）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-53 | `update_state` 每次调用改**一项**（参数 `{op, path, value, reason}`），多项变化在同一次回复中多次调用。原因：实测 Claude Sonnet 5.5（中转站 A）把"操作对象数组"参数写成 JSON 字符串，且字符串内部引号未转义，无法解析（4 / 4 次；给 value 加类型、放宽额外字段均无效），单个操作的参数 2 / 2 次正确，并能在一次回复中并行调用。评测中发现这只在文字含 ASCII 双引号时发生，`offer_choices` 也会遇到：工具参数解码失败时，把写成 JSON 字符串的数组 / 对象参数还原（必要时按"引号后面不是 , : } ] 就是文字里的引号"修复未转义的引号）后再解码一次 | `internal/tools/builtins.go`、`internal/tools/args.go` |
| D-54 | 叙事标记**不设 `inline: false` 警告**：作品格式中没有 `inline` 字段，标签一律写在一行之内，未闭合时到段末自动结束（D-51），没有需要警告的情形；段首的 `<scene>` / `<cg>` 提升为块级由解析器决定，作者标签不能声明为块级 | `internal/markup`、`web/src/markup/parse.ts` |
| D-55 | 作者模板的 `for` 循环**不单独计数**：失控的循环由 1 秒渲染时限（超时即杀掉隔离工作进程，D-46）与 64 KiB 输出上限（写满即中止）终止。原因：所用 Liquid 库没有循环计数的扩展点，计数需要重写 `for` 标签；隔离工作进程已保证失控模板不会拖垮服务 | `internal/tmpl/render.go`（`newLiquid`） |
| D-56 | 作品主题的排版数值与玩家阅读设置合并：字体与缩进选"跟随作品"时用主题的值；行距、段距、版心宽度等于平台默认值时视为玩家没有改过，用主题的值，调整过则用玩家的值；玩家可以在设置中关闭作品主题。阅读设置不改为可空 | `web/src/features/reader/useReadingStyle.ts` |

## M3 实现（2026-10-23）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-57 | 上下文整理的执行与触发：①整理在 runtime 中执行，与回合共用会话、工具与 VFS；`compact` 包只放参数与规则（阈值、硬上限、保留起点、摘要校验）；②回合提交后，上下文规模（最后一次调用的输入 + 输出）达到阈值时，写者槽位直接交给后台整理；整理期间提交的输入排队（至多一个），整理结束后开始；移动 head、修改正文、编辑记忆或状态等操作返回 409；③回合内达到硬上限（窗口 − 预留）、模型报上下文超限时在回合内整理，每回合至多一次：只整理本回合之前的历史，整理节点成为本回合的父节点，本回合的消息全部保留；超限时原上下文已经放不下，改用 separate 的输入方式（没有轻量模型时用主笔模型）；④自动触发但没有可以摘要掉的回合时不提示玩家；⑤自动触发要求上次整理之后至少 3 个回合、且本回合不是出错结束（预算小于常驻部分加保留原文时不会每个回合都整理） | `internal/runtime/compact.go`、`compact_job.go`、`internal/compact` |
| D-58 | 阶段与历史：整理后的请求以最近一次整理的阶段块开头，后接保留的回合与之后的回合；保留的回合去掉推理摘要与协议原生内容（Claude 等模型的推理签名依赖完整的历史，历史改变后原样回传会被拒绝），文字与工具调用保留；对应调用已不在上下文中的工具结果改写为文字；整理过程的消息存在整理节点上，不进入之后的上下文；交接摘要目标为 min(3000, 阈值 × 10%) 字，超过 6000 token 时要求压缩一次 | `internal/runtime/history.go`、`internal/contextx/epoch.go` |
| D-59 | token 与整理参数：节点 meta 的 `context_tokens`（回合为最后一次调用的输入 + 输出，整理节点为新阶段的估算）是阈值判断与用量环的依据；请求估算按模型以实际输入做滑动平均校准，只在内存中；整理参数在全局设置 `context`（预算、保留原文、整理方式），存档设置可以覆盖，零值沿用全局 | `internal/runtime/tokens.go`、`internal/settings`、`save.Settings` |
| D-60 | 原文与检索：原文索引的一行就是 `/transcript/NNNN` 首行之后的内容（玩家输入行 + 正文纯文本，`<say who>` 换成人物名），读取与检索看到同一份文字；检索的字面匹配不区分大小写，每个词至少 3 字时原文先走 trigram 索引，否则扫描本存档的原文，结果都按进度线过滤；结果带行号；同一批工具调用按原顺序执行，相邻的读类并行，写入之后的读取能读到新内容 | `internal/save/transcript.go`、`internal/vfs/search.go`、`internal/tools/registry.go` |
| D-61 | 分支与编辑：修改 AI 正文生成原回合的兄弟节点（各段助手文本合并为一段放在第一段的位置，去掉协议原生内容），不附加注记，前端的修改框说明修改的结果；修改玩家输入即以新输入重新生成；编辑节点与整理节点的回合号是它们所跟随的回合；玩家修改记忆或状态产生编辑节点，注记附在下一回合的 `<notes>`；时间线包含编辑与整理节点 | `internal/save/branch.go`、`edit.go`、`web/src/features/reader/stream/TurnToolbar.tsx` |
| D-62 | 轻量模型在设置 → 模型的"⋯"菜单中设定；separate 整理使用它，没有设定时 separate 改用 inline（超限时的紧急整理除外，见 D-57） | `internal/runtime/tokens.go`（`lightModel`）、`web/src/features/settings/sections/ProvidersSection.tsx` |

## 游玩调试（2026-10-23）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-63 | 阅读页提供按存档独立的调试开关，默认关闭；开启后记录模型调用并提供可收起的侧边检查器。模型明文推理只展示供应商返回的内容；HTTP 鉴权头与 URL 的密钥字段脱敏。调试记录仅保留在进程内，全局至多 100 次调用 / 64 MiB，单次至多 8 MiB；VFS 在调用前与工具执行后冻结快照。状态编辑仍由开发者选项控制 | runtime/inspect.go、llm/observe.go、reader/debug/ |

## M4 三协议与供应商（2026-10-25）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-64 | 模型预设随应用版本维护，按接入协议与模型前缀匹配；模型设置提供“应用预设能力参数”和“填入官方价格”，填入表单后保存生效。已有模型配置不会自动更新，能力更新不覆盖中转站报价。预设不建立在线更新服务 | `provider/presets.go`、`server/api_providers.go`、`settings/providers/ModelDialog.tsx` |
| D-65 | M4 验收采用三协议在 T1/T3 的短流程与跨协议切换，费用核对原始 API usage 与本地统计；供应商控制台仅在已有账单数据可用时核对。M4-8 按场景覆盖，以历史真实录制加手工边界夹具覆盖文本、工具往返、Native、缓存、推理、截断、拒答、错误与重试，不以每协议十份真实录制为门槛。真实评测默认跳过，限制每回合调用数与输出 token，失败时先通过独立脚本判断供应商状态 | `runtime/m4_live_test.go`、`provider/replay_test.go`、`llm/*/*_test.go` |

## M5 体验与调用费用（2026-10-25）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-66 | 后台助手与存档标题默认关闭。玩家主动开启时说明轻量模型费用；供应商连接测试只在点击“开始测试”后调用，允许跳过。助手每回合累计最多 10 次调用；标题同进度自动生成去重，手动命名优先 | runtime/helper.go、runtime/title.go、settings、ProviderDialog |
| D-67 | 正文的工具旁白由提示词约束与检查器启发式提示，不按关键词删改；`<br>` / `<br/>` 是安全换行，其他 HTML 不放开。作品支持 `<act>`、音频与限时选项 | prompts、markup、reader/debug |
| D-68 | 调试图标只切换记录，不展开检查器；检查器入口位于用量环与回合菜单。输入栏分设模型 / 推理与上下文 / 成本入口，空闲时保存、下一回合生效；模型显示供应商，切换前说明文风、能力、缓存及上下文影响并确认，推理使用滑块；篇幅只在输入栏设置并保存至存档；预算留空以淡色数字展示继承的全局或运行时预设 | reader/input、reader/chrome、reader/debug、runtime/settings.go |

## M6 分发与创作者工具（2026-10-25）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-69 | 存档升级先按旧版本整理，再原子提交目标版本与状态；新字段使用默认值，兼容值递归保留，删除或类型不兼容的旧值存入节点迁移记录，不参与当前模型状态。作品详情的存档菜单与状态抽屉可查看记录；升级前的分支完整保留，回到该分支即恢复旧版本绑定。开发目录使用相同状态迁移规则，在下一次提交时保存迁移记录；回合使用开始时的作品快照 | `state/migrate.go`、`runtime/upgrade.go`、`runtime/compact.go`、`save/repo.go` |
| D-70 | 主动删除内置示例或版本后，启动时不再自动导入该作品。删除标记按作品 ID 保存在 settings；手动导入仍可恢复。发布包附两部示例 zip | `work/service.go`、`work/versions.go`、`cmd/release-assets` |

## 多开场（2026-10-26）

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-71 | 作品显式声明非空 `openings` 列表，ID 稳定且唯一，第一项默认；每项独立指定静态正文或生成指引及可选初始状态覆盖。状态依次应用默认值、开场覆盖、开局表单。开场及其中断续写不计玩家首轮；首次玩家模型交互成功保存后永久锁定，含场外对话，回退、删除与复制均不解锁。切换由现有写者槽位串行执行，事务重建初始节点与开场、清除试读分支、保留用量；生成式切换回来重新生成。只有一个开场不显示切换控件。详见 [多开场](../openings.md) | `work/openings.go`、`save/opening.go`、`runtime/opening.go`、`reader/OpeningSwitcher.tsx` |
