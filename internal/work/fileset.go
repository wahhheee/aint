package work

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/wahhheee/yanyi/internal/store"
)

// Limits 是读取作品文件时的安全限制（《02-技术方案》5.1，默认值可配置）。
type Limits struct {
	MaxTotal    int64 // 总量
	MaxFiles    int   // 文件数
	MaxFileSize int64 // 单文件
	MaxDepth    int   // 路径深度
}

// DefaultLimits 是默认安全限制。
var DefaultLimits = Limits{MaxTotal: 512 << 20, MaxFiles: 5000, MaxFileSize: 64 << 20, MaxDepth: 16}

// alwaysIgnored 是始终忽略的路径段与文件名。
var alwaysIgnored = []string{".git", ".github", ".DS_Store", "Thumbs.db", "desktop.ini", "node_modules"}

// FileSet 是作品文件的规范化集合：路径相对作品根、'/' 分隔、已排除忽略文件。
// 所有导入来源都先转成 FileSet，之后的编译、哈希、入库流程完全一致。
type FileSet struct {
	files map[string][]byte
}

// NewFileSet 创建空集合。
func NewFileSet() *FileSet { return &FileSet{files: map[string][]byte{}} }

// Add 添加文件。路径不合法时返回错误。
//
// TODO(M1-4)：文件名 NFC 规范化后去重冲突报错（golang.org/x/text/unicode/norm）。
func (f *FileSet) Add(p string, data []byte) error {
	clean, err := cleanPath(p)
	if err != nil {
		return err
	}
	if _, dup := f.files[clean]; dup {
		return fmt.Errorf("文件路径重复：%s", clean)
	}
	f.files[clean] = data
	return nil
}

// Get 读取文件内容。
func (f *FileSet) Get(p string) ([]byte, bool) {
	b, ok := f.files[p]
	return b, ok
}

// Has 判断文件是否存在。
func (f *FileSet) Has(p string) bool { _, ok := f.files[p]; return ok }

// Paths 返回排序后的全部路径。
func (f *FileSet) Paths() []string {
	out := make([]string, 0, len(f.files))
	for p := range f.files {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// Glob 返回目录 dir 下（递归）扩展名为 ext 的文件路径，已排序。
func (f *FileSet) Glob(dir, ext string) []string {
	var out []string
	prefix := dir + "/"
	for _, p := range f.Paths() {
		if strings.HasPrefix(p, prefix) && strings.HasSuffix(p, ext) {
			out = append(out, p)
		}
	}
	return out
}

// TreeHash 计算文件树哈希：对排序后的每个文件生成 path + "\0" + blobhash + "\n"，整体 SHA-256。
// 同一作品 ID 下同一哈希即同一版本（导入去重、导出再导入一致性都依赖于此）。
func (f *FileSet) TreeHash() string {
	h := sha256.New()
	for _, p := range f.Paths() {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write([]byte(store.HashBytes(f.files[p])))
		h.Write([]byte{'\n'})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// FromFS 从任意 fs.FS（目录、zip、embed）读取作品文件，应用忽略规则与安全限制。
// fsys 的根应当就是作品根（含 work.yaml），可先用 LocateRoot 定位。
//
// TODO(M6)：读取作品根的 .yanyiignore（gitignore 语法）。
// TODO(M1-4)：Git LFS 指针文件检测（E-LFS）。
func FromFS(fsys fs.FS, lim Limits) (*FileSet, error) {
	set := NewFileSet()
	var total int64
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if slices.Contains(alwaysIgnored, d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if strings.Count(p, "/")+1 > lim.MaxDepth {
			return fmt.Errorf("路径层级过深（上限 %d）：%s", lim.MaxDepth, p)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("不支持符号链接或特殊文件：%s", p)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > lim.MaxFileSize {
			return fmt.Errorf("文件过大（上限 %d MiB）：%s", lim.MaxFileSize>>20, p)
		}
		if total += info.Size(); total > lim.MaxTotal {
			return fmt.Errorf("作品总体积超过上限（%d MiB）", lim.MaxTotal>>20)
		}
		if len(set.files) >= lim.MaxFiles {
			return fmt.Errorf("文件数超过上限（%d）", lim.MaxFiles)
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		return set.Add(p, data)
	})
	if err != nil {
		return nil, err
	}
	return set, nil
}

// LocateRoot 定位作品根：根目录有 work.yaml，或只有一个顶层目录且其中有 work.yaml。
func LocateRoot(fsys fs.FS) (string, error) {
	if _, err := fs.Stat(fsys, "work.yaml"); err == nil {
		return ".", nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !slices.Contains(alwaysIgnored, e.Name()) {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 1 {
		if _, err := fs.Stat(fsys, dirs[0]+"/work.yaml"); err == nil {
			return dirs[0], nil
		}
	}
	return "", fmt.Errorf("没有找到 work.yaml：作品根目录或唯一的顶层目录中需要有 work.yaml")
}

// cleanPath 规范化路径并拒绝绝对路径、".." 等不安全写法。
func cleanPath(p string) (string, error) {
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" || strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':') {
		return "", fmt.Errorf("不允许绝对路径：%s", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", fmt.Errorf("路径中不允许 ..：%s", p)
		}
	}
	clean := path.Clean(p)
	if clean == "." {
		return "", fmt.Errorf("路径为空")
	}
	return clean, nil
}
