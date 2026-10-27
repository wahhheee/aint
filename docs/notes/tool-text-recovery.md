# 正文伪工具调用的纠正

## 行为与边界

回合运行时识别正文中以本次可用工具名开头的 XML 标签，例如 `offer_choices`、`update_state` 和 `read`。标签内容只用于识别错误，不解析成工具参数，也不直接执行。

- 普通故事与叙事标记实时显示；工具标签及其内容不进入 `turn.text`、显示项或原文索引。闭合标签后的故事继续显示；未闭合标签的剩余内容留在原始消息中。
- 代码引用、反斜杠转义、作品声明的同名样式标签、未知标签原样保留。不按普通文字中的工具名删除旁白，也不解释任意 JSON、HTML 或模型私有工具格式。
- 一条回复混有伪调用时，其中的原生工具调用整批退回，逐个回填“未执行”的错误结果，避免纠正时重复执行状态增量或记忆写入。
- 运行时追加纠正提示，由模型重新发起原生调用；工具注册表继续负责参数校验、状态维护、选项和其他副作用。已经发现但尚未成功执行的工具会一直记为待纠正。
- 每回合最多发起两次纠正请求，仍受总调用数、工具次数、费用与墙钟预算限制；取消后不继续纠正。纠正调用按实际报告的用量计费。
- 模型持续输出伪调用，或结束时仍有未完成的纠正，回合以 `tool_format` 中断并显示原因；有正文时保留正文，没有可保留内容时发送可重试的 `turn.failed`。

### 发送历史

纠正中的本回合保留模型原始消息与 `Native`，后接纠正提示和实际调用。加载已提交回合用于后续请求时，`toolHistoryCleaner` 构建发送副本：

1. 去掉助手正文中的伪工具标签及内容，保留故事。
2. 成对移除因伪调用而整批退回的原生调用与错误结果，移除平台纠正提示。
3. 保留成功执行的原生调用、工具结果和玩家选项作答，保持调用 ID 配对。
4. 自首个修改位置起，去掉所有后续消息的 `Native` 和推理，按中立内容重建请求。推理中可能重复错误示范，协议签名也不能沿用被改写的历史。

存储消息不变，检查器和原始报文仍可查看错误输出；判断错误示范是否继续回传，应检查后续请求的上下文。清理适用于当前上下文中的已有回合，不需要重建存档。相同历史的清理结果确定、幂等；清理会使修改位置起的历史缓存失效，稳定层的前缀哈希不受影响。

这项处理独立于 `features.force_choices`。模型没有输出伪调用、自然结束时未给选项，仍按作品设置决定是否补调。整理会读取清理后的历史，但整理生成过程、后台助手与标题生成不使用正文过滤器。已经进入整理摘要的伪调用不在这项清理范围内。

代码：`internal/runtime/tool_text.go`、`tool_history.go`、`history.go`、`turn.go`；提示词版本：`2026.10.26-2`；决策 D-49。L0 保留各工具的正常用法，并说明工具名称不能作为叙事标签。

## 提示词对照实验（2026-10-26）

DeepSeek Flash、低推理、《雾港来信》生成式开场，Anthropic 兼容与 Responses 接口各重复三次。每次从无助手历史的完整开场请求开始，读取资料后继续，API 工具定义完全相同。实验组只移除选项用法、选项工具说明和交还选项的要求，保留通用调用规则与其他工具提示。资料读取使用真实作品文件，记忆写入使用临时映射；这组探测不用于验证运行时状态提交。

| 提示词 | 独立开场数 | 伪工具调用 | 自然结束且无选项 | 原生选项调用 |
| --- | ---: | ---: | ---: | ---: |
| 完整提示 | 6 | 4 | 2 | 0 |
| 选项只保留 API 工具定义 | 6 | 5 | 1 | 0 |

这组小样本没有显示删除选项提示的收益，不能据此判断训练数据是否来自某种产品。补充探测中，将选项工具统一改名为 `await_player`，四次均出现以新名称为标签的伪调用；增加原生调用示例或模型私有格式说明也未表现出稳定效果。这些实验变体不进入正式提示词。

