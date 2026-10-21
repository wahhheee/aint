# AGENTS.md

给在本仓库工作的 AI 助手的说明（`CLAUDE.md` 导入本文，两者是同一份说明，只改本文）。

## 项目

演义（Yanyi）：以作品为单位、本地运行的 AI 互动小说运行时。Go 后端 + React 前端，单可执行文件分发。
当前处于脚手架阶段：架构与骨架就位，大量功能以 `TODO(任务编号)` 标注。

## 先读这些

- `docs/progress.md`：**进度**——每个任务的状态、已有与缺口、建议的下一步、按功能找任务；搜任务编号找代码
- `docs/architecture.md`：分层与依赖方向、回合生命周期、前后端契约、**扩展指南（想加 X 改哪里）**
- `docs/development.md`：开发命令与**调试手册**
- `docs/notes/decisions.md`：已确认的决策（D-23～D-43）；`docs/notes/open-questions.md`：待确认事项
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
