# 作品格式参考（format 1，作品能力 2.0）

本文是作品与运行时之间的契约：字段、取值、默认值与校验规则。代码片段只示意语法，占位内容按作品替换。

## 目录

```
<作品目录>/
├── work.yaml              必需：作品清单
├── README.md              推荐：给玩家看的介绍与玩法（不发给执笔者）
├── LICENSE                可选
├── prompts/
│   ├── narrator.md        执笔指令（路径可由 narrator.instructions 改）
│   ├── openings/*.md      开场文件（由 work.yaml 的 openings 引用，路径自定）
│   ├── memory.md          可选：记忆约定补充
│   └── compaction.md      可选：整理上下文时需特别保留的内容
├── lore/**/*.md           可选：设定文档，任意层级；_index.md 为目录摘要
├── state.yaml             可选：状态定义
├── commands/*.md          可选：命令，一个文件一个命令，不递归子目录
├── ui/
│   ├── markup.yaml        可选：样式标签
│   ├── layout.yaml        可选：面板布局与命令栏
│   ├── panels/*.liquid    面板模板（ui/ 下任意 .liquid 都可被 render 引用）
│   ├── theme.yaml         可选：主题
│   └── custom.css         可选：自定义样式（由 theme.yaml 引用）
├── memory/                可选：记忆初始模板，复制到每个新存档的 /memory
├── assets/                可选：图片、音频、字体
├── assets.yaml            可选：素材元数据
└── .aintignore            可选：导入与打包时忽略的文件（gitignore 语法）
```

## 通用约定

- 文本文件 UTF-8（无 BOM），换行 LF；YAML 1.2；Markdown frontmatter 是开头 `---` 包围的 YAML。
- 路径相对作品根，用 `/`，不含 `..`，区分大小写。建议文件名用小写 ASCII。
- 作品 ID：`^[a-z0-9][a-z0-9.-]{2,63}$`，建议"作者.作品名"；用户没给作者名时用 `local.<作品slug>`。发布后不要更改。
- 文档、素材、开场 ID：`^[a-z0-9][a-z0-9_-]{0,63}$`；文档与素材省略时取文件名（去扩展名、转小写），中文文件名需在 frontmatter / `assets.yaml` 中写 `id`。
- 命令名取文件名：字母（含中文）、数字、`_`、`-`，1–24 字符。
- 样式标签名：`^[a-z][a-z0-9-]*$`。
- 保留名：命令 `ooc` `continue` `compact` `help` `rewrite` `choices`；标签 `scene` `cg` `say` `bgm` `sfx` `pause` `fx` `aside` `act` `ooc`（另有内置的 `<br/>`）。
- YAML 中含 `: `、`#` 或以特殊字符开头的字符串加引号；`engine` 的范围写成带引号的字符串。以半角引号开头、引号之后还有文字的值（如 `desc: "我"的名字`）会解析失败：整段加引号，或在中文里改用全角引号 `“”`。

## work.yaml

| 字段 | 必需 | 默认 | 说明 |
| --- | --- | --- | --- |
| `format` | ✓ | | `1` |
| `id` | ✓ | | 作品 ID |
| `version` | ✓ | | semver，如 `0.1.0` |
| `title` | ✓ | | 标题 |
| `openings` | ✓ | | 开场列表，至少一项（见下） |
| `engine` | | | 需要的作品能力版本范围，写 `">=2.0 <3.0"` |
| `subtitle` / `description` / `tags` | | | 副标题、一句话简介、标签（书架与详情页展示） |
| `authors` | | | `[{name, url?}]`；用户没给出作者名时省略 |
| `language` | | `zh-CN` | 正文语言 |
| `content_rating` | | | `general` \| `teen` \| `mature`，展示用，按作品实际内容填写 |
| `license` | | | SPDX 标识或说明；用户没指定时省略 |
| `cover` | | | 封面素材（ID 或 `assets/` 路径），书架上按 5:7 竖版居中裁切显示；只给界面用；不写时显示按标题生成的排版封面 |
| `requires.vision` | | `none` | `none` \| `preferred` \| `required`：作品的图片需要执笔者看见时写 `preferred`，开局时对不能看图的模型给出提示 |
| `narrator.instructions` | | `prompts/narrator.md` | 执笔指令文件 |
| `narrator.length` | | `medium` | 默认篇幅：`short`（约 300–600 字）\| `medium`（800–1500）\| `long`（2000–4000）\| `"最少-最多"`；玩家可在游玩中调整 |
| `narrator.reasoning` | | | 推荐推理强度 `off` \| `low` \| `medium` \| `high`，玩家可覆盖 |
| `new_game.fields` | | | 开局表单（见下） |
| `docs.default_load` | | `index` | 设定文档默认加载方式 |
| `changelog` | | | 本版本更新说明，存档升级时告知执笔者与玩家 |

