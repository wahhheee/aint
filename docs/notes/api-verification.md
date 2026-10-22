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

## 中转站与兼容接口的注意事项

- 中转站会在 SSE 的 JSON 后补空格、上游限流时返回 429（`Upstream rate limit exceeded`）、偶尔连接被 EOF：重试策略能覆盖。
- 中转站 B 的 `/v1beta/models` 返回的是通用列表，不代表该 key 可用的模型；其 OpenAI 风格的 `/v1/models` 更准确。
- 价格：中转站 A 以人民币计价；中转站 B 以美元标价并有分组倍率（供应商的价格倍率，D-48）。DeepSeek 按高峰价计。
