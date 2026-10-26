package work

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/store"
)

// PreviewDirectory 从服务所在电脑读取目录，确认前只暂存快照。
func (im *Importer) PreviewDirectory(ctx context.Context, dir string, dev bool) (*ImportPreview, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errs.Invalid("请填写目录路径")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, errs.Invalid("目录路径不正确")
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, errs.Invalid("无法打开目录，请填写服务所在电脑上的路径")
	}
	set, err := FromFS(os.DirFS(abs), im.lim)
	if err != nil {
		return nil, err
	}
	typ := "dir"
	if dev {
		typ = "dev"
	}
	return im.preview(ctx, set, Source{Type: typ, Path: abs, Name: filepath.Base(abs)})
}

func (s *Service) importDev(ctx context.Context, set *FileSet, src Source, expectedID string) (*ImportResult, error) {
	m, rep := s.compile(set)
	if expectedID != "" && expectedID != m.Manifest.ID {
		rep.Errorf(CodeManifest, "work.yaml", 0, "新作品请单独链接", "开发目录的作品 ID 不能改变")
	}
	if rep.HasErrors() {
		return nil, errs.New(errs.CodeWorkInvalid, "开发目录校验未通过，保留上次可用内容").WithDetails(rep)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res := &ImportResult{WorkID: m.Manifest.ID, Report: rep}
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		var linked string
		err := tx.QueryRowContext(ctx, `SELECT COALESCE(dev_path, '') FROM works WHERE id = ?`, m.Manifest.ID).Scan(&linked)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if expectedID != "" && linked != src.Path {
			return errs.Conflict("开发目录已解除或更换链接")
		}
		if linked != "" && linked != src.Path {
			return errs.Conflict("此作品已经链接了另一个开发目录：%s", linked)
		}
		err = tx.QueryRowContext(ctx, `SELECT id FROM work_versions WHERE work_id = ? AND is_dev = 1`, m.Manifest.ID).Scan(&res.VersionID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if res.VersionID == "" {
			res.VersionID, res.Created = ids.New(), true
		}
		if expectedID == "" || res.Created {
			if err := upsertWork(ctx, tx, m.Manifest.ID, m.Manifest.Title, res.VersionID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE works SET dev_path = ?, updated_at = ? WHERE id = ?`, src.Path, store.NowMillis(), m.Manifest.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE works SET title = ? WHERE id = ? AND latest_version_id = ?`, m.Manifest.Title, m.Manifest.ID, res.VersionID); err != nil {
			return err
		}
		v := versionRow{ID: res.VersionID, WorkID: m.Manifest.ID, Version: m.Manifest.Version, TreeHash: m.TreeHash, IsDev: true, Source: src, Manifest: m.Manifest, Report: *rep, ImportedAt: store.NowMillis()}
		if res.Created {
			if err := insertVersion(ctx, tx, v); err != nil {
				return err
			}
		} else {
			mf, err := jsonx.Marshal(v.Manifest)
			if err != nil {
				return err
			}
			rp, err := jsonx.Marshal(rep)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE work_versions SET version = ?, tree_hash = ?, manifest = ?, report = ?, imported_at = ? WHERE id = ?`, v.Version, v.TreeHash, string(mf), string(rp), v.ImportedAt, v.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM work_files WHERE version_id = ?`, v.ID); err != nil {
				return err
			}
		}
		for _, p := range set.Paths() {
			b, _ := set.Get(p)
			hash, err := s.blobs.Put(ctx, tx, b, "")
			if err != nil {
				return err
			}
			if err := insertFile(ctx, tx, v.ID, p, hash, len(b)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	m.WorkID, m.VersionID, m.IsDev = m.Manifest.ID, res.VersionID, true
	s.cache[res.VersionID] = m
	return res, nil
}

type devLink struct {
	id, path, version, hash string
	report                  Report
}

func (s *Service) devLinks(ctx context.Context) ([]devLink, error) {
	rows, err := s.db.Read().QueryContext(ctx, `SELECT w.id, w.dev_path, v.id, v.tree_hash, v.report FROM works w JOIN work_versions v ON v.work_id = w.id AND v.is_dev = 1 WHERE w.dev_path IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []devLink
	for rows.Next() {
		var link devLink
		var report string
		if err := rows.Scan(&link.id, &link.path, &link.version, &link.hash, &report); err != nil {
			return nil, err
		}
		if err := jsonx.Unmarshal([]byte(report), &link.report); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Service) reloadDev(ctx context.Context, link devLink) *Report {
	set, err := FromFS(os.DirFS(link.path), DefaultLimits)
	if err == nil && set.TreeHash() == link.hash && !link.report.HasErrors() {
		return nil
	}
	var rep *Report
	if err == nil {
		var res *ImportResult
		res, err = s.importDev(ctx, set, Source{Type: "dev", Path: link.path, Name: filepath.Base(link.path)}, link.id)
		if err == nil {
			return res.Report
		}
	}
	if e, ok := errs.As(err); ok {
		rep, _ = e.Details.(*Report)
	}
	if rep == nil {
		rep = &Report{Issues: []Issue{}}
		rep.Errorf(CodeManifest, "work.yaml", 0, "保留上次可用内容", "%v", err)
	}
	if err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		b, err := jsonx.Marshal(rep)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE work_versions SET report = ? WHERE id = ?`, string(b), link.version)
		return err
	}); err != nil {
		slog.WarnContext(ctx, "保存开发目录校验报告失败", "err", err)
	}
	return rep
}

// WatchDevelopment 递归监听开发目录，400 ms 去抖后重载；返回关闭并等待退出的函数。
// 每秒同步链接与目录监听，覆盖新建子目录、目录重命名和重启后的链接恢复。
func (s *Service) WatchDevelopment(parent context.Context, publish func(string, *Report)) (func(), error) {
	w, err := fsnotify.NewBufferedWatcher(1024)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = w.Close() }()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		defer timer.Stop()
		var links []devLink
		known := map[string]string{}
		dirty := map[string]bool{}
		syncLinks := func() {
			var err error
			links, err = s.devLinks(ctx)
			if err != nil {
				if ctx.Err() == nil {
					slog.WarnContext(ctx, "读取开发目录失败", "err", err)
				}
				return
			}
			wanted := map[string]bool{}
			active := map[string]bool{}
			for _, link := range links {
				active[link.id] = true
				if known[link.id] != link.path {
					dirty[link.id] = true
					known[link.id] = link.path
					timer.Reset(400 * time.Millisecond)
				}
				count := 0
				err := filepath.WalkDir(link.path, func(p string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !d.IsDir() {
						return nil
					}
					if p != link.path && slices.Contains(alwaysIgnored, d.Name()) {
						return fs.SkipDir
					}
					count++
					if count > DefaultLimits.maxEntries() {
						return fs.SkipAll
					}
					wanted[p] = true
					return nil
				})
				if err != nil {
					dirty[link.id] = true
					timer.Reset(400 * time.Millisecond)
				}
			}
			for id := range known {
				if !active[id] {
					delete(known, id)
					delete(dirty, id)
				}
			}
			for _, p := range w.WatchList() {
				if !wanted[p] {
					_ = w.Remove(p)
				}
				delete(wanted, p)
			}
			for p := range wanted {
				if err := w.Add(p); err != nil {
					slog.WarnContext(ctx, "监听开发目录失败", "path", p, "err", err)
				}
			}
		}
		syncLinks()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				syncLinks()
			case e, ok := <-w.Events:
				if !ok {
					return
				}
				if e.Op == fsnotify.Chmod {
					continue
				}
				for _, link := range links {
					if rel, err := filepath.Rel(link.path, e.Name); err == nil && (rel == "." || filepath.IsLocal(rel)) {
						dirty[link.id] = true
					}
				}
				timer.Reset(400 * time.Millisecond)
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.WarnContext(ctx, "开发目录监听异常", "err", err)
				for _, link := range links {
					dirty[link.id] = true
				}
				timer.Reset(400 * time.Millisecond)
			case <-timer.C:
				if current, err := s.devLinks(ctx); err == nil {
					links = current
				}
				for _, link := range links {
					if dirty[link.id] {
						if rep := s.reloadDev(ctx, link); rep != nil {
							publish(link.id, rep)
						}
					}
				}
				clear(dirty)
			}
		}
	}()
	return sync.OnceFunc(func() { cancel(); <-done }), nil
}