开关（全部显式写出）：

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `features.free_input` | `true` | 允许自由输入；`false` 时玩家只能通过选项与命令行动，执笔者每回合给出选项 |
| `features.force_choices` | `false` | 正文自然结束而没有选项时，追加一次提示让执笔者补上选项；纯场外回应不触发 |
| `features.dice` | `false` | 提供 `roll_dice` 掷骰工具，玩家看到骰子卡片 |
| `features.timed_choices` | `false` | 允许限时选项（需要有效的默认项，1–3600 秒；玩家可在阅读设置中关闭限时） |
| `features.ooc` | `true` | 场外对话 `/ooc`；执笔者的场外回应写在 `<ooc>` 中 |
| `presentation.dialogue` | `plain` | `tagged`：执笔者用 `<say who>` 标注台词，显示说话人名字与颜色 |
| `presentation.stage` | `true` | 舞台层：有背景素材时启用 `<scene>` 背景与标题卡 |
| `presentation.text_effects` | `false` | 向执笔者提供 `<pause>`、`<fx>`、`<aside>` |

骨架：

```yaml
format: 1
id: <作者>.<作品名>
version: 0.1.0
title: <标题>
description: <一句话简介>
engine: ">=2.0 <3.0"
narrator:
  length: medium
openings:
  - {id: <开场ID>, title: <开场名>, mode: static, file: prompts/openings/<文件>.md}
features:
  free_input: true
  force_choices: false
  dice: false
  timed_choices: false
  ooc: true
presentation:
  dialogue: plain
  stage: true
  text_effects: false
```

### 开局表单 `new_game.fields`

| 字段 | 说明 |
| --- | --- |
| `key` | 写入的状态路径，必须由 `state.yaml` 中声明的顶层字段或 object 子字段构成（不能是 map 的键） |
| `label` | 显示名 |
| `type` | `text` \| `textarea` \| `number` \| `select` \| `multiselect` \| `toggle`，与状态类型对应（文本 → string，number → number，multiselect → list，toggle → boolean） |
| `default` / `options` / `min` / `max` / `max_length` / `hint` / `required` | 默认值、候选项、取值范围、提示与必填 |

表单值写进初始状态，执笔指令与开场可以用 `{{ state.<路径> }}` 引用。`aint work prompt` 用表单字段的 `default` 预览；没有 `default` 的字段在预览中为空。

### 开场 `openings`

作品至少声明一个开场，第一项为默认开场。

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

每个开场文件都会校验 Liquid 语法与模板引用。使用 `aint work validate <目录>` 校验；使用 `aint work prompt <目录> --opening <开场ID>` 检查所选开场的初始状态、生成指引与提示词组装。

首个玩家回合成功后开场永久锁定（包括场外对话）。此前可切换开场，切换会重建初始状态、记忆与历史，保留开局表单、游玩设置和累计用量。

### 正文结束后交还选项

作品可以在 `work.yaml` 中开启 `features.force_choices`：

```yaml
features:
  force_choices: true
```

该开关默认关闭，与 `features.free_input` 独立。开启后，平台提示词要求执笔者在每回合正文结束时调用 `offer_choices` 给出选项。允许自由输入时，玩家仍可以自行输入行动。

如果模型自然结束输出、尚未给出选项，平台会保留已生成的正文，在消息末尾追加一次提示，并继续当前回合的模型与工具循环。后续调用仍可查资料、维护状态和记忆；有效的 `offer_choices` 调用会显示选项并结束回合。已经给出选项时不增加调用。

