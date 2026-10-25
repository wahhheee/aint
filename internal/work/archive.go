package work

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
)

// zip 炸弹判定：解压后超过 bombMinSize、且压缩比超过 bombRatio 的条目视为异常。
// 正常的文本约 3–10 倍，图片音频几乎不可压缩；上千倍只有大段重复内容才做得到。
const (
	bombMinSize = 1 << 20
	bombRatio   = 100
)

// FromZip 读取 zip 作品包。
//
// 在读取任何内容之前依次检查：条目数（目录结束记录）、加密、压缩比（zip 炸弹）、路径安全（zip slip），
// 然后与目录走同一流程（collect）：定位作品根 → 忽略规则 → 按声明大小检查限制 → 读取。
// archive/zip 在读取时校验实际解压量不超过头中声明的大小，所以按声明大小做的限制检查是可靠的。
func FromZip(r io.ReaderAt, size int64, lim Limits) (*FileSet, error) {
	return fromZip(r, size, lim, nil)
}

// subdir 非空指针表示 GitHub 归档：移除归档顶层目录，只收集指定子目录。
func fromZip(r io.ReaderAt, size int64, lim Limits, subdir *string) (*FileSet, error) {
	if n, ok := zipEntryCount(r, size); ok && n > uint64(lim.maxEntries()) { //nolint:gosec // maxEntries 是正数，不会溢出
		return nil, rejectf("zip 包中的条目过多（%d 个，上限 %d）", n, lim.maxEntries())
	}
	zr, err := zip.NewReader(r, size)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) { // 不安全的路径由下面逐条检查并给出具体文件名
		return nil, errs.Wrap(err, errs.CodeWorkInvalid, "不是有效的 zip 文件，或文件已损坏")
	}
	if len(zr.File) > lim.maxEntries() {
		return nil, rejectf("zip 包中的条目过多（%d 个，上限 %d）", len(zr.File), lim.maxEntries())
	}

	files := make([]srcFile, 0, len(zr.File))
	archiveRoot := ""
	for _, f := range zr.File {
		name, err := zipName(f)
		if err != nil {
			return nil, err
		}
		if f.Flags&0x1 != 0 {
			return nil, rejectf("不支持加密的 zip 包：%s", name)
		}
		if strings.HasSuffix(name, "/") || f.Mode().IsDir() {
			// 目录条目不产生文件，但同样拒绝不安全的路径：出现 "../" 本身就说明这个包不可信
			if dir := strings.TrimRight(name, "/"); dir != "" && dir != "." {
				if _, err := normPath(dir); err != nil {
					return nil, err
				}
			}
			continue
		}
		if f.UncompressedSize64 > bombMinSize && f.UncompressedSize64/max(f.CompressedSize64, 1) > bombRatio {
			return nil, rejectf("文件的压缩比异常（%d 倍），疑似 zip 炸弹：%s", f.UncompressedSize64/max(f.CompressedSize64, 1), name)
		}
		if subdir != nil {
			clean, err := normPath(name)
			if err != nil {
				return nil, err
			}
			top, rest, ok := strings.Cut(clean, "/")
			if !ok || (archiveRoot != "" && archiveRoot != top) {
				return nil, rejectf("GitHub 归档应只有一个顶层目录")
			}
			archiveRoot = top
			name = rest
			if *subdir != "" {
				name, ok = strings.CutPrefix(rest, *subdir+"/")
				if !ok {
					continue
				}
			}
		}
		files = append(files, srcFile{
			path: name, size: clampInt64(f.UncompressedSize64), mode: f.Mode().Type(),
			open: func() (io.ReadCloser, error) {
				rc, err := f.Open()
				if errors.Is(err, zip.ErrAlgorithm) {
					return nil, rejectf("不支持的压缩方式：%s（请使用 zip 默认的 Deflate 压缩重新打包）", name)
				}
				return rc, err
			},
		})
	}
	return collect(files, lim)
}

// zipName 返回条目名。没有 UTF-8 标记、且不是合法 UTF-8 的名称按 GB18030 解码——
// 中文 Windows 自带的"压缩文件夹"及一些旧工具用 GBK 写文件名。
func zipName(f *zip.File) (string, error) {
	if utf8.ValidString(f.Name) || !f.NonUTF8 {
		return f.Name, nil
	}
	name, err := simplifiedchinese.GB18030.NewDecoder().String(f.Name)
	if err != nil || !utf8.ValidString(name) {
		return "", rejectf("zip 中的文件名编码无法识别：%q（请用支持 UTF-8 文件名的工具重新打包）", f.Name)
	}
	return name, nil
}

// zipEntryCount 从目录结束记录读出条目总数，在 zip.NewReader 把整个中央目录读进内存之前拒绝条目数异常的包。
// 读不出来时返回 false，由 zip.NewReader 报告格式错误。解析方式与 archive/zip 一致。
func zipEntryCount(r io.ReaderAt, size int64) (uint64, bool) {
	const (
		endLen      = 22 // 目录结束记录（不含注释）
		end64LocLen = 20 // zip64 目录结束定位记录
		end64Len    = 56 // zip64 目录结束记录
	)
	if size < endLen {
		return 0, false
	}
	n := min(size, endLen+math.MaxUint16)
	buf := make([]byte, n)
	if _, err := r.ReadAt(buf, size-n); err != nil && !errors.Is(err, io.EOF) {
		return 0, false
	}
	i := -1
	for j := len(buf) - endLen; j >= 0; j-- {
		if string(buf[j:j+4]) == "PK\x05\x06" && j+endLen+int(binary.LittleEndian.Uint16(buf[j+20:])) <= len(buf) {
			i = j
			break
		}
	}
	if i < 0 {
		return 0, false
	}
	count := uint64(binary.LittleEndian.Uint16(buf[i+10:]))
	dirSize, dirOffset := binary.LittleEndian.Uint32(buf[i+12:]), binary.LittleEndian.Uint32(buf[i+16:])
	if count != 0xffff && dirSize != 0xffffffff && dirOffset != 0xffffffff {
		return count, true
	}
	// zip64：定位记录紧挨在目录结束记录之前
	locAt := size - n + int64(i) - end64LocLen
	if locAt < 0 {
		return count, true
	}
	loc := make([]byte, end64LocLen)
	if _, err := r.ReadAt(loc, locAt); err != nil || string(loc[:4]) != "PK\x06\x07" {
		return count, true // 没有 zip64 记录：archive/zip 同样按 32 位字段处理
	}
	at := binary.LittleEndian.Uint64(loc[8:])
	if size < end64Len || at > uint64(size-end64Len) {
		return 0, false
	}
	rec := make([]byte, end64Len)
	if _, err := r.ReadAt(rec, int64(at)); err != nil || string(rec[:4]) != "PK\x06\x06" { //nolint:gosec // 上面已确认 at < size
		return 0, false
	}
	return binary.LittleEndian.Uint64(rec[32:]), true
}

func clampInt64(u uint64) int64 {
	if u > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(u)
}
