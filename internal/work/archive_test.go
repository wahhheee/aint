package work

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/wahhheee/aint/internal/pkg/errs"
)

const minimalManifest = "format: 1\nid: test.import\nversion: 0.1.0\ntitle: 导入测试\n"

// zf 是测试用 zip 条目。
type zf struct {
	name    string
	data    string
	mode    fs.FileMode // 0 表示普通文件
	nonUTF8 bool
}

func makeZip(t *testing.T, files ...zf) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate, NonUTF8: f.nonUTF8}
		if f.mode != 0 {
			h.SetMode(f.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readZip(t *testing.T, lim Limits, files ...zf) (*FileSet, error) {
	t.Helper()
	b := makeZip(t, files...)
	return FromZip(bytes.NewReader(b), int64(len(b)), lim)
}

// wantReject 断言作品包被拒绝，且错误信息含 want。
func wantReject(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("应拒绝（%s），却通过了", want)
	}
	if !errs.Is(err, errs.CodeWorkInvalid) || !strings.Contains(err.Error(), want) {
		t.Fatalf("应因“%s”被拒绝，得到：%v", want, err)
	}
}

func TestZipRootAndIgnore(t *testing.T) {
	set, err := readZip(t, DefaultLimits,
		zf{name: "misty/"},
		zf{name: "misty/work.yaml", data: minimalManifest},
		zf{name: "misty/lore/a.md", data: "甲"},
		zf{name: "misty/drafts/old.md", data: "草稿"},
		zf{name: "misty/art/cover.psd", data: "psd"},
		zf{name: "misty/.aintignore", data: "drafts/\n*.psd\n"},
		zf{name: "misty/.git/config", data: "[core]"},
		zf{name: "misty/.DS_Store", data: "x"},
		zf{name: "misty/node_modules/.bin/tool", data: "../tool/cli.js", mode: fs.ModeSymlink | 0o777}, // 被忽略的符号链接无妨
		zf{name: "__MACOSX/misty/._work.yaml", data: "resource fork"},
		zf{name: "说明.txt", data: "作品根之外的文件"},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(set.Paths(), ",")
	if want := ".aintignore,lore/a.md,work.yaml"; got != want {
		t.Fatalf("文件集应为 %s，得到 %s", want, got)
	}
}

func TestZipRootLocation(t *testing.T) {
	if _, err := readZip(t, DefaultLimits, zf{name: "work.yaml", data: minimalManifest}, zf{name: "lore/a.md"}); err != nil {
		t.Fatalf("根目录有 work.yaml 应可导入：%v", err)
	}
	_, err := readZip(t, DefaultLimits, zf{name: "a/work.yaml", data: minimalManifest}, zf{name: "b/work.yaml", data: minimalManifest})
	wantReject(t, err, "没有找到 work.yaml")
	_, err = readZip(t, DefaultLimits, zf{name: "a/b/work.yaml", data: minimalManifest})
	wantReject(t, err, "没有找到 work.yaml")
}

func TestZipUnsafePaths(t *testing.T) {
	for _, name := range []string{"../evil.txt", "misty/../../evil.txt", `..\..\evil.txt`, "/etc/passwd", `C:\Windows\evil.txt`, "a/\x01b.txt"} {
		t.Run(name, func(t *testing.T) {
			_, err := readZip(t, DefaultLimits, zf{name: "work.yaml", data: minimalManifest}, zf{name: name, data: "x"})
			if err == nil || !errs.Is(err, errs.CodeWorkInvalid) {
				t.Fatalf("应拒绝不安全的路径 %q，得到 %v", name, err)
			}
		})
	}
	// 目录条目同样检查；被忽略的目录中出现 ".." 也拒绝
	_, err := readZip(t, DefaultLimits, zf{name: "work.yaml", data: minimalManifest}, zf{name: "../up/"})
	wantReject(t, err, "..")
	_, err = readZip(t, DefaultLimits, zf{name: "work.yaml", data: minimalManifest}, zf{name: ".git/../../evil"})
	wantReject(t, err, "..")
}

func TestZipSpecialFiles(t *testing.T) {
	_, err := readZip(t, DefaultLimits,
		zf{name: "work.yaml", data: minimalManifest},
		zf{name: "lore/link.md", data: "/etc/passwd", mode: fs.ModeSymlink | 0o777})
	wantReject(t, err, "符号链接")
	_, err = readZip(t, DefaultLimits,
		zf{name: "work.yaml", data: minimalManifest},
		zf{name: "dev/null", mode: fs.ModeDevice | fs.ModeCharDevice | 0o666})
	wantReject(t, err, "特殊文件")
}

func TestZipLimits(t *testing.T) {
	base := zf{name: "work.yaml", data: minimalManifest}
	lim := Limits{MaxTotal: 1 << 20, MaxFiles: 3, MaxFileSize: 256, MaxDepth: 3}

	_, err := readZip(t, lim, base, zf{name: "a.md"}, zf{name: "b.md"}, zf{name: "c.md"})
	wantReject(t, err, "文件数超过上限")

	_, err = readZip(t, lim, base, zf{name: "big.md", data: strings.Repeat("字", 100)})
	wantReject(t, err, "文件过大")

	small := Limits{MaxTotal: 200, MaxFiles: 10, MaxFileSize: 150, MaxDepth: 3}
	_, err = readZip(t, small, base, zf{name: "a.md", data: strings.Repeat("a", 100)}, zf{name: "b.md", data: strings.Repeat("b", 100)})
	wantReject(t, err, "总体积超过上限")

	_, err = readZip(t, lim, base, zf{name: "a/b/c/d.md"})
	wantReject(t, err, "层级过深")

	// 被忽略的文件不计入文件数
	if _, err := readZip(t, lim, base, zf{name: "a.md"}, zf{name: ".git/1"}, zf{name: ".git/2"}, zf{name: ".git/3"}); err != nil {
		t.Fatalf("被忽略的文件不应计入限制：%v", err)
	}
}

func TestZipEntryCount(t *testing.T) {
	// 条目数在读取中央目录之前检查（含将被忽略的条目）
	lim := Limits{MaxTotal: 1 << 20, MaxFiles: 2, MaxFileSize: 1 << 10, MaxDepth: 4}
	files := []zf{{name: "work.yaml", data: minimalManifest}}
	for i := range lim.maxEntries() {
		files = append(files, zf{name: ".git/objects/" + strings.Repeat("x", i+1)})
	}
	_, err := readZip(t, lim, files...)
	wantReject(t, err, "条目过多")

	b := makeZip(t, files[:5]...)
	if n, ok := zipEntryCount(bytes.NewReader(b), int64(len(b))); !ok || n != 5 {
		t.Fatalf("条目数应为 5，得到 %d（%v）", n, ok)
	}
}

func TestZipBomb(t *testing.T) {
	zeros := strings.Repeat("\x00", 4<<20) // 4 MiB 的 0，压缩后只有几 KiB
	_, err := readZip(t, DefaultLimits, zf{name: "work.yaml", data: minimalManifest}, zf{name: "assets/bomb.png", data: zeros})
	wantReject(t, err, "压缩比异常")
}

func TestZipCorrupt(t *testing.T) {
	_, err := FromZip(bytes.NewReader([]byte("not a zip")), 9, DefaultLimits)
	wantReject(t, err, "不是有效的 zip")

	// 内容被篡改：CRC 校验失败
	b := makeZip(t, zf{name: "work.yaml", data: minimalManifest}, zf{name: "lore/a.md", data: strings.Repeat("甲乙丙", 50)})
	i := bytes.Index(b, []byte("lore/a.md")) + len("lore/a.md") + 4
	b[i] ^= 0xff
	_, err = FromZip(bytes.NewReader(b), int64(len(b)), DefaultLimits)
	wantReject(t, err, "读取文件失败")
}

func TestNFCConflict(t *testing.T) {
	nfc, nfd := "lore/が.md", "lore/か\u3099.md"
	_, err := readZip(t, DefaultLimits, zf{name: "work.yaml", data: minimalManifest}, zf{name: nfc}, zf{name: nfd})
	wantReject(t, err, "路径相同")

	// 只有 NFD 写法时规范化为 NFC
	set, err := readZip(t, DefaultLimits, zf{name: "work.yaml", data: minimalManifest}, zf{name: nfd, data: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has(nfc) {
		t.Fatalf("文件名应规范化为 NFC：%q", set.Paths())
	}
}

func TestPathConflict(t *testing.T) {
	_, err := readZip(t, DefaultLimits, zf{name: "work.yaml", data: minimalManifest}, zf{name: "lore"}, zf{name: "lore/a.md"})
	wantReject(t, err, "既是文件又是目录")
}

func TestZipGBKNames(t *testing.T) {
	gbk, err := simplifiedchinese.GBK.NewEncoder().String("命令/调查.md")
	if err != nil {
		t.Fatal(err)
	}
	set, err := readZip(t, DefaultLimits, zf{name: "work.yaml", data: minimalManifest}, zf{name: gbk, data: "x", nonUTF8: true})
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("命令/调查.md") {
		t.Fatalf("GBK 文件名应被识别：%q", set.Paths())
	}
}

func TestLFSPointer(t *testing.T) {
	pointer := "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393\nsize 12345\n"
	set, err := readZip(t, DefaultLimits,
		zf{name: "work.yaml", data: minimalManifest},
		zf{name: "assets/cover.png", data: pointer},
		zf{name: "state.yaml", data: "fields:\n  day: {type: nope}\n"})
	if err != nil {
		t.Fatal(err)
	}
	_, rep := Compile(set)
	lfs, state := false, false
	for _, i := range rep.Issues {
		lfs = lfs || (i.Code == CodeLFS && i.Path == "assets/cover.png" && i.Level == LevelError)
		state = state || i.Code == CodeState
	}
	if !lfs {
		t.Fatalf("应报告 LFS 指针文件：%+v", rep.Issues)
	}
	if !state {
		t.Fatalf("有 LFS 错误时其余检查应照常进行：%+v", rep.Issues)
	}

	// work.yaml 本身是指针文件：只报 E-LFS，不再报 YAML 解析失败
	set, err = readZip(t, DefaultLimits, zf{name: "work.yaml", data: pointer})
	if err != nil {
		t.Fatal(err)
	}
	if _, rep := Compile(set); len(rep.Issues) != 1 || rep.Issues[0].Code != CodeLFS {
		t.Fatalf("work.yaml 是指针文件时应只报 E-LFS：%+v", rep.Issues)
	}
	if isLFSPointer([]byte("version https://git-lfs.github.com/spec/v1 只是一段正文")) {
		t.Fatal("没有 oid 的文本不是指针文件")
	}
}

func TestFolderUpload(t *testing.T) {
	lim := Limits{MaxTotal: 1 << 20, MaxFiles: 10, MaxFileSize: 64, MaxDepth: 4}
	add := func(u *FolderUpload, p, data string) {
		t.Helper()
		if err := u.Add(p, strings.NewReader(data)); err != nil {
			t.Fatalf("Add(%s)：%v", p, err)
		}
	}

	u := NewFolderUpload(lim)
	add(u, "misty/work.yaml", minimalManifest[:60])
	add(u, "misty/lore/a.md", "甲")
	add(u, "misty/.git/objects/ab", strings.Repeat("x", 1000)) // 始终忽略：不读取、不计数
	add(u, "misty/raw/master.wav", strings.Repeat("x", 1000))  // 超过单文件上限，但被 .aintignore 忽略
	add(u, "misty/.aintignore", "raw/\n")
	set, err := u.FileSet()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(set.Paths(), ","); got != ".aintignore,lore/a.md,work.yaml" {
		t.Fatalf("文件集不对：%s", got)
	}
	if u.Folder() != "misty" {
		t.Fatalf("文件夹名应为 misty，得到 %q", u.Folder())
	}

	u = NewFolderUpload(lim)
	add(u, "misty/work.yaml", "x")
	add(u, "misty/assets/big.png", strings.Repeat("x", 1000))
	_, err = u.FileSet()
	wantReject(t, err, "文件过大")

	u = NewFolderUpload(lim)
	wantReject(t, u.Add("../misty/work.yaml", strings.NewReader("x")), "..")
}

func TestFromFSLocatesRoot(t *testing.T) {
	fsys := fstest.MapFS{
		"misty/work.yaml":      {Data: []byte(minimalManifest)},
		"misty/lore/a.md":      {Data: []byte("甲")},
		"misty/.git/HEAD":      {Data: []byte("ref")},
		"misty/assets/link.md": {Data: []byte("x"), Mode: fs.ModeSymlink},
	}
	_, err := FromFS(fsys, DefaultLimits)
	wantReject(t, err, "符号链接")

	delete(fsys, "misty/assets/link.md")
	set, err := FromFS(fsys, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(set.Paths(), ","); got != "lore/a.md,work.yaml" {
		t.Fatalf("文件集不对：%s", got)
	}
}