每回合最多追加一次选项提示，补调用计入本回合的调用数、用量与费用，并受相同的预算和停止机制约束。模型再次自然结束却仍未给出选项时，正文以中断状态保存，阅读页提示可以继续。补调用出错或被停止时同样保留正文。

生成式开场适用此开关；静态开场使用开场文件中的 `choices`，并将选项记录为助手历史中的 `offer_choices` 调用，玩家首轮作答回填为工具结果。只有场外回应、没有故事正文时不触发补调用。拒答、出错、截断续写达到上限等停止原因也不触发；正常的截断续写在正文完成后再检查选项。

补调用使用普通的追加消息，不要求供应商支持强制工具选择参数；工具定义和已有消息保持不变。模型仍需具备可用的工具调用能力，平台不会把正文中的工具文本转换成选项。

系统提示词同时提供原生工具调用规则与逐项自然语言用法，内容来自实际发送的工具定义。选项按钮由平台根据 `offer_choices` 的 `choices` 参数生成；正文中的文字选项列表、代码块或 HTML 不算工具调用，开启补调用时仍会检查是否缺少有效选项。

玩家也可以在当前正文没有选项时使用 `/choices` 或「生成选项」按钮。这是一次只读的选项生成操作，与 `force_choices` 独立，沿用当前正文、状态和记忆。

## prompts/

- `narrator.md`：自由书写的 Markdown，整篇放进系统提示词的"执笔指令"一节，标题层级会整体下移。可用 Liquid 引用 `state`，但始终以存档创建时的**初始状态**渲染，之后的变化不会反映在这里。`[[id]]` 在这里不会补全。建议 2,000 字以内。
- `memory.md`：附在平台默认记忆约定之后（"记忆约定"一节），写这部作品特别值得记什么、怎样组织；写明替换默认目录约定时以作品为准。
- `compaction.md`：上下文整理时附在整理指令末尾，写交接摘要需要特别保留的内容。

## lore/ 设定文档

frontmatter：

| 字段 | 说明 |
| --- | --- |
| `id` | 默认取文件名 |
| `title` | 默认取文件名 |
| `kind` | 分类，决定设定索引的分组：`world` 世界、`outline` 主线、`character` 人物、`place` 地点、`faction` 势力、`item` 物品；其他取值原样作为组名，中文作品的自定义分类直接写中文（如 `kind: 案情`） |
| `aliases` | 别名，进入索引与检索 |
| `summary` | 一行摘要（建议 ≤ 40 字）：写清它是什么、什么时候需要读 |
| `load` | `always` \| `index` \| `none`，默认取 `docs.default_load` |
| `images` | 关联图片（素材 ID 或 `assets/` 路径）；执笔者读到本文档时会看到这些图片的路径与说明 |
| `pin_images` | 关联图片是否常驻上下文，默认 `false` |
| `display.color` | 人物名字颜色（`<say>` 说话人样式） |

| `load` | 效果 | 适用 |
| --- | --- | --- |
| `always` | 正文每回合都在上下文中（不再出现在索引里） | 每回合都需要的少量核心设定，合计建议 ≤ 5,000 字 |
| `index` | 路径、标题、别名与摘要进入设定索引，正文由执笔者按需读取 | 大多数设定 |
| `none` | 不进入索引，执笔者可通过检索、链接、目录摘要找到 | 大量细碎条目、长篇资料 |

- 加载方式只决定上下文中放什么，执笔者可以读到作品中的全部文档。希望延后揭示的内容，在文档里用自然语言写明揭示时机。
- `_index.md`：目录摘要，frontmatter 的 `summary` 作为整个目录在索引中的一行；配合子文档 `load: none`，索引只显示目录级摘要。
- 正文中的 `[[id]]` 在执笔者用 read 读取该文档时补全为 `[[id]](/work/lore/…)`；目标必须是存在的文档 ID，否则校验报 `E-REF`。`always` 文档的正文直接进入上下文，其中的 `[[id]]` 不会补全。
- `always` 文档开局就在执笔者的上下文里：执笔指令按标题提及即可，不必让执笔者去读；它的 `summary` 不出现在设定索引中，只在执笔者 list 目录时作为一行说明。
- `kind: character` 或写了 `display` 的文档登记为人物：`<say who>` 可以写它的 ID、标题或别名。
- 索引建议 ≤ 150 条或 6k token；更多时用目录摘要与 `load: none`。

