package work

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/fsx"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/store"
)

// ImportTTL 是暂存的导入的有效期：超时未确认则删除暂存文件。
const ImportTTL = 30 * time.Minute

// maxStagedImports 是同时暂存的导入数上限：超出时丢弃最早的一个（玩家一次只会确认一部作品）。
const maxStagedImports = 8

// ImportPreview 是导入第一步（解析与校验，不写入作品库）的结果，供玩家确认。
type ImportPreview struct {
	Task      string          `json:"task,omitzero"`       // 确认导入时使用；报告有 error 时为空（不能导入）
	ExpiresAt int64           `json:"expires_at,omitzero"` // 毫秒时间戳，过期后需重新选择作品
	Source    Source          `json:"source"`
	Work      ImportWork      `json:"work"`
	TreeHash  string          `json:"tree_hash"`
	Files     int             `json:"files"`
	Size      int64           `json:"size"`
	Existing  *ImportExisting `json:"existing,omitzero"` // 书架上已有同 ID 作品时
	Report    *Report         `json:"report"`
}

// ImportWork 是待导入作品的摘要（来自 work.yaml；清单有错误时可能不完整）。
type ImportWork struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle,omitzero"`
	Version       string   `json:"version"`
	Description   string   `json:"description,omitzero"`
	Tags          []string `json:"tags"`
	ContentRating string   `json:"content_rating,omitzero"`
	Authors       []Author `json:"authors"`
	Requires      Requires `json:"requires"`
}

// ImportExisting 是书架上已有的同 ID 作品。
type ImportExisting struct {
	Title         string `json:"title"`
	LatestVersion string `json:"latest_version"`
	// 内容完全相同（文件树哈希一致）的已有版本：确认后直接复用，不产生新版本
	SameVersionID string `json:"same_version_id,omitzero"`
	SameVersion   string `json:"same_version,omitzero"`
}

// Importer 是两段式导入（《04-交互与界面设计》5：解析阶段只读取与校验，不写入；确认后才导入）。
//
//	Preview*：来源 → FileSet → 编译校验 → 暂存文件（无 error 时）→ 预览与任务 ID
//	Confirm：读回暂存文件 → 核对文件树哈希 → Service.ImportFileSet（同作品同哈希复用旧版本）
//
// 暂存文件按内容哈希命名（不使用作品中的文件名：大小写不敏感的文件系统、Windows 保留名都不会出问题），
// 任务只登记在内存中：过期（ImportTTL）、取消或确认后删除；程序启动时清空整个暂存目录。
type Importer struct {
	remote *remoteSource
	works  *Service
	dir    string
	lim    Limits
	ttl    time.Duration

	mu    sync.Mutex
	tasks map[string]*importTask
}

type importTask struct {
	id       string
	dir      string
	src      Source
	treeHash string
	files    []stagedFile
	expires  time.Time
	timer    *time.Timer
	busy     bool // 正在确认
	dead     bool // 确认期间过期或被取消：确认结束后删除
}

type stagedFile struct {
	path string // 作品中的路径
	name string // 暂存文件名（内容哈希）
}

// NewImporter 创建导入器。dir 是专用的暂存目录，会被清空（上次运行的任务只登记在内存中，无法再确认）。
func NewImporter(works *Service, dir string, lim Limits) (*Importer, error) {
	if err := os.RemoveAll(dir); err != nil {
		return nil, fmt.Errorf("清空导入暂存目录失败: %w", err)
	}
	if err := os.MkdirAll(dir, fsx.DirPerm); err != nil {
		return nil, fmt.Errorf("创建导入暂存目录失败: %w", err)
	}
	return &Importer{works: works, dir: dir, lim: lim, ttl: ImportTTL, tasks: map[string]*importTask{}, remote: newRemoteSource()}, nil
}

// Limits 返回导入的安全限制。
func (im *Importer) Limits() Limits { return im.lim }

