package work

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"hash/crc32"
	"io"
	"time"
)

// WriteZip 输出确定性的作品包：路径排序、固定时间与权限，不包含宿主文件属性。
// 高度可压缩的大文件使用 Store，使导出包也满足导入器的压缩比限制。
func WriteZip(w io.Writer, set *FileSet) error {
	zw := zip.NewWriter(w)
	for _, p := range set.Paths() {
		data, _ := set.Get(p)
		var compressed bytes.Buffer
		fw, err := flate.NewWriter(&compressed, flate.DefaultCompression)
		if err != nil {
			return err
		}
		if _, err := fw.Write(data); err != nil {
			return err
		}
		if err := fw.Close(); err != nil {
			return err
		}
		method, payload := zip.Deflate, compressed.Bytes()
		if len(payload) >= len(data) || (len(data) > bombMinSize && len(data)/max(len(payload), 1) > bombRatio) {
			method, payload = zip.Store, data
		}
		h := &zip.FileHeader{Name: p, Flags: 0x800, Method: method, CRC32: crc32.ChecksumIEEE(data),
			CompressedSize64: uint64(len(payload)), UncompressedSize64: uint64(len(data))}
		h.Modified = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
		h.ModifiedDate = 33 //nolint:staticcheck // CreateRaw 不从 Modified 转换 DOS 日期；确定性归档必须显式填写两个时间表示。
		h.SetMode(0o644)
		entry, err := zw.CreateRaw(h)
		if err != nil {
			return err
		}
		if _, err := entry.Write(payload); err != nil {
			return err
		}
	}
	return zw.Close()
}
