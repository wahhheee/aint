# 创作指南

作品由文本文件和素材组成，用普通编辑器与 Git 管理。`work.yaml` 描述作品信息和功能，执笔指令定义文风、玩家身份与故事约定，开场提供故事起点。完整字段与默认值见[格式参考](../skills/aint-work/references/format.md)。

## 创建第一部作品

发布包中的 `aint` 提供创作者命令。在源码仓库中可将下面的 `aint` 替换为 `go run ./cmd/aint`。

```bash
aint work init my-story
aint work validate my-story
aint work prompt my-story
```

`init` 接受新目录或空目录，生成最小作品。编辑 `work.yaml` 中的 `id`、`title`、`description`、`authors` 和 `license`，再写 `prompts/narrator.md`。作品 ID 建议使用 `作者.作品名`，公开后保持稳定。

最小清单如下，使用生成式开场：

```yaml
format: 1
engine: ">=2.0 <3.0"
id: local.my-story
version: 0.1.0
title: 山间旅店
description: 在山路尽头经营一间旅店，接待途经这里的旅人。
openings:
  - id: arrival
    title: 第一位客人
    mode: generate
```

在 `prompts/narrator.md` 中写清玩家是旅店主人、故事从什么时间开始、采用什么人称，以及人物和叙事的主要特点。平台负责工具用法、记忆维护和篇幅控制，作品指令着重描述本作的内容。

## 选择开场与组织设定

希望每次开局显示同一段正文时，编写静态开场，将清单中的开场设为 `mode: static` 并填写 `file: prompts/openings/arrival.md`。文件正文支持 Liquid，可用 `{{ state.player.name }}` 引用已经在 `state.yaml` 定义的初始状态。frontmatter 中的 `choices` 提供初始选项。

一部作品可以声明多个开场，每个开场有独立的介绍和初始状态覆盖。第一项为默认开场，首个玩家回合完成前可切换。字段、命令选项与覆盖顺序见[开场格式](../skills/aint-work/references/format.md#开场-openings)。

较长的资料放在 `lore/` 下，按人物、地点或情节组织 Markdown 文件。给每份资料写标题、摘要与阅读时机，模型通过索引按需读取；常驻资料使用 `load: always`。文档间的 `[[id]]` 链接用于关联资料。

## 按需要添加功能

| 需要 | 文件与说明 |
| --- | --- |
| 名字、背景等开局设置 | `new_game.fields`，对应 `state.yaml` 中的字段 |
| 记录时间、物品、关系等 | `state.yaml`，每个字段写明类型、默认值和更新时机 |
| 斜杠命令、按钮或输入模式 | `commands/*.md`，正文为发送给模型的输入模板 |
| 显示状态 | `ui/layout.yaml` 与 Liquid 面板模板 |
| 特殊文字样式 | `ui/markup.yaml`，说明标签含义和使用位置 |
| 颜色、字体、背景和版式 | `ui/theme.yaml`、`custom.css` 和素材 |
| 插图、音频或常驻图片 | `assets/` 与 `assets.yaml` |
| 正文结束后补选项 | `features.force_choices: true`，见[选项行为](../skills/aint-work/references/format.md#正文结束后交还选项) |

《第一个故事》展示最小结构；[《倒悬的街角》](../examples/works/inverted-stars/)包含多个开场、状态、面板和命令。测试作品[《雾港来信》](../testdata/works/misty-harbor/)提供图片、标签和掷骰等格式示例。

## 试玩与检查

1. 运行 `aint work validate <作品目录>`，按报告处理错误与警告。
2. 运行 `aint work prompt <作品目录> --opening <开场ID>`，检查模型收到的指令、资料索引与工具。多开场作品逐个检查。
3. 启动 AINT，在书架「导入作品 → 开发目录」填写本机作品路径。保存文件后自动校验；通过后更新作品，失败时保留上次有效内容并显示报告。
4. 新建存档检查开场、表单和面板，再试玩选项、自由输入与作品命令。阅读页「调试」可查看模型请求、工具、状态和记忆。

校验器检查结构、引用与模板语法。模板中的过滤器、状态字段、实际渲染，以及正文中的素材路径还需要在试玩时核对，见[校验覆盖](../skills/aint-work/references/format.md#校验覆盖)。编辑器补全配置见 [Schema 说明](../schemas/README.md)。

## 打包与更新

```bash
aint work pack my-story -o my-story.zip
```

输出路径放在作品目录之外，且使用尚不存在的文件名。打包会先校验，再按固定顺序生成 zip。玩家可以从书架导入 zip、文件夹、公开 GitHub 仓库或 HTTPS 下载链接。作品 README 面向玩家介绍故事、身份、命令和面板。

更新已分发的作品时，保持作品、开场、文档和状态字段的 ID 稳定，递增 `version` 并填写 `changelog`。新状态字段提供默认值。玩家在版本页导入新版本，并在存档菜单选择升级；升级会先整理旧进度，再迁移状态，旧分支保留。开发目录直接重载当前开发版本。

将编辑器配置、草稿和制作记录写入 `.aintignore`。作者署名与许可证写进清单，第三方素材的来源与许可随作品附上。

## 使用 AI 创作技能

仓库的 [skills/aint-work](../skills/aint-work/SKILL.md) 是完整的技能目录，包含 `SKILL.md` 与 `references/`。将整个目录复制到所用助手的技能目录，或让支持读取本地文件的助手读取 `SKILL.md`，然后描述作品题材、玩家身份、文风和希望使用的功能。

[写作说明](../skills/aint-work/references/writing.md)涵盖执笔指令、开场、设定与命令文本；[设计说明](../skills/aint-work/references/design.md)涵盖状态、面板、视角与素材。技能与创作者文档共用同一份格式参考。