## state.yaml

```yaml
fields:
  <字段>:
    type: boolean | number | string | list | map | object
    default: <值>            # 缺省：false / 0 / "" / [] / {}
    desc: <含义、范围、更新时机>
    items: <类型或字段定义>   # list 的元素，缺省 string
    values: <类型或字段定义>  # map 的值（键为字符串，可以是中文，不能含 .），缺省 string
    fields: {<子字段>}        # object
```

- 简写：`items: number`；一行写法：`<字段>: {type: number, default: 1, desc: …}`。
- 每个非 object 字段都写 `desc`（缺少时 `W-NO-DESC`）。`desc` 原样出现在执笔者的"状态说明"中，是它维护状态的唯一依据。
- 执笔者用 `update_state` 每次改一项：`set`、`inc`（数字加减）、`append` / `remove`（列表）、`delete`（删除 map 的键）；路径如 `<object>.<子字段>`、`<map>.<键>`。
- 平台只检查类型，不判断剧情能否发生；范围与含义写进 `desc` 由执笔者把握。
- 没有 `state.yaml` 时，状态说明与状态工具都不会出现；不必预留空字段。

## commands/ 命令

文件 `commands/<命令名>.md`，frontmatter：

| 字段 | 说明 |
| --- | --- |
| `title` | 显示名，默认取命令名 |
| `description` | 给玩家和执笔者看的说明（缺少时 `W-NO-DESC`） |
| `args` | 参数：`[{name, hint?, options?, required?}]`，参数都是文本，`options` 为候选项 |
| `present` | `slash`（输入 `/命令`）、`mode`（输入栏模式）、`button`（命令栏 / 面板按钮），默认 `[slash]` |
| `mode_label` / `mode_style` | 作为输入模式时的标签与样式标签名 |
| `icon` | 图标名（见"面板"一节） |
| `confirm` | 执行前的确认提示 |
| `ai_visible` | 是否列入执笔者的命令表，默认 `true` |

- 正文是 Liquid 模板：`args.<参数名>`、`input`（玩家附带的文字）、`state`（当前状态）、`turn.no`（即将开始的回合号）。展开结果作为玩家本回合的输入发给执笔者；正文为空时直接用 `input`。
- `/命令 参数` 按空格依次填入参数（带空格的值用引号，也可写 `参数名=值`），多出的文字归入 `input`。自由长文本用 `input` 接收更自然。
- `mode`：玩家切换到该模式后输入的文字作为 `input`；需要 `features.free_input: true`。
- 命令栏：`ui/layout.yaml` 的 `command_bar: [<命令名>]`，命令的 `present` 需含 `button`；有必填参数的按钮会先把 `/命令 ` 填进输入框。
- 平台内置 `/ooc`、`/continue`、`/compact`、`/help`、`/rewrite`、`/choices`，作品不必实现。

## 叙事标记（开场正文与执笔者的正文）

- 每个非空行是一个段落。`*强调*`、`**加重**`；整行只有 3 个以上 `-` 或 `*` 为场景分隔；`>` 开头为引用。
- 没有标题、列表、表格与代码块：`#`、`- ` 开头的行按原文显示。
- 反斜杠转义 `\\ \* \< \> \-` 得到字符本身，如 `\---` 显示一行短横线。中文破折号 `——` 不受影响。
- 标签写在一行之内：`<name attr="值">内容</name>` 或 `<name attr="值"/>`，属性值用双引号；忘写结束标签时到段末自动结束；`<br/>` 是段内换行。
- 未声明或未启用的标签按原文显示（尖括号可见），开场里只用作品已启用的标签。

内置标签（按作品情况自动启用，未启用的不出现在执笔者的提示词里）：

