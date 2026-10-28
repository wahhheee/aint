# 设计笔记

设计决策与验证记录。验证文档写明日期、场景和观察结果；当前任务状态与变更见[项目进度](../progress.md)。

| 文档 | 内容 |
| --- | --- |
| [设计决策](decisions.md) | D-01～D-73 的当前约定与实现位置 |
| [待确认事项](open-questions.md) | 需要进一步明确的问题 |
| [API 与运行时验证](api-verification.md) | 三协议、缓存、工具、记忆与长篇游玩实测 |
| [Gemini 开场验证](gemini-turn-validation.md) | 工具结果排序与真实开场作答 |
| [正文伪工具调用](tool-text-recovery.md) | DeepSeek 工具文本纠正与历史清理 |
| [M5 体验验证](m5-verification.md) | 阅读、演出、交互和浏览器回归 |
| [M6 分发验证](m6-verification.md) | 导入、版本、访问控制与发布包 |
| [重写与补选项验证](turn-operations-verification.md) | 分支、快照、记忆核对与只读选项 |

本地真实调用的密钥、原始报文和报告位于忽略目录。公开验证文档保留测试范围与结论，仓库中的真实调用测试默认跳过，可按[开发文档](../development.md)提供自己的模型配置运行。
