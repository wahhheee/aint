# 决策记录：脚手架评审（2026-10-20）

搭建脚手架时发现的文档冲突、未写明的默认值与实现中的补充，已逐项确认。
编号与 `.local/docs/README.md` 的决策记录一致（D-23～D-40）；设计文档已同步修订。

| 编号 | 决策 | 代码位置 |
| --- | --- | --- |
| D-23 | `presentation.text_effects` 默认关闭；`features.ooc` 默认开启 | `internal/work/manifest.go` |
| D-24 | 平台默认的记忆目录约定写在 L0"记忆"节；作品 `memory.md` 在 L1 补充，作品写明替换时以作品为准 | `internal/prompts/l0.go` |
| D-25 | 有 `opening.md` 未写 `mode` 时为 `static`；命令未写 `present` 时为 `[slash]` | `internal/work/compile.go` |
| D-26 | 叙事标记每个非空行为一个段落 | `web/src/markup/parse.ts` |
| D-27 | 选项 `style` 只取作品样式标签，不内置 `danger`/`subtle`；作品有样式标签时才有该参数 | `internal/tools/builtins.go` |
| D-28 | `<cg>` 只在有非背景图片素材时启用；背景图只用于 `<scene bg>` | `internal/contextx/profile.go` |
| D-29 | 选项待答时玩家另有输入（命令 / 场外 / 继续）：工具结果如实写"没有从选项中作答"，输入照常跟在后面 | `internal/contextx/envelope.go` |
| D-30 | 内置标签 `<ooc>`（随 `features.ooc`）：AI 的场外回应写在其中，浅色块显示，不进入原文检索 | `prompts/l0.go`、`markup/`、`web/src/markup/` |
| D-31 | "开发模式"拆为**开发目录**（作品链接本地目录）与**开发者选项**（设置开关，默认关；显示检查器、回合检查、完整状态与状态编辑；开发目录作品默认显示）。状态编辑属于开发者工具，只在显示开发者工具时可用 | `internal/settings`、阅读页 |
| D-32 | `engine` 比较独立的**作品能力版本**（程序内置，目前 `1.0`），与程序发布版本解耦 | `internal/work/engine.go` |
| D-33 | 静态开场节点存两条消息：合成的 user `<input kind="opening">（开场）</input>` + 助手开场正文，历史始终以 user 开头 | `internal/save/service.go` |
| D-34 | 模型价格带货币；费用按货币分别汇总，不做汇率换算 | `internal/llm/usage.go`、`usage_log.currency` |
| D-35 | AI 读取设定文档时把 `[[id]]` 补全为路径（不改作品文件）；校验器检查链接目标 | `internal/work/links.go` |
| D-36 | 作者模板中的 `turn` 是对象，`turn.no` 为回合序号 | `internal/panel`、`internal/runtime/turn.go` |
| D-37 | 开发版本是"树哈希即版本身份"的例外：每部作品至多一个，原地更新，不参与去重 | `store/migrations/0001_init.sql`（部分唯一索引） |
| D-38 | 视觉语言取材线装书：宣纸白、墨、磁青、泥金；书架以线装书封面呈现 | `web/src/styles/globals.css`、`features/library/WorkCover.tsx` |
| D-39 | 实现补充：`GET /events`、`GET /saves?limit=`、`GET /saves/{id}` 返回呈现信息；回合 ID 即节点 ID；前端自动提交生成式开场；第 0 阶段块与执笔指令即时渲染；内置常用图标子集；TypeScript 6.0 | 见 `docs/architecture.md` |
| D-40 | 运行时代码 Apache-2.0；示例作品单独许可（《雾港来信》CC-BY-NC-4.0） | `LICENSE` |

另：D-08 修订为"Liquid 用于开场、执笔指令（只引用初始状态）、命令文本与面板；平台不设条件表达式类字段，模板内部的 `if`/`for` 不受此限"。