| 标签 | 用途 | 启用条件 |
| --- | --- | --- |
| `<scene bg="素材ID" title="" transition="fade\|cut\|blur"/>` | 切换舞台背景，`title` 显示标题卡；写在段首 | `presentation.stage` 且有 `role: background` 的图片 |
| `<cg src="素材ID" caption=""/>` | 正文插图卡片，可点开大图；写在段首 | 有非背景、非封面的图片素材 |
| `<say who="人物">台词</say>` | 带说话人名字与颜色的台词 | `presentation.dialogue: tagged` |
| `<bgm src="素材ID"/>`、`<bgm stop="true"/>`、`<sfx src="素材ID"/>` | 音乐与音效（可带 `volume="0–1"`） | 有音频素材 |
| `<pause ms="800"/>`、`<fx type="shake\|glitch\|fade\|whisper\|flicker\|blur-reveal">…</fx>`、`<aside>…</aside>` | 停顿、文字特效、旁白 | `presentation.text_effects` |
| `<act cmd="/命令 参数">文字</act>` | 可点击词，点击执行作品命令 | 有执笔者可见的命令 |
| `<ooc>…</ooc>` | 执笔者的场外回应，浅色块显示，不计入故事原文 | `features.ooc` |

## ui/markup.yaml 样式标签

```yaml
tags:
  <标签名>:
    description: <给执笔者：它表示什么、唯一的使用场合、写出时要同步做什么>
    style:
      color: "#rrggbb"
      dark_color: "#rrggbb"   # 缺省时自动计算对比度足够的深色模式颜色
      background: "#rrggbb"
      weight: normal | bold
      italic: true | false
      underline: none | solid | dotted | wavy
      size: 0.8–1.6           # 相对字号
      font: body | heading | mono | <字体素材>
      letter_spacing: 0–0.5   # em
      opacity: 0.3–1
      effect: glow | shake | flicker | fade-in | blur-reveal | whisper | glitch
    icon: <图标名>             # 非颜色提示（无障碍）
    class: <custom.css 中的类名>
    tooltip: <悬停提示，可用 {{ text }}>
```

- `description` 原样出现在执笔者提示词的"叙事标记"一节，格式为 `- <标签>…</标签>：说明`。
- 样式标签同时是选项可用的 `style` 与命令 `mode_style` 可引用的样式。
- 特效受玩家"减少动画"设置约束。`effect` 的取值与内置 `<fx type>` 不同。

## ui/layout.yaml 与面板

```yaml
panels:
  - id: <ID>
    title: <标题>            # 侧栏与抽屉中显示；顶栏、悬浮面板不显示标题
    place: topbar | sidebar | drawer | float
    template: ui/panels/<文件>.liquid
    format: markdown | html  # 默认 markdown
    order: <整数>
    collapsed: false
command_bar: [<命令名>]
```

| 位置 | 宽屏 | 窄屏（< 1200 px）与手机 |
| --- | --- | --- |
| `topbar` | 正文上方居中的一条胶囊，单行，超出截断 | 同左 |
| `sidebar` | 右侧栏 | 成为抽屉中的一个标签页（手机从底部拉出，最高约 3/4 屏，内容可滚动） |
| `drawer` | 抽屉中的标签页 | 同左 |
| `float` | 角落悬浮小卡 | 手机上不显示 |

- 模板变量：`state`、`turn.no`（当前回合）、`work.title`、`save.title`。渲染结果为空（只有空白）时面板不显示。
- `markdown` 格式支持 Markdown 与内嵌 HTML；`html` 格式原样作为 HTML。
- 内置组件（写成成对标签）：`<ui-stat label icon value>`（一行：图标、标签、右对齐的值）、`<ui-bar label value max>`（进度条，标签宽约 4 个汉字）、`<ui-badge icon>文字</ui-badge>`、`<ui-button command args confirm>文字</ui-button>`、`<ui-collapse title open>…</ui-collapse>`、`<ui-tooltip text>文字</ui-tooltip>`、`<ui-image src alt size="sm|md|lg"></ui-image>`。
- HTML 会被净化：没有脚本、事件属性、iframe、表单、外链；`style` 只保留颜色、字号字重、边距、边框、对齐、宽高等排版属性；图片只能引用作品素材（`<img src="{{ '<素材ID>' | asset }}">` 或 `<ui-image src="<素材ID>">`）。
- 图标用 lucide 名称。内置常用子集：anchor backpack book calendar castle cloud-fog coins compass crown dices eye feather flame footprints gem ghost heart hourglass info key lightbulb lock map message-circle moon music quote scroll search shield ship skull sparkles stamp star sun sword triangle-alert user users zap；其他名称首次使用时按需加载完整图标集。

