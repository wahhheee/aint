# 三家 API 实测结论（M0 核对清单）

> 2026-10-22 实测。对应技术方案第 24 节「实现前核对清单」1–18 项。
> 适配器为标准库手写（D-44），测试代码：`internal/llm/*/*_test.go`（录制夹具回放）、`internal/provider/live_test.go`（真实调用，默认跳过）。
> 中转站只写代号，地址与密钥不入库（`.local/secrets/`）。

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

发现与处理：

- **选项写进正文**：第一次 20 回合（ds-anthropic）中模型从开场起就把选项写成正文末尾的列表而不调用 offer_choices，并在后续回合模仿。
  L0 补充"选项会以按钮显示给玩家，正文里不需要再列出"后不再出现（提示词版本 2026.10.22-1）。
- **伪工具调用**：ds-anthropic 在第 12 回合起把工具调用写成正文中的 XML，一旦出现就被后续回合模仿；补充"工具直接调用即可，
  正文里不需要写出工具名或参数"（2026.10.22-2）后仍出现。同一模型经 Responses 接口 21 回合零出现，判断为 DeepSeek
  Anthropic 兼容层在多轮工具历史上的问题。**DeepSeek 建议用 Responses 协议接入**；Anthropic 兼容接口可用于不依赖工具的场景。
  若其他兼容接口也出现，再考虑在提交时剥离正文中的工具名标签（记入 progress.md 已知问题）。
- **带叙事标记的作品**：《雾港来信》（作品定义了 `<say>` `<gold>` 等标签与状态）上，deepseek-flash（Responses）连续 8 个回合都把
  `<update_state>{…}</update_state>`、`<offer_choices>{…}</offer_choices>` 写进正文、从不真正调用工具（只调用过 read）；
  在"叙事标记"说明中补充"标签只用于正文呈现，状态与选项通过工具完成"后仍是 5/5，已撤回该改动。同一作品上 claude 正常
  （标签与工具调用都正确）。结论：deepseek-flash 在"作品自定义 XML 标签 + 工具"的组合下不可靠。处理方式待定（见 progress.md 已知问题 K8）。
- **失败提示**：上游 5xx 重试耗尽时玩家看到的是"回合运行出错"，原因是运行器只保存了错误文字；已改为保留原始错误再分类。
- 首回合常调用 list / read / write：read / list 已实现（M3-1）；write 等记忆写入随 M3-2，目前返回"暂时不可用"，每次多花一次调用。

## M0-4：Liquid 隔离工作进程（D-46）

- `osteele/liquid` 的默认模板存储会让 `{% include %}` / `{% render %}` 读取工作目录下的任意文件（实测可读到源码），已替换为拒绝一切读取的存储。
- 渲染无法中断：放进工作进程，超时（1 秒）即杀掉并重启。限制：源码 64 KiB、变量 256 KiB、输出 64 KiB（写满立即中止）。
- 实测（Windows，典型面板模板）：进程内单次渲染约 16 µs，经工作进程约 41 µs；工作进程启动约 7 ms（按需启动，最多 2 个）。
- 渲染库在少数模板上会 panic（如对 `forloop` 赋值后用 `cycle`），已在渲染核心中恢复为模板错误。

## 中转站与兼容接口的注意事项

- 中转站会在 SSE 的 JSON 后补空格、上游限流时返回 429（`Upstream rate limit exceeded`）、偶尔连接被 EOF：重试策略能覆盖。
- 中转站 B 的 `/v1beta/models` 返回的是通用列表，不代表该 key 可用的模型；其 OpenAI 风格的 `/v1/models` 更准确。
- 价格：中转站 A 以人民币计价；中转站 B 以美元标价并有分组倍率（供应商的价格倍率，D-48）。DeepSeek 按高峰价计。