探测脚本与结果分别在 `.local/scripts/probe_choices_prompt.py`、`.local/eval/choices-prompt-only/`，均不入库。

## strict 对照实验（2026-10-27）

[工具调用文档](https://api-docs.deepseek.com/zh-cn/guides/tool_calls/)规定，strict 通过 Beta 地址与各函数的 `strict: true` 启用，对象的属性必须全部列入 `required`，并设置 `additionalProperties: false`。[思考模式文档](https://api-docs.deepseek.com/zh-cn/guides/thinking_mode/)要求携带工具的后续请求完整回传 `reasoning_content`，包括未调用工具的助手回复。

### 启用验证与 schema

同一份最小工具定义分合规与不合规两种，不合规版本缺少必填声明与 `additionalProperties: false`：

| 官方端点 | `strict: true` 合规 schema | 不合规 schema |
| --- | --- | --- |
| `/beta/chat/completions` | HTTP 200 | HTTP 400，要求全部属性必填 |
| `/beta/responses` | HTTP 200 | HTTP 400，同一校验错误 |
| `/responses` | HTTP 200 | HTTP 200 |
| `/anthropic/v1/messages` | HTTP 200 | HTTP 200 |

只有 Beta 端点在这项探测中表现出严格 schema 校验；普通地址上的 HTTP 200 不足以证明 strict 生效。完整工具集合也通过 Beta schema 预检。

作品原始 schema 含可选字段、`minItems` / `maxItems` 和未限定类型的 `update_state.value`，不能直接全部设为 strict。实验用 schema 将可选字段表示为必填且可为 null，移除不支持的数组长度约束，并把状态值限定为字符串、数字、布尔值或字符串列表。因此这份实验 schema 不覆盖任意对象状态，不适合作为通用生产转换器。嵌套 `anyOf` 的初始版本被服务端拒绝，展开后的有效实验单独记录为 `story-v2-*`，拒绝请求不计入行为对照。

### 完整提示词的行为对照

DeepSeek Flash，完整平台提示词 `2026.10.26-2`、《雾港来信》生成式开场、中等篇幅（800–1500 字），包含相同的资料和图片。三组均使用 `/beta/responses`、`tool_choice: auto`；关闭思考（`reasoning.effort: none`）和低思考各重复三次。Responses 的全部推理 item 原样回传，Chat Completions 的助手消息连同 `reasoning_content` 完整回传。不追加纠正提示、不清理生成历史，以免把运行时补救混入 strict 效果。

| schema 与 strict | 独立开场数 | 正文伪调用 | 无选项结束 | 原生选项调用 | API 请求数 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 原始 schema，strict 关闭 | 6 | 6 | 0 | 0 | 13 |
| 兼容 schema，strict 关闭 | 6 | 5 | 1 | 0 | 13 |
| 相同兼容 schema，strict 开启 | 6 | 5 | 1 | 0 | 17 |

`/beta/chat/completions` 的 strict 模式另按关闭思考、低思考各复核一次，两次都出现正文伪调用，共五次 API 请求。有效 strict 组生成的 28 次原生调用参数均通过对应 schema 子集校验，但仍将 `offer_choices` 写成正文标签；没有原生选项调用。原生参数合规与正确选择调用通道是两个不同的观测指标。

这组样本没有显示 strict 对正文伪调用的收益，与文档限定的“输出函数调用时约束参数”范围一致。结果只针对该模型、作品和提示词，不代表其他模型或参数约束任务无效。正式适配器未启用 strict，运行时继续使用有界纠正与历史清理。

实验工具在临时内存中模拟记忆和状态，资料从实际作品文件读取，不修改存档；这不是完整运行时验收。可复现脚本为 `.local/scripts/probe_deepseek_strict.py`，有效行为记录为 `.local/eval/deepseek-strict/story-v2-*`，汇总为 `stories-responses.json`；所有文件均不入库。`python .local/scripts/probe_deepseek_strict.py` 执行端点探测，追加 `story 3` 执行三组行为对照，会使用本地凭据产生真实调用费用。

## 真实运行时验证（2026-10-26）

使用独立临时数据库与真实工具注册表。短篇幅三回合覆盖生成式开场、选项作答（无选项时继续）、明确状态维护；开启发送历史清理与当前 L0 格式说明：

| 接口 | 推理设置 | 回合 | 模型调用 | 纠正请求 | 结果 |
| --- | --- | ---: | ---: | ---: | --- |
| Anthropic 兼容 | off | 3 | 6 | 3 | 通过 |
| Anthropic 兼容 | low | 3 | 7 | 2 | 通过；第二回合自然结束无选项 |
| Responses | off | 3 | 7 | 1 | 通过 |
| Responses | low | 3 | 6 | 1 | 通过；第二回合自然结束无选项 |

合计 12 回合、26 次模型调用、7 次纠正请求，四组均实际更新信件柜场景状态。Anthropic 在明确状态维护的第三回合仍可能误写 `update_state`，不能把问题限定为选项工具；没有选项但允许自由输入的自然结束符合运行时规则。

Anthropic / low / 中等篇幅另跑连续六回合：

| 回合 | 1 | 2 | 3 | 4 | 5 | 6 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 模型调用 | 4 | 1 | 1 | 1 | 2 | 3 |
| 纠正请求 | 1 | 0 | 0 | 0 | 0 | 0 |

六回合全部成功交还选项，共 12 次模型调用；第三回合实际更新状态；每回合首个请求都不含历史伪调用。这个结果说明清理错误示范可以改善连续游玩的表现，但不保证首次输出和长对话完全无误。测试同时使用历史清理与格式说明，不能将全部收益归因于其中一项。

本地证据：`.local/eval/deepseek-tool-text/clean-history.log`、`history-medium.log` 及同名目录，包含正文和脱敏协议报文，不入库。推理设置是请求档位，供应商报告的推理 token 可能为零，或在 off 下仍有报告值。

探测中 `tool_choice: required` 在低推理模式返回 HTTP 400，因此运行时不依赖强制工具选择。官方协议说明见 [DeepSeek Responses](https://api-docs.deepseek.com/guides/responses_api/)；支持情况以对应模型和接入的实测为准。

## 回归与复现

- `TestToolTextFilter`：每个字节分割点、属性 / 子元素 / JSON 内容、自闭合与截断标签、代码引用、正常标签、同名作品样式与未知名称。
- `TestToolTextRecovery`：真实工具执行、状态不重复递增、下一回合选项回填与历史清理、存储原文与协议数据保留、用量、预算、拒答、网络失败、持续错误与遗漏操作。
- `TestToolHistoryCleaner`：分文本块识别、退回调用与结果配对移除、成功调用保留、推理与签名失效、存储不变、跨回合分组一致与幂等。
- `TestToolTextCancel`：取消时保留故事、隐藏尚未完成的工具标签前缀，不发起纠正请求。

`task check` 通过：静态检查零问题、Go 全量测试通过、前端 434 项通过。

`TestToolTextLive` 默认跳过，设置凭据后会产生真实调用费用。默认六回合，第三回合明确要求更新状态；每回合至多 8 次调用、90 秒，每次至多 4000 输出 token，不自动重试网络错误。PowerShell 运行全部接口与推理档位：

```powershell
$env:AINT_TOOL_TEXT_LIVE = (Resolve-Path .local/secrets/live.json).Path
go test ./internal/runtime -run '^TestToolTextLive$' -count=1 -v -timeout 35m
Remove-Item Env:AINT_TOOL_TEXT_LIVE
```

`AINT_TOOL_TEXT_TURNS` 可设 3–12（默认 6），`AINT_TOOL_TEXT_PACE` 可设 `medium`（默认 `150-250`），`AINT_TOOL_TEXT_OUT` 指定报告目录。仅测试 Anthropic / low 时使用 `-run '^TestToolTextLive$/^anthropic$/^low$'`。配置与已有 `live.json` 相同，只选择模型名以 `deepseek-` 开头的供应商；离线回归不读取凭据。