## Liquid 模板

| 用途 | 变量 |
| --- | --- |
| 执笔指令、开场 | `state`（初始状态） |
| 命令正文 | `args`、`input`、`state`、`turn` |
| 面板 | `state`、`turn`、`work`、`save` |

- 标签：`if/elsif/else`、`unless`、`case/when`、`for`、`assign`、`capture`、`comment`、`raw`、`render`；不支持 `include`。`render` 只能引用 `ui/` 下的 `.liquid`（如 `{% render 'panels/_row', label: x %}`），被引用的模板只看到传给它的变量。
- 遍历 map 得到按键排序的 `[键, 值]`：`{% for p in state.<map> %}{{ p[0] }}：{{ p[1] }}{% endfor %}`。键按 Unicode 码点排序，中文键不按拼音或笔画，看起来像乱序。需要固定顺序时列出键名，按名取值，再把列表外的键补在末尾：

  ```liquid
  {%- assign order = "<键1>,<键2>,<键3>" | split: "," -%}
  {%- for k in order -%}{%- if state.<map>[k] != nil -%}…{{ k }} {{ state.<map>[k] }}…{%- endif -%}{%- endfor -%}
  {%- for p in state.<map> -%}{%- unless order contains p[0] -%}…{{ p[0] }} {{ p[1] }}…{%- endunless -%}{%- endfor -%}
  ```
- `for` 的参数：`limit: N`、`offset: N`（只接受整数字面量，写成计算结果会报错）、`reversed`。最近 N 条、新的在前：`{% for c in state.<list> reversed limit: 8 %}`。
- 过滤器：标准过滤器（`size` `default` `plus` `minus` `times` `divided_by` `round` `join` `first` `last` `reverse` `slice` `sort` `map` `where` `upcase` `downcase` `truncate` `escape` 等）及 `asset`（素材引用 → 图片地址）、`percent`、`markdown`。
- 不存在的变量与字段渲染为空字符串，不报错。过滤器名写错时整个模板渲染失败：执笔指令失败则每个回合都无法开始，静态开场失败则无法新建存档，面板失败则该面板显示错误提示。
- `{%-`、`-%}`、`{{-`、`-}}` 去掉标签两侧的空白与换行，单行面板靠它保持紧凑。
- 源码与输出各 ≤ 64 KiB，单次渲染超过 1 秒中止。

## ui/theme.yaml 与 custom.css

```yaml
fonts:
  body: serif | sans | <woff2 字体素材>
  heading: serif | sans | <woff2 字体素材>   # 场景标题卡
colors:
  light: {background: "#rrggbb", text: "#rrggbb", muted: "#rrggbb", accent: "#rrggbb", panel: "#rrggbb"}
  dark:  {background: "#rrggbb", text: "#rrggbb", muted: "#rrggbb", accent: "#rrggbb", panel: "#rrggbb"}
reading:
  max_width: 40em          # CSS 长度：em rem px ch 或 0
  line_height: 1.9         # 1.2–2.6
  paragraph_spacing: 0.9em
  indent: 2em              # 中文首行缩进；0 关闭
background:
  image: <素材>
  opacity: 0.1             # 0–0.5
custom_css: ui/custom.css
```

- 主题只作用于阅读页；只给一套配色时另一模式用平台默认。玩家可以关闭作品主题，玩家调整过的阅读设置优先。
- 封面与主题背景图只给界面用，不出现在执笔者的素材索引中；同一张图也要给执笔者用（`<scene>`、`<cg>`、查看）时，为界面另存一份文件，否则报 `W-UI-ASSET`。
- `custom.css` 作用域限于阅读页；可用 `url(asset:<素材ID>)` 引用素材；不支持 `@import`、外部 `url()`、`:root`、反斜杠转义、`image-set()`；顶层 at 规则只支持 `@media` `@supports` `@container` `@layer` `@font-face` `@keyframes`。常用类：`.mk-<标签名>`（样式标签）、`.scene-title`（场景标题卡）、`.cg-card`（插图卡片）。
- 中文字体体积大，使用前子集化并确认许可。