// NewFolderUpload 创建一次文件夹上传。
func (im *Importer) NewFolderUpload() *FolderUpload { return NewFolderUpload(im.lim) }

// PreviewZip 是 zip 导入的第一步：把上传内容写入暂存目录的临时文件，再安全地读取、编译、校验。
func (im *Importer) PreviewZip(ctx context.Context, name string, r io.Reader) (*ImportPreview, error) {
	f, err := os.CreateTemp(im.dir, "upload-*.zip")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close() // 只读过的临时文件，关闭错误无影响
		if err := os.Remove(f.Name()); err != nil {
			slog.WarnContext(ctx, "删除上传的临时 zip 失败", "path", f.Name(), "err", err)
		}
	}()
	limit := im.lim.UploadLimit()
	n, err := io.Copy(f, io.LimitReader(r, limit+1))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidArgument, "接收上传的 zip 包失败")
	}
	if n > limit {
		return nil, rejectf("zip 包过大（作品总体积上限 %d MiB）", im.lim.MaxTotal>>20)
	}
	set, err := FromZip(f, n, im.lim)
	if err != nil {
		return nil, err
	}
	return im.preview(ctx, set, Source{Type: "zip", Name: sourceName(name)})
}

// PreviewFolder 是文件夹导入的第一步。name 是所选文件夹的名称，为空时取上传路径的第一段。
func (im *Importer) PreviewFolder(ctx context.Context, u *FolderUpload, name string) (*ImportPreview, error) {
	set, err := u.FileSet()
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = u.Folder()
	}
	return im.preview(ctx, set, Source{Type: "dir", Name: sourceName(name)})
}

func (im *Importer) preview(ctx context.Context, set *FileSet, src Source) (*ImportPreview, error) {
	m, rep := im.works.compile(set)
	mf := m.Manifest
	p := &ImportPreview{
		Source: src, TreeHash: m.TreeHash, Files: set.Len(), Size: set.Size(), Report: rep,
		Work: ImportWork{
			ID: mf.ID, Title: mf.Title, Subtitle: mf.Subtitle, Version: mf.Version, Description: mf.Description,
			Tags: nonNil(mf.Tags), ContentRating: mf.ContentRating, Authors: nonNil(mf.Authors), Requires: mf.Requires,
		},
	}
	if reWorkID.MatchString(mf.ID) {
		ex, err := im.works.existing(ctx, mf.ID, m.TreeHash)
		if err != nil {
			return nil, err
		}
		p.Existing = ex
	}
	if rep.HasErrors() {
		return p, nil // 有 error 不暂存：不能确认导入
	}
	t, err := im.stage(ctx, set, src, m.TreeHash)
	if err != nil {
		return nil, err
	}
	p.Task, p.ExpiresAt = t.id, t.expires.UnixMilli()
	slog.InfoContext(ctx, "作品已解析，等待确认导入", "task", t.id, "work_id", mf.ID, "version", mf.Version,
		"files", p.Files, "size", p.Size, "source", src.Type)
	return p, nil
}

// stage 把文件集写入暂存目录并登记任务。
func (im *Importer) stage(ctx context.Context, set *FileSet, src Source, treeHash string) (t *importTask, err error) {
	t = &importTask{id: ids.New(), src: src, treeHash: treeHash}
	t.dir = filepath.Join(im.dir, t.id)
	if err := os.MkdirAll(t.dir, fsx.DirPerm); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			im.removeDir(ctx, t)
		}
	}()
	written := map[string]bool{}
	for _, p := range set.Paths() {
		data, _ := set.Get(p)
		name := strings.TrimPrefix(store.HashBytes(data), "sha256:")
		if !written[name] {
			if err := os.WriteFile(filepath.Join(t.dir, name), data, fsx.PrivateFilePerm); err != nil {
				return nil, fmt.Errorf("写入导入暂存文件失败: %w", err)
			}
			written[name] = true
		}
		t.files = append(t.files, stagedFile{path: p, name: name})
	}

	im.mu.Lock()
	t.expires = time.Now().Add(im.ttl)
	t.timer = time.AfterFunc(im.ttl, func() { im.discard(context.Background(), t.id) })
	im.tasks[t.id] = t
	oldest := im.oldestIdle(t.id)
	im.mu.Unlock()
	if oldest != "" {
		im.discard(ctx, oldest)
	}
	return t, nil
}

