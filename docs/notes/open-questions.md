# 待确认事项与实现中的决定

> 记录脚手架阶段发现的**文档冲突**、**文档未覆盖的细节**以及**偏离文档的实现选择**。
> 每条注明当前做法与代码位置；确认后在此标注结论，必要时同步修订 `.local/docs/`。
> 日期：2026-10-20

## A. 文档之间的冲突（需要确认）

| # | 冲突 | 当前做法 | 位置 |
| --- | --- | --- | --- |
| A1 | 规范 5.1 / 技术方案 16.2 规定 `presentation.text_effects` 与 `features.ooc` **默认开启**；但技术方案 8.6 描述"最小作品组装后只剩……消息格式中的 `<input>` 与 `<notes>`"，其中没有叙事标记一节和 `<ooc>` 说明 | 按字段默认值实现：最小作品也会出现 `<pause>/<fx>/<aside>` 与 `<ooc>` 的说明。若希望最小作品更干净，可把这两个开关的默认值改为关闭 | `work/manifest.go` `defaultManifest`；`contextx/builder_test.go` |
| A2 | 技术方案 12.3 说"默认记忆约定（目录建议）写入 L1"；但 20.2 的 L1 片段只在作品提供 `prompts/memory.md` 时才出现"记忆约定"一节，8.6 的最小作品也没有 | 按 20.2：只有作品提供 memory.md 时才出现 | `contextx/l1.go` |

## B. 文档未写明的默认值（暂定，需要确认）

| # | 问题 | 暂定 | 位置 |
| --- | --- | --- | --- |
| B1 | 有 `prompts/opening.md` 但 `opening.mode` 未设置时的默认值 | `static`（直接展示作者写好的开场，零 token） | `work/compile.go` `prompts()` |
| B2 | 命令的 `present` 未设置时的默认值 | `[slash]` | `work/compile.go` `commands()` |
| B3 | 叙事标记的分段规则：规范写"空行分隔"，但模型常用单个换行分段 | 每个非空行视为一个段落（两种写法都能正确显示）。Go 端 `markup.PlainText` 不受影响 | `web/src/markup/parse.ts` |
| B4 | `assets.image` 特征是否包含背景图 | 不包含：`<cg>` 只在有非背景图片时启用；背景图只用于 `<scene bg>` | `contextx/profile.go` |

## C. 偏离或补充技术方案的实现选择（已实施，如有异议可调整）

| # | 内容 | 原因 |
| --- | --- | --- |
| C1 | 任务编排用 Taskfile 而不是 Makefile | Windows 无 make；已与你确认 |
| C2 | 新增 `GET /api/v1/events`（全局事件流：作品重载、存档标题） | 技术方案只列了存档事件流，但 `work.reloaded`、`save.titled` 发生在书架页 |
| C3 | 新增 `GET /api/v1/saves?limit=`（最近游玩的存档） | 书架的"继续阅读"需要跨作品的最近存档 |
| C4 | `GET /saves/{id}` 返回"呈现信息"读模型（存档 + 作品界面配置 + 模型摘要），而不只是存档 | 阅读页一次请求拿到渲染所需的全部配置，前端不需要理解作品格式 |
| C5 | 回合 ID 即提交后的节点 ID | 前后端、日志、事件用同一个 ID 关联 |
| C6 | 第 0 阶段块不在建档时存 blob，而是每次构建时由初始状态与初始记忆即时渲染 | 二者不可变，结果字节稳定；避免 save 依赖 contextx 造成循环依赖。整理产生的阶段块仍按技术方案存 blob |
| C7 | 执笔指令中的 Liquid 在构建时以存档初始状态渲染（结果按初始状态哈希缓存），而不是建档时渲染并存储 | 同上，确定性且无需额外存储 |
| C8 | 生成式开场由前端在 `needs_opening` 时自动提交 `kind=opening` | 建档接口保持同步、快速返回；开场的流式过程与普通回合一致 |
| C9 | 作品图标只收录常用的约 40 个 lucide 图标，未收录的显示占位 | 全量按需加载会为四千多个图标各生成一个构建产物，嵌入二进制后文件数暴增（TODO：找到合适的按需加载方式） |
| C10 | TypeScript 固定 6.0.x | `typescript-eslint` 目前只支持 `<6.1` |
| C11 | 界面配色没有照搬交互设计 10.1 的色值：底色用偏冷的宣纸白 `#f2f1ec`，强调色用"泥金"而非棕色，主操作用"磁青" | 你说明界面文档仅供参考；取材线装书（磁青封面、泥金题字），避免常见的"奶油底 + 赭陶色"模板感。色值集中在 `web/src/styles/globals.css`，调整一处即可 |

## D. M0 核对清单的已知结论

| # | 结论 |
| --- | --- |
| 17 | `modernc.org/sqlite` v1.60.1 的 FTS5 + trigram 在 Windows/amd64 可用，中文子串检索正常（`store/store_test.go`）；其余平台待 CI 确认 |
| — | Go 1.27 默认启用 `encoding/json/v2`（`internal/buildcfg` 的 baseline），决策 D-15 成立，无需 `GOEXPERIMENT` |
