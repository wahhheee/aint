# CLAUDE.md

给在本仓库工作的 AI 助手的说明。

## 项目

演义（Yanyi）：以作品为单位、本地运行的 AI 互动小说运行时。Go 后端 + React 前端，单可执行文件分发。
当前处于脚手架阶段：架构与骨架就位，大量功能以 `TODO(任务编号)` 标注。

## 先读这些

- `docs/progress.md`：**进度**——每个任务的状态、已有与缺口、建议的下一步、按功能找任务；搜任务编号找代码
- `docs/architecture.md`：分层与依赖方向、回合生命周期、前后端契约、**扩展指南（想加 X 改哪里）**
- `docs/development.md`：开发命令与**调试手册**
- `docs/notes/decisions.md`：评审中确认的决策（D-23～D-40）；`docs/notes/open-questions.md`：待确认事项
- `.local/docs/`（不入库）：需求、技术方案、作品格式规范、交互设计、实施计划。`TODO(M3-6)` 这类编号对应实施计划

## 约定

- 注释、文案、提交信息使用**简体中文**；提交遵循 Conventional Commits，如 `feat(runtime): …`
- 推进或完成任务后按 `docs/progress.md` 顶部的"更新规则"更新它（任务状态、总览计数、进度日志）
- 未完成处写 `TODO(任务编号)：说明`；文档冲突或需要用户拍板的写 `TODO(待确认)` 并记入 open-questions.md
- 文档只是设计讨论，遇到不合理或需要抉择的地方先与用户确认，不要死磕文档
- 后端只允许向下依赖（见 architecture.md 第 2 节）；JSON 经 `internal/pkg/jsonx`；业务错误用 `internal/pkg/errs`
- 修改前后端契约（types / events / errors / routes）时两边同步
- 修改平台提示词文本时递增 `internal/prompts.Version`
- 前端文案写进 `web/src/i18n/zh-CN.ts`

## 常用命令

```bash
task dev        # 后端（假模型）+ 前端，访问 http://localhost:5173
task test       # Go + 前端测试
task lint       # go vet、gofmt、ESLint、tsc
task prompt -- examples/works/misty-harbor   # 查看组装后的提示词
```

在 `web/` 内执行 pnpm（不要在仓库根用 `pnpm -C web`，见 development.md 常见问题）。
