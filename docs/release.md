# 发布构建

`.goreleaser.yaml` 定义 Windows、macOS、Linux 的 amd64 / arm64 六种产物，全部关闭 CGO。每份压缩包包含可执行文件、使用说明、作品 Schema 和两部示例 zip；`checksums.txt` 覆盖发布压缩包。

## 本地验证

使用 GoReleaser 2.19.0 和仓库规定的 Go / pnpm 版本。先运行 `task check` 和 `task build:web`，再运行 `goreleaser check` 与 `goreleaser release --snapshot --clean`。快照不会上传发布页，不需要模型密钥。构建产物位于 `dist/`，示例暂存于 `.release/`。Schema 变更后运行 `go run ./cmd/schema-gen`；测试会检查定义与已提交的文件一致。

验证六份包均有可执行文件、说明、10 个 Schema 与两部示例 zip，核对 `checksums.txt`。在本机解压后使用临时数据目录启动，确认静态页面、`version` 和示例导入可用。其他系统的实际运行与发布候选版验收属于 M6-11。

## GitHub Actions

`release.yml` 在推送 `v*` 标签时运行检查并创建 **草稿发布**。维护者检查产物、补齐发布说明和预览阶段数据变更提示后，再手动发布草稿。手动运行 workflow 只生成 snapshot 并上传 Actions artifact，不创建 Release。

发布流程需要仓库自带 `GITHUB_TOKEN` 的 contents 写权限，不读取 `.local/`、个人模型密钥或运行时数据。实际发布标签必须在约定的版本验收完成后创建。