// oldestIdle 在暂存数超出上限时返回最早的空闲任务（调用方持有锁）。
func (im *Importer) oldestIdle(except string) string {
	if len(im.tasks) <= maxStagedImports {
		return ""
	}
	var idle []*importTask
	for _, t := range im.tasks {
		if !t.busy && t.id != except {
			idle = append(idle, t)
		}
	}
	if len(idle) == 0 {
		return ""
	}
	return slices.MinFunc(idle, func(a, b *importTask) int { return a.expires.Compare(b.expires) }).id
}

// Confirm 是导入的第二步：把暂存的作品写入作品库。同作品同文件树哈希的版本已存在时直接复用。
// 失败时（如数据库暂时不可写）保留暂存，可以重试。
func (im *Importer) Confirm(ctx context.Context, id string) (*ImportResult, error) {
	t, err := im.claim(id)
	if err != nil {
		return nil, err
	}
	res, err := im.commit(ctx, t)
	im.release(ctx, t, err == nil)
	return res, err
}

func (im *Importer) claim(id string) (*importTask, error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	t := im.tasks[id]
	if t == nil || t.dead {
		return nil, errs.NotFound("这次导入已过期或已完成，请重新选择作品")
	}
	if t.busy {
		return nil, errs.Conflict("这部作品正在导入，请稍候")
	}
	t.busy = true
	return t, nil
}

func (im *Importer) commit(ctx context.Context, t *importTask) (*ImportResult, error) {
	set := NewFileSet()
	for _, f := range t.files {
		data, err := os.ReadFile(filepath.Join(t.dir, f.name))
		if err != nil {
			return nil, fmt.Errorf("读取导入暂存文件失败: %w", err)
		}
		if err := set.Add(f.path, data); err != nil {
			return nil, err
		}
	}
	if h := set.TreeHash(); h != t.treeHash {
		return nil, fmt.Errorf("暂存的作品文件与解析时不一致：%s ≠ %s", h, t.treeHash)
	}
	return im.works.ImportFileSet(ctx, set, t.src)
}

func (im *Importer) release(ctx context.Context, t *importTask, done bool) {
	im.mu.Lock()
	t.busy = false
	remove := done || t.dead
	if remove {
		delete(im.tasks, t.id)
		t.timer.Stop()
	}
	im.mu.Unlock()
	if remove {
		im.removeDir(ctx, t)
	}
}

// Cancel 放弃导入并删除暂存文件。任务不存在时什么也不做；正在确认时等确认结束后删除。
func (im *Importer) Cancel(ctx context.Context, id string) { im.discard(ctx, id) }

func (im *Importer) discard(ctx context.Context, id string) {
	im.mu.Lock()
	t := im.tasks[id]
	if t == nil {
		im.mu.Unlock()
		return
	}
	if t.busy {
		t.dead = true
		im.mu.Unlock()
		return
	}
	delete(im.tasks, id)
	t.timer.Stop()
	im.mu.Unlock()
	im.removeDir(ctx, t)
}

func (im *Importer) removeDir(ctx context.Context, t *importTask) {
	if err := os.RemoveAll(t.dir); err != nil {
		slog.WarnContext(ctx, "删除导入暂存文件失败（下次启动时清理）", "task", t.id, "err", err)
	}
}

// sourceName 取上传文件或文件夹的名称（只保留最后一段），用于版本来源的展示。
func sourceName(name string) string {
	name = strings.TrimSpace(path.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "." || name == "/" {
		return ""
	}
	if r := []rune(name); len(r) > 120 {
		name = string(r[:120])
	}
	return name
}
