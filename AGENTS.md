# AGENTS.md

给在本仓库工作的 AI 助手的说明（`CLAUDE.md` 导入本文，两者是同一份说明，只改本文）。

## 项目

AINT（AINT Is Not Tavern / Agentic Interactive Narrative Theatre）：以作品为单位、本地运行的 AI 互动小说运行时。Go 后端 + React 前端，单可执行文件分发。
当前处于开发预览阶段：M0–M3 完成（真实模型、作品能力、记忆与整理、分支），M4–M5 完成，M6 进行中，未完成处以 `TODO(任务编号)` 标注。

## 先读这些

- `docs/progress.md`：**进度**——每个任务的状态、已有与缺口、建议的下一步、按功能找任务；搜任务编号找代码
- `docs/architecture.md`：分层与依赖方向、回合生命周期、前后端契约、**扩展指南（想加 X 改哪里）**
- `docs/development.md`：开发命令与**调试手册**
- `docs/notes/decisions.md`：已确认的决策（D-23～D-71）；`docs/notes/open-questions.md`：待确认事项
- `.local/docs/`（不入库）：需求、技术方案、作品格式规范、交互设计、实施计划。`TODO(M3-6)` 这类编号对应实施计划

## 现阶段不做向后兼容（D-43）

首次发布前，前后端在同一仓库、随同一个可执行文件发布，彼此是唯一的使用方。改动直接改到位，不要为不存在的旧版本、旧客户端写代码：

- API 与事件（types / events / errors / routes）：直接改名、删字段、改结构，前后端在同一次改动中一起改；不保留旧字段、旧路由或别名，不加 API 版本号，不写"旧格式也接受"的分支与降级逻辑
- 数据库：直接改 `internal/store/migrations/0001_init.sql`，不追加迁移；本地数据删 `.devdata/` 重建
- 设置、作品格式改定义时同步改示例作品与测试，不为旧写法留兼容
- 产品功能中的版本机制不在此列：作品 `format` / `engine` 检查、存档绑定作品版本、存档升级（M6-6）是需求本身，照常实现

## 约定

- 注释、文案、提交信息使用**简体中文**；提交遵循 Conventional Commits，如 `feat(runtime): …`
- 推进或完成任务后按 `docs/progress.md` 顶部的"更新规则"更新它（任务状态、总览计数、进度日志）
- 未完成处写 `TODO(任务编号)：说明`；文档冲突或需要用户拍板的写 `TODO(待确认)` 并记入 open-questions.md
- 文档只是设计讨论，遇到不合理或需要抉择的地方先与用户确认，不要死磕文档
- 提交前 `task check` 通过；误报用 `//nolint:<linter> // 原因` 就地说明，不要关闭检查（见 development.md 6.1）
- 后端只允许向下依赖（见 architecture.md 第 2 节）；JSON 经 `internal/pkg/jsonx`；业务错误用 `internal/pkg/errs`
- 修改前后端契约（types / events / errors / routes）时两边同步
- 修改平台提示词文本时递增 `internal/prompts.Version`
- 前端文案写进 `web/src/i18n/zh-CN.ts`

## 注释与文档写法

- **注释描述当前代码**：写清职责、行为、约束与边界，不记录修改过程。即使在修 bug，也不要写“修复 xxx”“将 xxx 改成 xxx”“原来……现在……”之类的注释。
- 对容易让人疑惑的实现，可以解释**为什么这样做**，例如控制复杂度、优化性能或满足具体业务约束。原因必须有明确依据；未提供原因时省略，不自行推测或补写理由。
- 用户让 AI 编写或修改代码不是技术或业务原因。注释、文档及变更记录中都不得用“按用户要求”“根据用户反馈”“用户让做的”“用户确认”等表述交代指令来源或充当理由。
- **项目内普通文档同样描述当前状态**，包括设计、架构、开发指南与决策说明；不夹带修改经过、新旧实现对比或对话过程。
- 只有专门记录变更的文档或章节（如 `docs/progress.md` 的“进度日志”、CHANGELOG、发布说明）可以记录变更；直接写新增、调整、删除或修复的事项，不写“从什么改成什么”“原来怎样、现在怎样”，也不记录用户指令来源。
- `TODO(任务编号)` 描述尚未完成的能力或约束，不充当修改日志。修改代码时同步维护附近注释和相关文档，删除过时内容；已有的上述表述一并清理。

## 常用命令

```bash
task dev        # 后端（假模型）+ 前端，访问 http://localhost:5173
task check      # 提交前：lint + test，与 CI 相同
task test       # Go + 前端测试
task lint       # golangci-lint（go vet、staticcheck、gosec 等）、go.mod 整洁、ESLint、tsc、Prettier
task fmt        # gofmt + goimports、Prettier
task prompt -- examples/works/misty-harbor   # 查看组装后的提示词
```

在 `web/` 内执行 pnpm（不要在仓库根用 `pnpm -C web`，见 development.md 常见问题）。
