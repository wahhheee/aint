# AINT

**AINT Is Not Tavern**，也读作 **Agentic Interactive Narrative Theatre**。

以"作品"为单位、本地独立运行、模型供应商自由的 AI 互动小说运行时。

- **作品**是一个普通文件夹：执笔指令、设定、状态、命令、界面、素材，纯文本 + 图片/音频，可以 git 管理、打包分发；
- **玩家**下载一个可执行文件，配置自己的 API，导入作品即可游玩——无账号、无云端依赖，数据都在程序旁的 `aint-data/`；
- **运行时**以现代 Agent 技术栈驱动 AI 执笔：工具调用、按需读取资料、自主维护记忆、自动整理上下文、精细的缓存与 token 控制。

> **当前状态：开发预览。** 可以用自己的模型（Anthropic Messages、OpenAI Responses、Gemini 三种协议，含中转站与兼容接口）
> 游玩内置示例或导入的作品：流式正文与选项、状态与面板、记忆与原文检索、长篇游玩时自动整理上下文、分支（重新生成、修改、回到此处、切换）都已可用。
> 用量统计、后台助手、存档标题、音频、GitHub 导入、检查器等尚未完成（代码中以 `TODO(任务编号)` 标注）；首次发布前界面、接口与作品格式仍会调整，
> 不保证旧数据可用。每个任务的状态与下一步见 [进度](docs/progress.md)。

## 快速开始

需要 Go ≥ 1.27、Node ≥ 22、pnpm、[Task](https://taskfile.dev)；静态检查另需 [golangci-lint](https://golangci-lint.run)（版本见 [开发文档](docs/development.md)）。

```bash
task setup
```

```bash
task dev
```

然后访问 <http://localhost:5173>。`task dev` 会同时启动：

- 后端：`--fake-llm demo`（内置演示脚本，不需要任何 API Key）、调试日志、模型调用追踪，数据在 `.devdata/`；
- 前端：Vite 开发服务器，`/api` 代理到后端。

书架上有两部内置示例作品：《雾港来信》（状态、命令、样式标签、面板、舞台背景）与《第一个故事》（只有两个文件的最小作品）。

使用自己的模型：`task dev:server:real` 代替假模型启动后端，在设置 → 模型中添加供应商并设为主笔（见 [开发文档](docs/development.md) 2.1）。

## 常用命令

| 命令 | 作用 |
| --- | --- |
| `task dev` | 开发：后端（假模型）+ 前端 |
| `task check` | 提交前检查：全部静态检查 + 全部测试（与 CI 相同） |
| `task test` / `task lint` / `task fmt` | 测试 / 静态检查（需要 golangci-lint）/ 格式化 |
| `task build` | 构建单文件可执行程序到 `bin/`（前端已嵌入） |
| `task prompt -- <作品目录>` | 打印 AI 实际收到的完整提示词与工具定义（调试提示词组装） |
| `task validate -- <作品目录>` | 校验作品，输出带行号的问题列表 |
| `aint --help` | 可执行文件的全部子命令与参数 |

## 目录

```
cmd/aint/         命令行入口：serve | work validate|prompt | version
internal/         后端（分层见 docs/architecture.md）
web/              前端（React + Vite），构建产物嵌入二进制
examples/works/   内置示例作品（同时是测试夹具）
schemas/          作品格式 JSON Schema（TODO(M6-4)）
testdata/         测试数据（假模型脚本、标记解析向量等）
docs/             架构、开发与调试指南、设计决策记录
```

## 文档

- [进度](docs/progress.md)：里程碑与任务状态、现在能用什么、建议的下一步、按功能找任务
- [架构](docs/architecture.md)：分层、依赖方向、一个回合的生命周期、前后端契约、扩展指南
- [开发与调试](docs/development.md)：环境、日常开发、**调试手册**、测试、构建、提交规范
- [决策记录](docs/notes/decisions.md)：已确认的决策；[待确认事项](docs/notes/open-questions.md)
- 需求、技术方案、作品格式规范、交互设计、实施计划：在本地的 `.local/docs/`（不入库）

## 许可

运行时代码以 [Apache License 2.0](LICENSE) 发布。示例作品单独许可，见各作品的 `work.yaml`（《雾港来信》为 CC-BY-NC-4.0）。
