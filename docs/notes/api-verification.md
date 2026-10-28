# 三家 API 实测结论（M0 核对清单）

> 2026-10-22 实测。下表列出协议与运行时的 18 项核对结果。
> 适配器为标准库手写（D-44），测试代码：`internal/llm/*/*_test.go`（录制夹具回放）、`internal/provider/live_test.go`（真实调用，默认跳过）。
> 中转站只写代号，地址与密钥不入库（`.local/secrets/`）。

## DeepSeek 正文伪调用（2026-10-27）

Responses 与 Anthropic 兼容接口在带叙事标签的作品上均可能输出伪工具 XML。只删除选项提示、保留 API 工具定义的对照实验未显示改善：基线 6 次中 4 次伪调用，删除提示后 6 次中 5 次伪调用，两组均没有原生选项调用，不能据此判断训练数据来源。

运行时提供流式隔离、有界原生调用纠正与后续发送历史清理。四组接口 / 推理设置共 12 回合通过；Anthropic / low / 中等篇幅另跑 6 回合，逐回合纠正次数为 `[1, 0, 0, 0, 0, 0]`，均成功交还选项。首次输出仍可能错误，更多型号与长对话尚未覆盖。范围、用量与复现方式见 [正文伪工具调用](tool-text-recovery.md)。