## 素材

| 类型 | 格式 | 建议 |
| --- | --- | --- |
| 图片 | png、jpg、webp、gif（取首帧给执笔者） | 长边 ≤ 2048 px，背景图用 webp |
| 音频 | mp3、ogg、m4a | BGM 可循环，单文件 ≤ 10 MB |
| 字体 | woff2 | 子集化，确认许可 |

只有 `assets/` 下的文件是素材。`assets.yaml`（可选）补充元数据：

```yaml
- path: assets/<路径>
  id: <素材ID>              # 省略时取文件名
  description: <给执笔者：画面里有什么、什么时候用>
  role: background | cg | avatar | bgm | sfx | font | reference
  pin: false                # 常驻上下文（模型能看图时每回合发送）
```

- 未列出的文件也可使用；图片默认 `role: reference`，音频默认 `bgm`。
- 背景图必须写 `role: background`，才会出现在 `<scene bg>` 可用的背景列表中；背景图只用于 `<scene>`。其他图片可用于 `<cg>`，也可供执笔者 read 查看；`cg`、`avatar`、`reference` 在运行时效果相同，按用途填写即可。
- `description` 进入执笔者的素材索引，是它决定何时看图、何时插图、何时换背景的依据；模型不能看图时，这是它对画面的全部了解。写画面里有什么、适合什么时候用；使用频率等约定写进执笔指令，不写使用记录（"已经用过"之类开场后就不再成立）。
- 常驻图片（`pin: true` 或文档 `pin_images: true`）建议 ≤ 3 张、长边 ≤ 1568 px（`W-PIN-IMAGE`）。
- 素材不要用 Git LFS（`E-LFS`）。

## ID 与路径

| 位置 | 写法 |
| --- | --- |
| `work.yaml` 的 `cover`；`theme.yaml` 的 `background.image`、字体；文档的 `images`；`assets.yaml` 的 `path` | 素材 ID 或 `assets/…` 相对路径（不写 `/work/`） |
| `openings[].file`、`layout.yaml` 的 `template`、`theme.yaml` 的 `custom_css`、`narrator.instructions` | 相对作品根的文件路径 |
| `{% render %}` | `ui/` 下的模板，可省略 `ui/` 与 `.liquid` |
| 面板 `<ui-image src>`、`{{ '<ID>' \| asset }}`、`custom.css` 的 `url(asset:<ID>)` | 素材 ID |
| 正文 `<scene bg>`、`<cg src>`、`<bgm src>`、`<sfx src>` | 素材 ID（执笔者在素材索引中看到的 ID） |
| `<say who>` | 人物文档的 ID、标题或别名 |
| `<act cmd>`、选项（含静态开场选项）的 `command` | 以 `/` 开头的完整命令：`/命令 参数` |
| `<ui-button command args>` | 命令名（可带 `/`），参数另写在 `args` |
| `layout.yaml` 的 `command_bar` | 不带 `/` 的命令名 |
| 设定文档正文中的 `[[id]]` | 文档 ID；执笔者读取该文档时补全为路径 |
| 执笔指令、`always` 文档、命令模板、开场指引中希望执笔者去读的文件 | 完整路径 `/work/lore/…`、`/work/assets/…`（这些地方的 `[[id]]` 不会补全） |
| 状态路径（`update_state`、`initial_state`、`{{ state.… }}`） | 点分路径；map 的键可以出现在 `initial_state` 与 `update_state` 中，开局表单的 `key` 不能指向 map 的键 |
| 命令的 `mode_style`、选项的 `style` | 样式标签名 |

## 执笔者看到的提示词

系统提示词 = 平台核心（按功能裁剪）+ `# 作品：<标题>` 下的各节：

