# Gemini 开场选项 400 排查与验证

核对日期：2026-10-28。接入为中转站 A 的 Gemini 原生协议，模型 `gemini-4-argon`。

## 结论与证据边界

Gemini 适配器在合并相邻用户消息后，将普通文字与独立图片放在 `functionResponse` 之前。各组内部顺序、工具结果的内嵌图片与助手原生签名不变，中立请求和存档不被修改。该规则应用于全部用户历史消息，避免已经完成的回合在后续请求中使用不同的排序。

复现对照定位到工具结果与附加文字在同一条用户消息中的排序问题。错误文字不能直接用来断定 AINT 发送的最后角色为 `model`。中转站内部的转换与上游路由不可见，尚不能确定错误发生在中转转换层还是具体的 Google 接入端；最小工具请求中的相同顺序可以成功，不能推论所有混合消息都会失败。

中转站 A / Argon 的真实运行时验证覆盖静态开场选择、工具往返与下一轮选择；两回合均正常提交并交还选项。多工具结果、两种图片承载方式、相邻用户消息合并、原生签名、中立请求不变和历史前缀稳定性另有离线回归覆盖；尚未实测所有 Gemini 接入及型号。

## 官方契约与相关实现

Google 的 [Gemini 3.6 Flash / 3.5 Flash-Lite API 变更说明](https://ai.google.dev/gemini-api/docs/generate-content/whats-new-gemini-3.6#api-changes-and-parameter-updates) 明确规定：从这两个型号起及后续型号，最后一个非空消息为 `model` 时返回 400，不支持助手预填。因此该限制适用于 Argon，但并非 Argon 首次引入。

[Argon 迁移说明](https://ai.google.dev/gemini-api/docs/generate-content/latest-model) 还涉及采样参数、推理档位、显式缓存与长输出续接；这些变化不能单独解释本次排序对照中的结果。

CLIProxyAPI 的 [issue #5607](https://github.com/router-for-me/CLIProxyAPI/issues/5607) 记录了同一报错：其维护记录中的 Vertex AI 对照显示 `[functionResponse, text]` 失败，`[text, functionResponse]` 或纯工具结果成功，并链接了相应修复。它提供了同类问题的实现证据，不证明中转站 A 使用同一上游。

## 2026-10-28 复现实验记录

所有生成调用均使用 `/v1beta/models/gemini-4-argon:streamGenerateContent?alt=sse`，`thinkingLevel=low`、`maxOutputTokens=2048`；客户端不重试。凭据来自忽略目录中的已有私有配置，不写入请求与响应产物。

先用固定短文本和最小工具定义作对照，再由运行时测试装配临时存档，创建《雾港来信》默认静态开场、选择选项 A，捕获交给模型的首个中立请求，交给现有 Gemini 适配器序列化。保留系统提示、工具、常驻图片、开场历史与玩家选择；真实调用时仅统一输出预算与推理档位。

完整开场请求以 `user` 结尾，末条消息为 `[functionResponse, text]`，其中 `text` 包含 `<pace>`。两次均返回 HTTP 400，错误为 `Requests ending with a model turn are not supported.`；仅交换末条两个部件的对照，两次均返回 HTTP 200，并产生原生工具调用及 `STOP`。

| 请求 | 次数 | 结果 |
| --- | ---: | --- |
| 最小请求，以普通 `user` 文字结尾 | 1 | 200，正文 `OK` |
| 最小请求，以普通 `model` 文字结尾 | 1 | 400，同一错误 |
| 最小工具请求，以 `[functionResponse]` 结尾 | 1 | 200，正文 `OK` |
| 最小工具请求，以 `[functionResponse, text]` 结尾 | 1 | 200，正文 `OK` |
| 最小工具请求，以 `[text, functionResponse]` 结尾 | 1 | 200，正文 `OK` |
| AINT 开场请求，末条为 `[functionResponse, text]` | 2 | 均为 400；约 52.27 秒、31.33 秒 |
| 同一 AINT 请求，仅交换末条两个部件 | 2 | 均为 200；约 6.97 秒、5.59 秒；原生工具调用，`STOP` |

共 9 次生成调用，另有 1 次模型列表查询。6 次成功调用合计已报告输入 12,967 token、输出 190 token；失败调用没有 usage，不推算其费用。已运行应用的当日日志也记录了 `kind=choice`、`turn_no=2`、`model=gemini-4-argon` 的相同 400。

本地复现材料（不入库）：

- `.local/tmp/gemini-turn-probe.py`：读取私有配置的最小探针。
- `.local/tmp/gemini-turn-probe/aint-neutral.json`：运行时捕获的中立请求。
- `.local/tmp/gemini-turn-probe/aint-opening.json`：现有适配器构建的完整协议请求。
- 同目录的 `*.request.json`、`*.response.txt`、`*.summary.json`：九次调用的请求、原始响应和摘要；不含鉴权头。

## 代码位置与测试

1. `internal/save/opening.go` 的 `prepareOpening` 为静态开场创建 `offer_choices` 调用历史。
2. `internal/contextx/envelope.go` 的 `Envelope.Message` 先回填玩家选择为工具结果，再追加篇幅、注记等文字，形成中立层的 `[tool_result, text]`。
3. `internal/llm/gemini/gemini.go` 的 `parts` 转换部件；`Body` 合并相邻同角色消息，再由 `orderToolResults` 按普通部件、工具结果两组排列用户消息。

- `internal/llm/gemini/request_test.go`：多工具结果、文字与图片交错、相邻用户消息合并、结果内嵌图片 / 独立图片、错误结果、助手 Native、请求不变与后续回合前缀。
- `internal/runtime/opening_test.go`：开场选项、自由输入、另有输入、修改正文与命令选项的实际请求，断言 Gemini 用户消息以工具结果结尾。
- `internal/runtime/prefix_test.go`：三协议跨回合与回合内调用的前缀稳定性。
- `internal/runtime/gemini_live_test.go`：默认跳过的有界真实运行时验证，执行方法见 [开发指南](../development.md#41-m4-协议验收)。

## 2026-10-28 修复验收记录

新增 Gemini 用户部件排序并补充回归测试。真实验证使用中转站 A / `gemini-4-argon`，静态开场作答与下一轮选项作答各完成两次模型调用。两回合各成功执行 `read` 两次、`update_state` 与 `offer_choices` 各一次，均以 `choices` 正常提交；无工具错误、无重试、无 400。

4 次调用合计已报告未缓存输入 27,552 token、输出 801 token，缓存与推理 token 为 0。报告与脱敏协议报文在 `.local/eval/gemini-opening/`。这一结果验证了完整运行时循环，复现实验中的 200 对照仅验证单次请求接受。

专项测试已通过：`go test ./internal/llm/gemini ./internal/runtime -run 'TestBody|TestStaticOpeningToolHistory|TestPrefixStableAcrossTurns' -count=1`。`task check` 通过：Go 静态检查 0 问题、Go 全量测试与前端 439 项测试均通过。
