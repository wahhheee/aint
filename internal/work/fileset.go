package work

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/store"
)

// Limits 是读取作品文件时的安全限制（《02-技术方案》5.1，默认值可配置）。
type Limits struct {
	MaxTotal    int64 // 总量（解压后）
	MaxFiles    int   // 文件数（不含被忽略的文件）
	MaxFileSize int64 // 单文件
	MaxDepth    int   // 路径深度
}

// DefaultLimits 是默认安全限制。
var DefaultLimits = Limits{MaxTotal: 512 << 20, MaxFiles: 5000, MaxFileSize: 64 << 20, MaxDepth: 16}

// maxEntries 是来源中允许出现的条目总数（含将被忽略的 .git/ 等）。
// 在读取任何内容之前拒绝条目数异常的来源（如中央目录塞满空条目的 zip 炸弹），同时给被忽略的文件留出余量。
func (l Limits) maxEntries() int { return l.MaxFiles * 10 }

// UploadLimit 是上传请求体的上限：解压后总量加上 zip 头与表单分隔的余量。
func (l Limits) UploadLimit() int64 { return l.MaxTotal + 16<<20 }

// rejectf 是作品包本身不可接受（路径不安全、超过限制、找不到作品根……）时的错误，面向玩家。
func rejectf(format string, args ...any) *errs.Error {
	return errs.New(errs.CodeWorkInvalid, format, args...)
}

// FileSet 是作品文件的规范化集合：路径相对作品根、'/' 分隔、NFC 规范化、已排除忽略文件。
// 所有导入来源都先转成 FileSet，之后的编译、哈希、入库流程完全一致。
type FileSet struct {
	files map[string][]byte
}

// NewFileSet 创建空集合。
func NewFileSet() *FileSet { return &FileSet{files: map[string][]byte{}} }

