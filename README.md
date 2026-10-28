# AINT

**AINT Is Not Tavern**，也读作 **Agentic Interactive Narrative Theatre**。

AINT 是在本机运行的 AI 互动小说运行时。玩家导入作品，配置模型 API，在浏览器中阅读、选择行动或自由输入。程序使用 Go 后端与 React 前端，发布为单个可执行文件，作品、设置和存档保存在本机。

作品是一个包含 YAML、Markdown、Liquid 模板和素材的目录。作者可以编写执笔指令、人物与世界设定、多个开场、状态、命令、面板和主题。模型通过工具查阅资料、维护记忆与状态、生成正文和选项；上下文达到预算时，运行时整理记忆并保留近期回合。

**当前为开发预览。** 已支持 Anthropic Messages、OpenAI Responses、Gemini 三种协议，作品导入与更新、开发目录、分支、重写、音频、用量与费用、调试检查器和访问密码。后续工作包括长篇真实模型评测、示例定稿和发布候选版验收，见[项目进度](docs/progress.md)。

## 开始使用

从[发布页](https://github.com/wahhheee/aint/releases)选择对应系统与处理器的压缩包，解压后运行 `aint.exe`（Windows）或 `./aint`（macOS / Linux）。浏览器打开后，在「设置 → 模型」中添加供应商并设为主笔，再从书架开启存档。

书架自带《倒悬的街角》和《第一个故事》。《倒悬的街角》包含三个静态开场、人物与世界设定、状态面板和作品命令；《第一个故事》展示最小作品结构。

启动、模型配置、作品导入、阅读操作、数据备份和局域网访问见[使用说明](docs/usage.md)。

## 从源码运行

需要 Go ≥ 1.27、Node ≥ 22、pnpm 11 和 [Task](https://taskfile.dev)。详细版本与检查工具见[开发环境](docs/development.md#1-环境)。

```bash
task setup
task dev
```

访问 <http://localhost:5173>。开发服务使用固定的假模型脚本，数据存放在 `.devdata/`。连接真实模型时，在两个终端分别运行 `task dev:server:real` 和 `task dev:web`，然后在设置中添加模型。

| 命令 | 用途 |
| --- | --- |
| `task check` | 静态检查与 Go、前端单元测试 |
| `task test:e2e` | 浏览器端到端测试 |
| `task fmt` | 格式化代码 |
| `task build` | 构建嵌入前端的程序到 `bin/` |
| `task validate -- <作品目录>` | 校验作品 |
| `task prompt -- <作品目录>` | 查看组装后的提示词与工具定义 |
| `aint --help` | 查看命令行用法 |

## 文档与目录

- [文档索引](docs/README.md)：阅读入口与术语。
- [创作指南](docs/authoring.md)：创建、试玩、校验、打包和更新作品。
- [aint-work 技能](skills/aint-work/SKILL.md)：供 AI 助手创作作品的流程，附[格式参考](skills/aint-work/references/format.md)、写作和设计说明。
- [架构](docs/architecture.md)：分层、回合生命周期、前后端契约和扩展位置。
- [开发与调试](docs/development.md)：开发命令、测试、排查和发布构建。
- [项目进度](docs/progress.md)、[设计决策与验证记录](docs/notes/README.md)。

```text
cmd/aint/         命令行入口
internal/         Go 后端
web/              React 前端与浏览器测试
examples/works/   随程序和发布包分发的作品
skills/aint-work/ 作品创作技能与格式参考
schemas/          作品 JSON Schema 与编辑器配置
testdata/         测试作品、假模型脚本和共享测试向量
docs/             使用、创作、开发文档与进度
```

## 许可

Copyright 2026 wahhheee。

运行时代码、文档和技能采用 [Apache-2.0](LICENSE)。《倒悬的街角》与测试作品《雾港来信》采用 CC-BY-NC-4.0，《第一个故事》采用 Apache-2.0；各作品的 `work.yaml` 标明作者与许可证。
