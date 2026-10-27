# 多开场

作品在 `work.yaml` 的 `openings` 中声明至少一个开场。列表顺序决定选择界面的顺序，第一项为默认开场；单开场作品也使用列表。开局设置与阅读页共用开场名称、介绍，只有一个开场时不显示选择控件。多开场格式属于作品能力版本 `2.0`，可用 `engine` 声明适用范围。

```yaml
engine: ">=2.0 <3.0"
openings:
  - id: post-office
    title: 邮局醒来
    description: 你攥着一封寄给明天的信，在邮局醒来。
    mode: static
    file: prompts/openings/post-office.md
    initial_state:
      situation: 雾越来越浓，老陈不见了。
  - id: arriving
    title: 乘船入港
    description: 渡船即将靠岸，你第一次来到雾港。
    mode: generate
    file: prompts/openings/arriving.md
    initial_state:
      situation: 渡船即将靠岸，码头上有人正在等你。
      trust.林舟: 0
```

## 作者配置

| 字段 | 规则 |
| --- | --- |
| `id` | 必填，1–64 个小写字母、数字、下划线或连字符，以字母或数字开头；作品内唯一，作为稳定身份 |
| `title` | 必填，非空名称 |
| `description` | 可选，选择列表中的简短介绍 |
| `mode` | 必填，`static` 或 `generate` |
| `file` | 作品内的文件路径；静态开场必填，生成式开场可省略 |
| `initial_state` | 可选，状态路径到初始值的映射，例如 `trust.林舟: 0` |

静态文件正文是 Liquid 模板，渲染后直接展示，不调用模型。frontmatter 可声明 `choices`，每项包含唯一的 `id`、非空 `label`、可选 `detail` 与可选 `command`；选项以 `offer_choices` 工具调用写入助手历史。

```markdown
---
choices:
  - {id: a, label: 拆开那封信}
  - {id: b, label: 跟着脚步声出去, command: /follow 林舟}
---
```

`command` 是选中即执行的命令，与模型给出的选项一致：可以是 `present` 含 `slash` 或 `button` 的作品命令，也可以是 `/ooc 内容`（作品开启场外对话时）或 `/continue`。参数按位置或 `key=value` 填写，带空格的值用引号。选中后先按命令规则确认，再提交命令；所选选项随输入一并记录，主笔在工具结果中看到玩家的选择。校验器检查命令存在、可从选项执行且必填参数齐全；`choices` 中的其他字段报告 `W-UNKNOWN-FIELD`。

生成式文件正文是开场指引，渲染后发送给主笔；生成式文件不声明 `choices`，选项由模型提供。省略文件时，主笔依据作品执笔指令生成。平台只生成当前选中的开场。

初始状态的覆盖顺序为：**状态定义默认值 → 所选开场的 `initial_state` → 开局表单**。开局表单的有效值随存档保存，切换时重新应用；普通游玩中的状态变化不影响这份表单。

覆盖使用状态系统的 `set` 语义，检查路径与类型；列表和映射整体赋值，对象缺少的字段取状态定义默认值。不要在同一开场同时覆盖父路径及其子路径，例如 `player` 与 `player.name`，校验器会报告冲突。开局表单与初始 `memory/` 文件由整部作品共用。

每个开场文件都会校验 Liquid 语法与模板引用。使用 `aint work validate <目录>` 校验；使用 `aint work prompt <目录> --opening arriving` 检查所选开场的初始状态、生成指引与提示词组装。

## 切换与锁定

- 开场本身不计入玩家首轮；中断开场的“继续”也属于开场。
- 开场后，第一次玩家触发的模型回复成功保存时，存档永久锁定开场。选项、自由输入、作品命令、继续和场外对话均计入。
- 生成、整理或事件连接尚未同步时，切换入口暂时禁用。停止、失败、超时或回合预算耗尽不视为成功完成。
- 切换清除存档中的试读历史与分支，重建初始状态、记忆和开场；不保留旧开场的状态变化或手动编辑。有未完成的玩家互动时先提示确认。
- 保留存档名称、开局表单、模型及游玩设置；已产生的用量继续归属于原存档。生成式开场切换回来会重新生成，选择当前项不重新开始。
- 锁定不随回退、删除分支或作品升级解除；复制存档继承所选开场与锁定状态。其他开场通过新建存档体验。

## 存档与接口

`saves.opening_id` 保存所选开场，`opening_locked` 保存不可逆的锁定状态，`initial_form` 保存有效开局表单。锁定与成功的玩家回合在同一事务提交；开场及其续写通过节点 `meta.opening` 标记。手动修改正文不产生首次模型交互。

创建存档可提交 `opening_id`，省略时取第一项。作品详情返回 `openings` 摘要列表；阅读页呈现信息返回 `opening.options`、`can_switch`、`has_unfinished_turn`，由服务端判断是否允许切换。

`PUT /api/v1/saves/{id}/opening` 接收 `opening_id` 与 `expected_head_id`。运行器占用现有单写者槽位，存档层先校验并渲染，再在事务中重建初始节点与开场；锁定、任务忙碌或页面的 head 过期时返回冲突。切换完成发送 `timeline.changed`（`reason: opening`）及最新面板，前端清除旧草稿并重置时间线查询。

生成式开场按当前起点自动提交 `kind: opening` 和 `expected_head_id`。多个阅读页同时提交时，冲突的页面重新同步会话，不展示自动开场失败。原文、状态、记忆和背景只使用新开场的进度线。
