// Package llm 是模型适配层：与供应商无关的中立类型、Provider 接口、错误分类、重试与调试追踪。
//
// 分层：
//
//	runtime / compact / helper   ← 只认识本包的中立类型
//	        │
//	llm（本包）：Request / Message / Part / Event / Usage / StopReason / Provider
//	        │
//	llm/openai  llm/anthropic  llm/gemini  llm/fake   ← 适配器：SDK 类型只出现在各自包内
//
// 关键约定（《02-技术方案》第 7 节）：
//   - Message.Native 原样保存协议输出（推理签名、加密推理等），同协议同模型的后续请求原样回传；
//     换供应商时由适配器丢弃不属于目标协议的 Native，退回中立表示。
//   - Part.CacheMark 表示"在此块之后放缓存断点"，是否支持由适配器决定。
//   - 停止原因、用量统一归一化（stop.go、usage.go），上层不处理协议差异。
//   - 装饰器（retry.go、trace.go）以 Provider 包装 Provider 的方式叠加能力，组装见 provider 包。
//
// 调试：启动参数 --trace-llm 会把每次调用的请求与流事件写入 <数据目录>/logs/llm-trace/。
package llm
