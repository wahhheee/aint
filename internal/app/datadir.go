package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wahhheee/aint/internal/pkg/fsx"
)

// Dirs 是数据目录的布局（《02-技术方案》4.1）。
//
//	aint-data/
//	├── aint.db           SQLite（WAL）：作品、存档、全部设置
//	├── blobs/            大于 64 KiB 的 blob
//	├── secrets.json      API Key 与访问密码哈希（永不导出）
//	├── backup/           结构迁移前的自动备份（最近 3 份）
//	├── cache/derived/    图片派生缓存（可随时删除）
//	├── tmp/              临时文件：imports/ 是导入暂存（启动时清空）
//	├── logs/             滚动日志；logs/llm-trace/ 为 --trace-llm 的调用记录
//	└── instance.json     运行中实例的信息（单实例检测）
type Dirs struct {
	Root     string `json:"root"`
	DB       string `json:"db"`
	Blobs    string `json:"blobs"`
	Secrets  string `json:"secrets"`
	Backup   string `json:"backup"`
	Cache    string `json:"cache"`
	Tmp      string `json:"tmp"`
	Logs     string `json:"logs"`
	Trace    string `json:"trace"`
	Instance string `json:"instance"`
}

// ResolveDirs 确定数据目录：默认在可执行文件旁的 aint-data/（决策 D-19），--data 可指定其他目录。
// 目录不可写时拒绝启动并说明原因，不静默改用其他位置。
func ResolveDirs(flagDir string) (Dirs, error) {
	root := flagDir
	if root == "" {
		exe, err := os.Executable()
		if err != nil {
			return Dirs{}, fmt.Errorf("无法定位可执行文件: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved // 经符号链接启动时，数据跟随真实文件
		}
		dir := filepath.Dir(exe)
		if strings.Contains(dir, "go-build") {
			return Dirs{}, fmt.Errorf("检测到通过 go run 启动（可执行文件位于临时目录 %s）：请用 --data 指定数据目录，例如 --data ./.devdata", dir)
		}
		root = filepath.Join(dir, "aint-data")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Dirs{}, err
	}
	if err := fsx.CheckWritable(root); err != nil {
		return Dirs{}, fmt.Errorf("%w\n请把程序放到可写的文件夹，或用 --data 指定数据目录", err)
	}
	d := Dirs{
		Root:     root,
		DB:       filepath.Join(root, "aint.db"),
		Blobs:    filepath.Join(root, "blobs"),
		Secrets:  filepath.Join(root, "secrets.json"),
		Backup:   filepath.Join(root, "backup"),
		Cache:    filepath.Join(root, "cache"),
		Tmp:      filepath.Join(root, "tmp"),
		Logs:     filepath.Join(root, "logs"),
		Trace:    filepath.Join(root, "logs", "llm-trace"),
		Instance: filepath.Join(root, "instance.json"),
	}
	for _, p := range []string{d.Blobs, d.Backup, d.Cache, d.Logs} {
		if err := os.MkdirAll(p, fsx.DirPerm); err != nil {
			return Dirs{}, err
		}
	}
	return d, nil
}
