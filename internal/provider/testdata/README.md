# M4 协议回放

`replay.json` 是历史真实 SSE 的预期停止原因与归一用量。夹具正文位于 `internal/llm/{anthropic,openai,gemini}/testdata/`，只含 SSE 正文；不保留中转站地址、请求头或密钥。

新增九份夹具来自 2026-10-22 的本地 M0 探针：

| 协议 | 模型 | 文件 | 原始录制时间 | 场景 |
| --- | --- | --- | --- | --- |
| Anthropic | claude-sonnet-5-5 | cached-tool-result.sse | 11:46:52 | 工具结果续写、签名与缓存写入 |
| Anthropic | claude-sonnet-5-5 | truncated.sse | 11:48:36 | max_tokens |
| Anthropic | claude-sonnet-5-5 | cache-hit.sse | 11:49:18 | 缓存读取与追加写入 |
| Responses | gpt-6.1-sol | tool-result.sse | 11:50:21 | 工具结果续写 |
| Responses | gpt-6.1-sol | truncated.sse | 11:51:11 | 截断、推理 token 与加密推理 |
| Responses | gpt-6.1-sol | cache-hit.sse | 11:51:44 | 缓存读取与追加写入 |
| Gemini | gemini-3.8-flash | tool-result.sse | 11:52:34 | 工具结果续写 |
| Gemini | gemini-3.8-flash | truncated.sse | 11:53:03 | MAX_TOKENS |
| Gemini | gemini-3.8-flash | reasoning.sse | 11:52:25 | 推理与正文用量分离 |

已有五份并行工具夹具保留原位置。`TestCrossProtocolHistory` 用三份真实响应覆盖九种协议组合的请求构建；同协议核对原生块，跨协议核对中立工具与结果。

手工 SSE 在各协议测试中明确标注，用来稳定触发拒答、暂停、坏参数、缺少 ID、流中错误、超限与断流等场景。运行器的 `TestProtocolContinuation` 经过三协议 HTTP 流验证自动续写与显示去重，`TestAnthropicPauseNative` 验证无正文的原生响应续发。测试不发起外网请求。