正常完整提示词的 strict 对照：官方 `/beta/responses` 已通过正反 schema 校验确认 strict 生效；同一兼容 schema 开启与关闭 strict 各测试六次，均为五次伪调用、一次无选项结束、零次原生选项调用。`/beta/chat/completions` 的两次 strict 复核也均出现伪调用。实验完整回传推理，未启用运行时纠正；没有观察到 strict 解决正文调用的收益。schema 限制和证据见 [strict 实验](tool-text-recovery.md#strict-对照实验2026-10-27)。

## M4 验收（2026-10-25）

按 D-65 使用 T1《第一个故事》与 T3《雾港来信》的短流程：每个场景两回合，第二回合切换协议；
每回合最多四次模型调用、单次输出上限 2048 token，目标正文 100–180 字，不自动重试。

| 协议切换 | T1 | T3 | 调用数 |
| --- | --- | --- | ---: |
| Claude Sonnet 5.5（Anthropic）→ DeepSeek Flash（Responses） | 2 回合通过 | 2 回合通过 | 6 + 5 |
| DeepSeek Flash（Responses）→ Gemini 3.8 Flash | 2 回合通过 | 2 回合通过 | 5 + 5 |
| Gemini 3.8 Flash → Claude Sonnet 5.5（Anthropic） | 2 回合通过 | 2 回合通过 | 3 + 6 |

T3 三组都完成资料读取、状态更新与面板渲染，工具执行没有错误。Gemini 的 T1 首回合自然结束未给选项，第二回合以“继续”输入接续。
其中一次 Claude 流在正文已输出后断开，运行器保存正文并自动续写，最终交还选项。

原始响应逐次与 SQLite `usage_log` 核对，五项互斥用量全部一致；费用以当前预设单价独立计算，与入库微单位一致：

| 协议 / 模型 | 调用数 | 未缓存输入 | 缓存读取 | 缓存写入 | 正文输出 | 推理 | 已报告用量按预设计费 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Anthropic / Claude | 12 | 24 | 64683 | 49581 | 1773 | 0 | USD 0.154670 |
| Responses / DeepSeek | 10 | 11756 | 33536 | 0 | 2152 | 310 | CNY 0.044550 |
| Gemini | 8 | 42019 | 0 | 0 | 1551 | 0 | 未配置价格 |

**核对边界**：29 次有完整用量，1 次断流只有起始用量。后者已报告的输入与初始输出计入记录，最终输出 token 未收到，不能视为完整账单。
中转站实际倍率、Gemini 报价与供应商控制台账单未核对；上表是预设估算，不能代表实际扣款总额。

GPT-6.1-sol 接入在一次切换时返回 403 `Insufficient account balance`，独立 Python 最小请求得到相同结果；游戏流程的 Responses 验收使用 DeepSeek 官方兼容接口。
GPT 接入的补充检查中，`GET /models` 返回 200，包含 `gpt-6.1-sol` 与 `gpt-6-luna`；四次独立 Python 流式 Responses 最小请求均以 `response.failed / upstream_error` 结束：
`gpt-6.1-sol + none` 两次（输出上限 32），`gpt-6.1-sol + low` 与 `gpt-6-luna + low` 各一次（输出上限 128）。
HTTP 均为 200，但只收到空白文本增量，没有最终 usage；未进入 GPT 游戏流程，不据此认定模型可用，也不能确认实际扣款。
这些探针不计入上表的 30 次游戏调用，本地原始响应与摘要在 `.local/eval/m4-gpt/`。
已成功回合的响应通过本地 HTTP 回放恢复后继续评测，避免重复付费；最终全部 30 份响应再次离线回放，六个场景与用量核对均通过。
本地报告：`.local/eval/m4-final/`、`.local/eval/m4-resume/`、`.local/eval/m4-all-replay/`；核对汇总：`.local/eval/m4-summary.json`。

离线回归包含 14 份历史真实 SSE（新增九份无需调用模型），来源见 [回放说明](../../internal/provider/testdata/README.md)。
手工边界夹具覆盖截断工具参数、隐藏推理、暂停、拒答、错误、重试、签名与缺失 ID；三协议 HTTP 竖切验证续写显示去重。
Anthropic 长响应缓存锚点依据 [缓存回溯窗口与四断点限制](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)，
Gemini 跨模型工具历史依据 [thought signatures](https://ai.google.dev/gemini-api/docs/generate-content/thought-signatures)。

## 测试对象

| 代号 | 协议 | 接入 | 模型 |
| --- | --- | --- | --- |
| claude | Anthropic Messages | 中转站 A | claude-sonnet-5-5 |
| gpt | OpenAI Responses | 中转站 A | gpt-6.1-sol |
| gemini-a | Gemini generateContent | 中转站 A | gemini-3.8-flash |
| gemini-b | Gemini generateContent | 中转站 B | gemini-3.8-flash |
| ds-anthropic | Anthropic Messages（兼容） | DeepSeek 官方 | deepseek-flash |
| ds-responses | OpenAI Responses（兼容） | DeepSeek 官方 | deepseek-flash |

每个对象跑同一组子测试（`go test ./internal/provider -run TestLive`）：四个推理档位、并行工具往返（含 Native 回传）、
用户消息图片、工具结果图片、截断与续写、缓存（同前缀连续两次）。

## 结果总览

| 子测试 | claude | gpt | gemini-a | gemini-b | ds-anthropic | ds-responses |
| --- | --- | --- | --- | --- | --- | --- |
| 推理四档均被接受 | ✅ | ✅ | — | ✅ | ✅ | ✅ |
| 并行工具调用 → 回填 → 最终回答 | ✅ 2 个并行 | ✅ 2 个并行 | — | ✅ 2 个并行 | ✅ 2 个并行 | ✅ 2 个并行 |
| 用户消息图片 | ✅ | ✅ | — | ✅ | ✅ | ✅ |
| 工具结果图片 | ✅ | ✅ | — | ✅ | ✅ | ✅ |
| 截断（max_tokens）→ user 提示续写 | ✅ | ✅ | — | ✅ | ✅ | ✅ |
| 缓存第二次命中 | ✅ 6254 | ✅ 3820 | ✅ 8154（约 1.1 万 token 前缀） | ❌ 未观察到 | ✅ 3072 | ✅ 3072 |

（gemini-a 只做了缓存验证。）

## 核对清单

### OpenAI Responses

1. **无状态多轮**：`store:false` + `include:["reasoning.encrypted_content"]` 可用；返回的输出项（reasoning / message / function_call）
   整体存入 Native，同模型原样放回 `input`，往返通过。gpt-6.1-sol 在 low 档对简单问题常不产生 reasoning 项；
   message 项带 `phase`（工具调用前的说明为 `commentary`），原样回传即可。DeepSeek 返回明文 reasoning（`content[].text`）
   与占位的 `encrypted_content`，原样回传被接受。
2. **工具结果含图**：`function_call_output.output` 用内容列表（`input_text` / `input_image` 的 data URL）即可，gpt-6.1-sol 与 DeepSeek 都能识别。
3. **缓存**：GPT-5.6 起有显式断点（`prompt_cache_breakpoint`，最多 4 个写入）与 `prompt_cache_options {mode, ttl: 30m|12h}`；
   写入 1.25×（12h 为 2.5×）、读取 0.1×（GPT-6.1 Sol 为 0.05×）；最小 1024 token。采用 `mode: implicit`（隐式断点自动落在
   最新一条消息末尾，相当于滚动断点）+ 显式断点放在 L1 / L3 / L4 末尾。实测第一次写入 3820、第二次读取 3820。
   早期模型只有隐式缓存与 `prompt_cache_key`。
4. **用量**：`input_tokens` 含 `input_tokens_details.cached_tokens` 与 `cache_write_tokens`；`output_tokens` 含
   `output_tokens_details.reasoning_tokens`。归一：未缓存输入 = input − cached − write。
5. **流式事件**：`response.output_item.added/done`、`response.output_text.delta`、`response.function_call_arguments.delta`、
   `response.reasoning_summary_text.delta`（DeepSeek 为 `response.reasoning_text.delta`）、`response.completed / incomplete / failed`；
   截断为 `incomplete` + `incomplete_details.reason = "max_output_tokens"`。没有 `[DONE]`。
6. **instructions 与 developer**：顶层 `instructions` 不能带断点，所以断点模式下 L0+L1 放首条 developer 消息；
   DeepSeek 把 developer 当 user，DeepSeek 用 instructions（Caps.Cache = implicit）。

### Anthropic Messages

7. **断点**：最多 4 个；TTL 5m / 1h，写入 1.25× / 2×，读取 0.1×；最小可缓存长度 Claude Sonnet 5.5 为 512 token
   （实测 475 token 的前缀不写缓存，514 token 写入 512）；回溯窗口 20 个位置。采用 L1、L3、L4 末尾 + 最后一条消息末尾（滚动）。
   实测第一次写入 6254、第二次读取 6254。
8. **自动缓存**：有（顶层 `cache_control`）。未采用：要放多个稳定边界，且中转站与兼容接口不一定支持顶层字段。
9. **tool_result 含图**：可以，实测通过。
10. **thinking**：Claude Sonnet 5.5 上 `thinking: {type: disabled}` 返回 400，关闭思考写 `{type: between_tools}`（只在 effort ≤ high 可用）；
    `adaptive` + `output_config.effort` 控制深度；`budget_tokens` 在 5 系列上 400；思考默认 `display: omitted`（thinking 文本为空，
    只有签名）。thinking 块必须原样回传；历史只追加（"preserved thinking"：改动之前的历史会使之后的 thinking 块失效）。
    不能用 assistant 预填（续写用 user 提示）。思考 token 计入 `max_tokens`，用量在 `usage.output_tokens_details.thinking_tokens`。
11. **stop_reason**：end_turn、tool_use、max_tokens、stop_sequence、pause_turn、refusal（带 `stop_details.category`）、
    model_context_window_exceeded。
12. **相邻同角色消息**：适配器自己合并（L3/L4 首条 user 与开场 user 相邻）。
- DeepSeek 兼容接口：thinking 默认开启（签名为消息 ID），可 `disabled`；effort 只有 low / high / max；`cache_control` 被忽略，
  硬盘缓存自动生效（第二次读取 3072）。

### Gemini

13. **thoughtSignature**：出现在一步中第一个 functionCall 部件上，流结束时常有一个"空文本 + 签名"的部件；
    模型输出的全部部件合并后原样存入 Native 回传（带签名的空文本单独成为一个部件，不并入前面的文本），往返通过。
    跨模型历史的函数调用用官方占位签名 `skip_thought_signature_validator`。
14. **函数结果含图**：`functionResponse.parts[].inlineData` 可用（gemini-3.8-flash 实测）。
15. **隐式缓存**：中转站 A 上约 3.6k token 的前缀两次都不命中，约 1.1 万 token 时第二次命中 8154（缓存按块计）；
    中转站 B 未观察到命中（同一请求的响应时有时无 Vertex 的 `trafficType` 字段，推测上游轮换多个账号，缓存不共享）。
    `cachedContentTokenCount` 包含在 `promptTokenCount` 内；`thoughtsTokenCount` 与 `candidatesTokenCount` 分开计。
    注意：一张 64×64 的图片计约 1100 token（默认媒体分辨率），M2-10 做图片时考虑 `media_resolution`。
16. **finishReason**：STOP、MAX_TOKENS、SAFETY 等，另有 MALFORMED_FUNCTION_CALL、MISSING_THOUGHT_SIGNATURE、
    CONTINUATION（Gemini 4 单次输出上限 65,536，可用 `continuationToken` 接续；目前按截断续写处理）。
    函数调用现在会返回 `id`；缺少时中立层生成 `gsyn_` 前缀的 ID，回传时不写入。3.8 / 4 系列思考档位只有 low / medium / high。

**安全设置**（M4-1，2026-10-22 验证）：每次请求的 `safetySettings` 中显式发送
`HARM_CATEGORY_HARASSMENT`、`HARM_CATEGORY_HATE_SPEECH`、`HARM_CATEGORY_SEXUALLY_EXPLICIT`、
`HARM_CATEGORY_DANGEROUS_CONTENT`、`HARM_CATEGORY_CIVIC_INTEGRITY`、`HARM_CATEGORY_JAILBREAK`，阈值全部为 `OFF`。
类别与阈值依据 [generateContent API](https://ai.google.dev/api/generate-content#safetysetting)；其中 CIVIC_INTEGRITY 已标记弃用，但仍列在支持的类别中。
这些设置仅控制 API 可配置的过滤器；[模型内建且不可调整的保护](https://ai.google.dev/gemini-api/docs/safety-settings)没有可降低的阈值。

验证使用 `.local/scripts/test_gemini_safety.py`（不入库），读取 `.local/secrets/live.json`，复用
`TestLive/<Gemini 接入>/roundtrip` 的天气工具场景。中转站 A / B 的 gemini-3.8-flash 均完成两个并行工具调用与结果回填后的回答；
五份原始请求全部携带上述设置（四次成功，一次上游 429 经现有重试成功），未遇到需要改用更高拦截阈值的参数限制。
日志与原始报文保存在 `.local/probe/gemini-safety/`（不入库）。

### 库

17. `modernc.org/sqlite`：FTS5 + trigram 可用，6 平台交叉编译通过（M0-3）。
18. `osteele/liquid`：渲染不可中断 → 使用可终止的隔离工作进程（D-46，M0-4）。

## 运行器上的真实游玩（M1 退出标准）

用 `.local/scripts/play.py`（不入库）经 HTTP API 驱动服务：开局 → 每回合有选项时轮流选择、每 4 回合改为自由输入。

| 对象 | 作品 | 回合 | 结果 |
| --- | --- | --- | --- |
| ds-responses | T1《第一个故事》 | 开场 + 20 | ✅ 21 回合全部以 offer_choices 交还；缓存读取 389k / 未缓存 10k；约 ¥0.24 |
| ds-anthropic | T1 | 开场 + 20 | ⚠️ 全部提交成功，但 11 个回合在正文中写出伪工具调用（`<offer_choices"> <choices><item…>`），只有 10 个回合真正调用了工具 |
| claude | T1 | 开场 + 4 | ✅ 全部以选项交还；每回合未缓存输入约 2 token（其余为缓存读取 / 写入） |
| gemini-a | T1 | 开场 + 4 | ✅ 全部以选项交还；前缀 4–8k 时未观察到隐式缓存命中 |
| gpt | T1 | — | 中转站 A 的 GPT 上游故障（`Upstream request failed`，含最小请求），未能完成；冒烟测试矩阵此前已全部通过 |

实测结论与当前行为：

- **选项呈现**：L0 说明"选项会以按钮显示给玩家，正文里不需要再列出"。提示词版本 2026.10.22-1 的验证未观察到正文列举选项；
  未包含该说明的 ds-anthropic 20 回合测试中，模型从开场起就在正文末尾列举选项、不调用 offer_choices，并在后续回合模仿。
- **伪工具调用**：ds-anthropic 在第 12 回合起把工具调用写成正文中的 XML，一旦出现就被后续回合模仿；提示词版本 2026.10.22-2
  包含"工具直接调用即可，正文里不需要写出工具名或参数"，该现象仍出现。同一模型经 Responses 接口 21 回合零出现，判断为 DeepSeek
  在多轮工具历史上存在误写风险；两种接口的运行时纠正与验证见 [正文伪工具调用](tool-text-recovery.md)。
- **带叙事标记的作品**：《雾港来信》（作品定义了 `<say>` `<gold>` 等标签与状态）上，deepseek-flash（Responses）连续 8 个回合都把
  `<update_state>{…}</update_state>`、`<offer_choices>{…}</offer_choices>` 写进正文、从不真正调用工具（只调用过 read）；
  含"标签只用于正文呈现，状态与选项通过工具完成"的实验提示词也有 5/5 回合出现该现象；正式提示词不含这句说明。同一作品上 claude 正常
  （标签与工具调用都正确）。deepseek-flash 在"作品自定义 XML 标签 + 工具"的组合下需要运行时纠正（D-49，progress.md K8）。
- **失败提示**：运行器保留原始错误并按类型分类，上游 5xx 重试耗尽时显示对应的模型服务错误。
- 首回合常调用 list / read / write：read / list 已实现（M3-1）；write 等记忆写入随 M3-2，目前返回"暂时不可用"，每次多花一次调用。

## T3 评测（M2 退出标准，2026-10-23）

用评测脚本 `testdata/eval/misty-harbor.yaml`（15 步行动 + 4 个检查问题）经运行器直接驱动（`TestEval`，见 development.md 第 4 节），
对象 claude（中转站 A，claude-sonnet-5-5，推理 low）。

| 轮次 | 结果 |
| --- | --- |
| 第 1 轮（update_state 参数为操作数组） | 14 步提交，第 15 步因故事已收束没有选项而中止（脚本问题）。update_state 24 次调用中 4 次、offer_choices 15 次中 3 次参数无法解析 |
| 第 2 轮（单项 update_state，D-53） | ✅ 15 步全部提交，14 步以选项交还（`/揭晓` 一步以结局收束）；update_state 24 次全部成功；4 个检查问题全部答对，与主线设定一致；offer_choices 3 次参数无法解析（该轮编译于引号修复之前）、write 2 次（M3-2 未实现）。56 次调用，缓存读取 199 万 / 缓存写入 9 万 / 未缓存 110 / 输出 4.1 万 token，按官方价约 $1.34 |

- **对象数组参数被写成字符串**：文字含 ASCII 双引号时，claude 把对象数组参数整体写成 JSON 字符串且内部引号未转义（update_state 的 ops、offer_choices 的 choices）。
  单独探查（`.local/scripts/probe_update_state.py`）：操作数组写法 4 / 4 次如此，给 value 加类型、放宽额外字段均无效；单项操作 2 / 2 次正确并能并行调用。
  处理：update_state 改为单项操作，工具参数解码时修复未转义的引号后再还原（D-53，修复用第 1 轮的真实样本做了单元测试）。
- **流中断**：中转站两次在流中途断开（HTTP/2 INTERNAL_ERROR、无结束事件），运行器按截断续写处理，回合正常完成。
- **模型行为**：读取了主线与人物设定（7 次 read），线索维护积极（14 条），封印陈述 2 条；没有掷骰，没有更新信任，没有看图——这些是作品写法与提示词层面的事，留给作品定稿（M6-7）与提示词迭代。
- **成本**：缓存读取占输入的 96%；每回合的输入随历史线性增长（19 回合末约 4.5 万 token），上下文整理（M3-6）之后会回落。

## 上下文整理（M3 退出标准，2026-10-23）

用 `.local/scripts/play_m3.py`（不入库）经 HTTP API 驱动独立数据目录上的服务：T1《第一个故事》，ds-responses（deepseek-flash，推理 low），
篇幅"短"，存档设置把上下文预算调低到 10,000 token、保留原文 3,000 token，开场 + 26 回合；每回合有选项时轮流选择、每 4 回合自由输入一次。

| 整理 | 时机 | 摘要掉 / 保留 | 上下文（token） | 摘要 | 落盘 | 调用 / 耗时 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 第 8 回合提交后 | 第 1–4 / 第 5 回合起 | 10,310 → 5,130 | 977 字 | 3 个文件 | 4 次 / 11.5 s |
| 2 | 第 19 回合提交后 | 第 5–13 / 第 14 回合起 | 10,279 → 5,443 | 1,379 字 | 4 个文件 | 5 次 / 21.0 s |
| 3 | 第 27 回合提交后 | 第 14–24 / 第 25 回合起 | 10,223 → 4,391 | 1,428 字 | 6 个文件 | 5 次 / 24.1 s |

- **结论**：27 个回合全部提交、整理 3 次，均为 inline（原上下文 + 整理指令，命中缓存）；达到退出标准"整理 ≥ 2 次且续写连贯"
  （本次使用调低的阈值；100 回合评测见 M3-11）。
- **摘要质量**：三份摘要都按六节结构写出，注明回合号与对应的记忆文件（`chronicle/ch01.md`、`ch02.md`），较早的情节压缩为章节级概述，
  "当前场景"与最新正文一致，未确定的事标注"未确认"。
- **续写连贯**：整理后的第一个回合直接接着上一回合的场景写（人物、物件、约定、语气一致），没有重复已写的内容，也没有遗忘前文的迹象。
- **记忆**：模型从开场起就主动 write / edit 记忆（M3-2 之前这些调用都白费），整理时补写章节纪要与线索；第 25 回合用 search 查过原文。
- **成本**：叙事 39 次调用 ¥0.17、整理 14 次调用 ¥0.08，合计约 ¥0.25；整理调用的输入几乎都是缓存读取（16.2 万 / 16.6 万）。

## M0-4：Liquid 隔离工作进程（D-46）

- 模板存储拒绝 `{% include %}` / `{% render %}` 的文件读取，防止作者模板访问宿主文件；`osteele/liquid` 的默认存储实测可读取工作目录下的源码。
- 渲染无法中断：放进工作进程，超时（1 秒）即杀掉并重启。限制：源码 64 KiB、变量 256 KiB、输出 64 KiB（写满立即中止）。
- 实测（Windows，典型面板模板）：进程内单次渲染约 16 µs，经工作进程约 41 µs；工作进程启动约 7 ms（按需启动，最多 2 个）。
- 渲染库在少数模板上会 panic（如对 `forloop` 赋值后用 `cycle`），渲染核心捕获 panic 并返回模板错误。

## 中转站与兼容接口的注意事项

- 中转站会在 SSE 的 JSON 后补空格、上游限流时返回 429（`Upstream rate limit exceeded`）、偶尔连接被 EOF：重试策略能覆盖。
- 中转站 B 的 `/v1beta/models` 返回通用模型列表；实测该密钥可用模型与 OpenAI 风格的 `/v1/models` 列表更一致。
- 价格：中转站 A 以人民币计价；中转站 B 以美元标价并有分组倍率（供应商的价格倍率，D-48）。DeepSeek 按高峰价计。