// Add 添加文件。路径不合法，或规范化后与已有文件重名时返回错误。
func (f *FileSet) Add(p string, data []byte) error {
	clean, err := normPath(p)
	if err != nil {
		return err
	}
	if _, dup := f.files[clean]; dup {
		return rejectf("文件路径重复：%s", clean)
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

// Len 返回文件数。
func (f *FileSet) Len() int { return len(f.files) }

// Size 返回全部文件的总字节数。
func (f *FileSet) Size() int64 {
	var n int64
	for _, b := range f.files {
		n += int64(len(b))
	}
	return n
}

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

// ---------------------------------------------------------------------------
// 来源 → FileSet 的公共流程

// srcFile 是导入来源中的一个文件：尚未定位作品根、尚未应用忽略规则，内容在通过全部检查后才读取。
type srcFile struct {
	path string      // 来源中的原始相对路径
	size int64       // 声明的大小（zip 头 / 文件信息 / 上传时的实际大小）；读取时不得超过
	mode fs.FileMode // 类型位；只接受普通文件
	open func() (io.ReadCloser, error)
}

// collect 是所有来源（目录、zip、浏览器上传的文件夹）的公共前半段：
//
//	路径规范化与冲突检查 → 定位作品根 → 忽略规则 → 按声明大小检查限制 → 读取内容
//
// 限制在读取任何内容之前就按声明大小检查完毕；读取时再以声明大小为上限，实际内容不会超过检查过的量。
func collect(files []srcFile, lim Limits) (*FileSet, error) {
	if len(files) > lim.maxEntries() {
		return nil, rejectf("文件数超过上限（%d）", lim.MaxFiles)
	}
	byPath := make(map[string]*srcFile, len(files))
	for i := range files {
		f := &files[i]
		clean, err := normPath(f.path)
		if err != nil {
			return nil, err
		}
		if _, dup := byPath[clean]; dup {
			return nil, rejectf("有两个文件的路径相同：%s（名称可能只是 Unicode 写法不同，常见于在 macOS 与其他系统之间复制过的文件），请删除其中一个", clean)
		}
		f.path = clean
		byPath[clean] = f
	}
	for p := range byPath {
		for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
			if _, ok := byPath[dir]; ok {
				return nil, rejectf("路径冲突：%s 既是文件又是目录", dir)
			}
		}
	}

	root, err := locateRoot(byPath)
	if err != nil {
		return nil, err
	}
	ign, err := readIgnore(byPath[path.Join(root, IgnoreFile)])
	if err != nil {
		return nil, err
	}

	// 过滤与限制：只看声明大小，不读内容
	type kept struct {
		rel string
		f   *srcFile
	}
	var keep []kept
	var total int64
	for i := range files {
		f := &files[i]
		rel := f.path
		if root != "." {
			var ok bool
			if rel, ok = strings.CutPrefix(f.path, root+"/"); !ok {
				continue // 作品根之外的文件（如 zip 根目录的说明文件）
			}
		}
		if alwaysIgnoredPath(rel) || ign.Ignored(rel) {
			continue
		}
		if strings.Count(rel, "/")+1 > lim.MaxDepth {
			return nil, rejectf("路径层级过深（上限 %d 层）：%s", lim.MaxDepth, rel)
		}
		if !f.mode.IsRegular() {
			return nil, rejectf("不支持符号链接或特殊文件：%s", rel)
		}
		if f.size > lim.MaxFileSize {
			return nil, rejectf("文件过大（上限 %d MiB）：%s", lim.MaxFileSize>>20, rel)
		}
		if len(keep) >= lim.MaxFiles {
			return nil, rejectf("文件数超过上限（%d）", lim.MaxFiles)
		}
		if total += f.size; total > lim.MaxTotal {
			return nil, rejectf("作品总体积超过上限（%d MiB）", lim.MaxTotal>>20)
		}
		keep = append(keep, kept{rel, f})
	}

	set := NewFileSet()
	for _, k := range keep {
		data, err := readSrc(k.f)
		if err != nil {
			return nil, err
		}
		if err := set.Add(k.rel, data); err != nil {
			return nil, err
		}
	}
	return set, nil
}

// readSrc 读取文件内容，最多读到声明大小；实际内容更多说明来源有问题（zip 头与内容不符，或读取时文件被改动）。
func readSrc(f *srcFile) ([]byte, error) {
	rc, err := f.open()
	if err != nil {
		return nil, readError(f.path, err)
	}
	defer func() { _ = rc.Close() }() // 只读：关闭错误不影响已读到的内容
	data, err := io.ReadAll(io.LimitReader(rc, f.size+1))
	if err != nil {
		return nil, readError(f.path, err)
	}
	if int64(len(data)) > f.size {
		return nil, rejectf("文件大小与记录不符：%s", f.path)
	}
	return data, nil
}

func readError(p string, err error) error {
	var be *errs.Error
	if errors.As(err, &be) {
		return err
	}
	return errs.Wrap(err, errs.CodeWorkInvalid, "读取文件失败：%s（文件可能已损坏）", p)
}

// readIgnore 读取作品根的 .yanyiignore；不存在时返回空规则。
func readIgnore(f *srcFile) (*Ignore, error) {
	if f == nil || !f.mode.IsRegular() {
		return nil, nil
	}
	if f.size > maxIgnoreFileSize {
		return nil, rejectf("%s 过大（上限 %d KiB）", IgnoreFile, maxIgnoreFileSize>>10)
	}
	data, err := readSrc(f)
	if err != nil {
		return nil, err
	}
	return ParseIgnore(data), nil
}

// locateRoot 定位作品根：根目录有 work.yaml，或只有一个顶层目录且其中有 work.yaml。
// 顶层的普通文件与始终忽略的目录（如 __MACOSX）不影响判断。返回 "." 或顶层目录名。
func locateRoot(byPath map[string]*srcFile) (string, error) {
	if _, ok := byPath["work.yaml"]; ok {
		return ".", nil
	}
	tops := map[string]bool{}
	for p := range byPath {
		if top, _, ok := strings.Cut(p, "/"); ok && !slices.Contains(alwaysIgnored, top) {
			tops[top] = true
		}
	}
	if len(tops) == 1 {
		for top := range tops {
			if _, ok := byPath[top+"/work.yaml"]; ok {
				return top, nil
			}
		}
	}
	return "", rejectf("没有找到 work.yaml：作品根目录，或唯一的顶层文件夹中需要有 work.yaml")
}

// FromFS 从任意 fs.FS（本机目录、embed）读取作品：定位作品根、应用忽略规则与安全限制。
// fsys 的根可以就是作品根，也可以只含一个作品文件夹。
func FromFS(fsys fs.FS, lim Limits) (*FileSet, error) {
	var files []srcFile
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if slices.Contains(alwaysIgnored, d.Name()) {
			if d.IsDir() {
				return fs.SkipDir // 不遍历 .git/、node_modules/：可能有成千上万个文件
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if len(files) >= lim.maxEntries() {
			return rejectf("文件数超过上限（%d）", lim.MaxFiles)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, srcFile{
			path: p, size: info.Size(), mode: d.Type(),
			open: func() (io.ReadCloser, error) { return fsys.Open(p) },
		})
		return nil
	})
	if err != nil {
		var be *errs.Error
		if errors.As(err, &be) {
			return nil, err
		}
		return nil, fmt.Errorf("读取作品目录失败: %w", err)
	}
	return collect(files, lim)
}

// normPath 规范化路径：'\' 视为分隔符、去掉多余的 "/" 与 "."、NFC 规范化；
// 拒绝绝对路径、盘符、".."、控制字符与非 UTF-8 名称。
func normPath(p string) (string, error) {
	if !utf8.ValidString(p) {
		return "", rejectf("文件名编码无法识别（需要 UTF-8）：%q", p)
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" || strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':') {
		return "", rejectf("不允许绝对路径：%s", p)
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return "", rejectf("文件名中有控制字符：%q", p)
		}
	}
	for seg := range strings.SplitSeq(p, "/") {
		if seg == ".." {
			return "", rejectf("路径中不允许 ..：%s", p)
		}
	}
	clean := path.Clean(p)
	if clean == "." {
		return "", rejectf("路径为空")
	}
	return norm.NFC.String(clean), nil
}