- `## 执笔指令`：`narrator.md` 渲染后的全文
- `## 叙事标记`：已启用的内置标签用法 + 每个样式标签的 `description`
- `## 命令`：执笔者可见的命令签名与 `description`、输入模式
- `## 状态说明`：每个字段的路径、类型与 `desc`
- `## 设定索引`：`index` 文档按 `kind` 分组列出路径、标题、别名与摘要
- `## 素材索引`：背景 ID、图片 ID 与路径、音频 ID，各带 `description`
- `## 记忆约定`：`memory.md`

首条消息带 `always` 文档全文、常驻图片、开局状态与初始记忆目录。之后每回合，玩家输入在 `<input>` 中（命令已展开为文字，输入模式带 `mode`），系统注记在 `<notes>` 中，末尾附 `<pace>` 篇幅。`aint work prompt` 可以看到全部内容。

## 校验覆盖

`aint work validate` 检查：清单、各 YAML 与 frontmatter 的结构、取值和未知字段、命名与重名、文件 / 文档 / 素材 / 模板引用、设定文档的 `[[id]]`、开局表单与 `initial_state` 的状态路径和类型、静态开场选项的 `command`、`command_bar`、Liquid 语法与 `render` 引用、主题与 `custom.css`、素材能否解码，以及封面或主题背景图是否同时被故事引用（`W-UI-ASSET`）。

不检查，需要自查：

- 过滤器名是否存在；`{{ state.… }}` 的字段名是否拼对（不存在时渲染为空）；
- 开场正文里的标签是否已声明或已启用（未启用的标签按原文显示）；
- 执笔指令、`always` 文档、命令模板中写的 `/work/…` 路径是否存在；
- 静态开场、命令模板与面板的渲染结果与显示效果。

`aint work prompt` 以初始状态渲染执笔指令（出错直接报错），并附上生成式开场的指引；静态开场正文、命令模板与面板不经过它。拿不准的 Liquid 写法，可以复制一份作品，把片段临时写进副本的执笔指令，用 `prompt` 看渲染结果，看完删掉副本。执笔指令里只有 `state`（所选开场的初始状态）：试命令模板时先用 `{% assign input = "…" %}` 模拟玩家附带的文字，`args.<参数>` 换成字面值；试面板时 `turn.no` 等同样换成字面值；需要别的状态值时，在副本开场的 `initial_state` 里设好。

## 校验代码

| 代码 | 含义 |
| --- | --- |
| `E-MANIFEST` | `work.yaml` 缺少必需字段或取值不合法（含开场、面板位置） |
| `E-PARSE` | YAML 或 frontmatter 语法错误 |
| `E-ENGINE` | `engine` 范围不合法或程序不满足 |
| `E-REF` | 引用了不存在的文件、文档、素材、模板、命令或状态字段 |
| `E-TEMPLATE` | Liquid 解析失败或用了 `include` |
| `E-NAME` | 命名不合法、重名或与保留名冲突；记忆模板不合规 |
| `E-STATE` | 状态定义的类型或默认值不合法 |
| `E-ASSET` | 素材无法解码或与扩展名不符 |
| `E-THEME` | 主题取值或 `custom.css` 写法不支持 |
| `E-LFS` | Git LFS 指针文件 |
| `W-NO-DESC` | 命令、状态字段或样式标签缺少说明 |
| `W-UNKNOWN-FIELD` | 未知字段（多为拼写错误） |
| `W-UI-ASSET` | 封面或主题背景图同时标为 `role: background` / `cg`、或被设定文档的 `images` 引用；它不会进入执笔者的素材索引 |
| `W-ENCODING` | 带 BOM 或使用其他文本编码 |
| `W-PREFIX-LARGE` / `W-INDEX-LARGE` / `W-PIN-IMAGE` | 常驻部分、设定索引或常驻图片过大 |
| `I-TOKENS` | 常驻部分各层的 token 估算 |

## token 参考

执笔指令 ≤ 2,000 字；`always` 文档合计 ≤ 5,000 字；设定索引 ≤ 150 条；常驻图片 2–3 张；命令 ≤ 30 个。常驻部分会被缓存，但规模直接影响每回合的成本。
