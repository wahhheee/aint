# 作品编辑器配置

这些 JSON Schema 覆盖 `work.yaml`、`state.yaml`、`assets.yaml`、`ui/layout.yaml`、`ui/markup.yaml`、`ui/theme.yaml`，以及设定、命令、开场和记忆的 Markdown frontmatter。Markdown 正文、Liquid、CSS、素材与跨文件引用使用 `aint work validate` 检查。

在 VS Code 安装 Red Hat YAML 扩展，将发布包的 `schemas/` 放在作品目录旁，在作品的 `.vscode/settings.json` 中配置：

```json
{
  "yaml.schemas": {
    "../schemas/work.schema.json": "work.yaml",
    "../schemas/state.schema.json": "state.yaml",
    "../schemas/assets.schema.json": "assets.yaml",
    "../schemas/layout.schema.json": "ui/layout.yaml",
    "../schemas/markup.schema.json": "ui/markup.yaml",
    "../schemas/theme.schema.json": "ui/theme.yaml"
  }
}
```

也可在 YAML 首行指定本地 Schema 路径：

```yaml
# yaml-language-server: $schema=../schemas/work.schema.json
format: 1
```

将 `.vscode/` 写进作品根的 `.aintignore`，避免编辑器配置进入分发包。

Markdown frontmatter 需要编辑器或扩展支持将开头 `---` 之间的 YAML 作为独立文档进行 Schema 校验；对应文件为 `lore-frontmatter.schema.json`、`command-frontmatter.schema.json`、`opening-frontmatter.schema.json`、`memory-frontmatter.schema.json`。普通 YAML 扩展不会自动识别 Markdown frontmatter，不要将整份 Markdown 文件强制当作 YAML。没有对应编辑器支持时运行 `aint work validate`，报告会给出文件名与行号。

Schema 允许未知字段；运行时校验器将未知配置作为警告。输入类型和已知枚举由 Schema 检查，默认值、资源引用与业务约束以运行时校验器为准。

维护命令：在仓库根运行 `go run ./cmd/schema-gen`。生成文件由测试与示例作品共同校验，随发布包离线提供。
