package work

import (
	"bytes"
	"io"
	"strings"

	"github.com/wahhheee/aint/internal/pkg/errs"
)

// FolderUpload 收集浏览器上传的文件夹（<input webkitdirectory> 或拖入的文件夹：多个文件，各带相对路径）。
//
// HTTP 层按顺序对每个文件调用 Add，内容暂存在内存中，总量受 Limits 约束；全部收到后由 FileSet
// 走与目录、zip 相同的流程（collect）。始终忽略的路径（.git/ 等）不读取；超过单文件上限的文件只记大小、
// 不保留内容——它若被 .aintignore 忽略则无妨，否则在 collect 中报"文件过大"。
type FolderUpload struct {
	lim   Limits
	files []srcFile
	total int64
}

// NewFolderUpload 创建文件夹上传。
func NewFolderUpload(lim Limits) *FolderUpload { return &FolderUpload{lim: lim} }

// Add 加入一个文件。p 是相对所选文件夹的路径（含文件夹名本身，如 misty-harbor/work.yaml）。
func (u *FolderUpload) Add(p string, r io.Reader) error {
	clean, err := normPath(p)
	if err != nil {
		return err
	}
	if alwaysIgnoredPath(clean) {
		return nil
	}
	if len(u.files) >= u.lim.maxEntries() {
		return rejectf("文件数超过上限（%d）", u.lim.MaxFiles)
	}
	data, err := io.ReadAll(io.LimitReader(r, u.lim.MaxFileSize+1))
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidArgument, "接收上传的文件失败：%s", clean)
	}
	size := int64(len(data))
	if size > u.lim.MaxFileSize {
		n, err := io.Copy(io.Discard, r)
		if err != nil {
			return errs.Wrap(err, errs.CodeInvalidArgument, "接收上传的文件失败：%s", clean)
		}
		size, data = size+n, nil
	} else if u.total += size; u.total > u.lim.MaxTotal {
		return rejectf("作品总体积超过上限（%d MiB）", u.lim.MaxTotal>>20)
	}
	u.files = append(u.files, srcFile{
		path: clean, size: size,
		open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
	})
	return nil
}

// Folder 返回所选文件夹的名称（第一个文件路径的第一段），用作来源名称。
func (u *FolderUpload) Folder() string {
	if len(u.files) == 0 {
		return ""
	}
	top, _, _ := strings.Cut(u.files[0].path, "/")
	return top
}

// FileSet 定位作品根、应用忽略规则与限制，得到文件集。
func (u *FolderUpload) FileSet() (*FileSet, error) {
	if len(u.files) == 0 {
		return nil, errs.Invalid("没有收到任何文件")
	}
	return collect(u.files, u.lim)
}
