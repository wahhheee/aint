package work

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"sync"

	"github.com/yuin/goldmark"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/ids"
	"github.com/wahhheee/yanyi/internal/store"
)

// Source 记录作品版本的来源（work_versions.source）。
type Source struct {
	Type   string `json:"type"`            // zip | dir | github | url | dev | builtin
	Name   string `json:"name,omitzero"`   // builtin 名称、zip 文件名
	Path   string `json:"path,omitzero"`   // dir / dev 的本机路径
	Repo   string `json:"repo,omitzero"`   // github：owner/repo
	Ref    string `json:"ref,omitzero"`    // github：分支/标签/提交
	Commit string `json:"commit,omitzero"` // github：实际提交 SHA
	Subdir string `json:"subdir,omitzero"` // github：子目录
	URL    string `json:"url,omitzero"`    // url
}

// Service 是作品服务。并发安全。
type Service struct {
	db    *store.DB
	blobs *store.Blobs

	mu    sync.Mutex
	cache map[string]*Model // 版本 ID → 编译结果（TODO(M1)：改为 LRU，限制内存）
}

// NewService 创建作品服务。
func NewService(db *store.DB, blobs *store.Blobs) *Service {
	return &Service{db: db, blobs: blobs, cache: map[string]*Model{}}
}

// ImportResult 是导入结果。
type ImportResult struct {
	WorkID    string  `json:"work_id"`
	VersionID string  `json:"version_id"`
	Created   bool    `json:"created"` // false 表示同内容版本已存在，直接复用
	Report    *Report `json:"report"`
}

// ImportFileSet 是所有导入来源的公共后半段：编译 → 校验 → 去重 → 入库。
func (s *Service) ImportFileSet(ctx context.Context, set *FileSet, src Source) (*ImportResult, error) {
	m, rep := Compile(set)
	if rep.HasErrors() {
		n, _ := rep.Count()
		return nil, errs.New(errs.CodeWorkInvalid, "作品有 %d 处错误，无法导入", n).WithDetails(rep)
	}
	res := &ImportResult{WorkID: m.Manifest.ID, Report: rep}

	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		existing, err := findVersionByHash(ctx, tx, m.Manifest.ID, m.TreeHash)
		if err != nil {
			return err
		}
		if existing != "" {
			res.VersionID = existing
			return nil
		}
		res.VersionID, res.Created = ids.New(), true
		// works 行需先于 work_versions 存在（外键）
		if err := upsertWork(ctx, tx, m.Manifest.ID, m.Manifest.Title, res.VersionID); err != nil {
			return err
		}
		if err := insertVersion(ctx, tx, versionRow{
			ID: res.VersionID, WorkID: m.Manifest.ID, Version: m.Manifest.Version, TreeHash: m.TreeHash,
			IsDev: src.Type == "dev", Source: src, Manifest: m.Manifest, Report: *rep, ImportedAt: store.NowMillis(),
		}); err != nil {
			return err
		}
		for _, p := range set.Paths() {
			data, _ := set.Get(p)
			hash, err := s.blobs.Put(ctx, tx, data, "")
			if err != nil {
				return err
			}
			if err := insertFile(ctx, tx, res.VersionID, p, hash, len(data)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("写入作品版本失败: %w", err)
	}
	if res.Created {
		slog.InfoContext(ctx, "已导入作品版本", "work_id", res.WorkID, "version_id", res.VersionID,
			"version", m.Manifest.Version, "source", src.Type)
	}
	return res, nil
}

// EnsureBuiltins 导入内置示例作品（每次启动执行；内容未变时只是一次哈希比对）。
// fsys 的每个顶层目录是一部作品。单部作品失败只记日志，不影响启动。
func (s *Service) EnsureBuiltins(ctx context.Context, fsys fs.FS) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		slog.WarnContext(ctx, "读取内置作品失败", "err", err)
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, _ := fs.Sub(fsys, e.Name())
		set, err := FromFS(sub, DefaultLimits)
		if err == nil {
			_, err = s.ImportFileSet(ctx, set, Source{Type: "builtin", Name: e.Name()})
		}
		if err != nil {
			slog.WarnContext(ctx, "导入内置作品失败", "name", e.Name(), "err", err)
			if be, ok := errs.As(err); ok {
				if rep, ok := be.Details.(*Report); ok {
					for _, i := range rep.Issues {
						slog.WarnContext(ctx, "  "+i.String())
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 查询（API 输出结构直接使用下列类型，带 json 标签）

// VersionInfo 是版本摘要。
type VersionInfo struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	Source     Source `json:"source"`
	IsDev      bool   `json:"is_dev"`
	ImportedAt int64  `json:"imported_at"`
	Errors     int    `json:"errors"`
	Warnings   int    `json:"warnings"`
}

// Summary 是书架上的作品卡片。
type Summary struct {
	ID            string      `json:"id"`
	Title         string      `json:"title"`
	Subtitle      string      `json:"subtitle,omitzero"`
	Description   string      `json:"description,omitzero"`
	Tags          []string    `json:"tags"`
	ContentRating string      `json:"content_rating,omitzero"`
	Authors       []Author    `json:"authors"`
	Latest        VersionInfo `json:"latest"`
	SaveCount     int         `json:"save_count"`
	UpdatedAt     int64       `json:"updated_at"`
	// TODO(M2-10)：CoverURL（封面素材 → /api/v1/assets/{hash}）；目前前端用标题生成排版封面。
}

// Detail 是作品详情页。
type Detail struct {
	Summary
	License    string        `json:"license,omitzero"`
	Requires   Requires      `json:"requires"`
	Narrator   NarratorConf  `json:"narrator"`
	NewGame    NewGame       `json:"new_game"`
	Changelog  string        `json:"changelog,omitzero"`
	ReadmeHTML string        `json:"readme_html,omitzero"`
	Versions   []VersionInfo `json:"versions"`
	Report     *Report       `json:"report"`
}

func versionInfo(v versionRow) VersionInfo {
	e, w := v.Report.Count()
	return VersionInfo{ID: v.ID, Version: v.Version, Source: v.Source, IsDev: v.IsDev, ImportedAt: v.ImportedAt, Errors: e, Warnings: w}
}

func summaryOf(w workRow, v versionRow) Summary {
	mf := v.Manifest
	return Summary{
		ID: w.ID, Title: mf.Title, Subtitle: mf.Subtitle, Description: mf.Description,
		Tags: nonNil(mf.Tags), ContentRating: mf.ContentRating, Authors: nonNil(mf.Authors),
		Latest: versionInfo(v), SaveCount: w.SaveCount, UpdatedAt: w.UpdatedAt,
	}
}

// List 返回全部作品（按更新时间倒序）。
func (s *Service) List(ctx context.Context) ([]Summary, error) {
	rows, err := listWorks(ctx, s.db.Read())
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(rows))
	for _, w := range rows {
		v, err := getVersion(ctx, s.db.Read(), w.LatestVersionID)
		if err != nil {
			return nil, fmt.Errorf("读取作品 %s 的最新版本失败: %w", w.ID, err)
		}
		out = append(out, summaryOf(w, v))
	}
	return out, nil
}

// Get 返回作品详情。
func (s *Service) Get(ctx context.Context, workID string) (*Detail, error) {
	w, err := getWork(ctx, s.db.Read(), workID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.NotFound("作品不存在：%s", workID)
	}
	if err != nil {
		return nil, err
	}
	versions, err := listVersions(ctx, s.db.Read(), workID)
	if err != nil {
		return nil, err
	}
	var latest versionRow
	d := &Detail{Versions: []VersionInfo{}}
	for _, v := range versions {
		d.Versions = append(d.Versions, versionInfo(v))
		if v.ID == w.LatestVersionID {
			latest = v
		}
	}
	d.Summary = summaryOf(w, latest)
	mf := latest.Manifest
	d.License, d.Requires, d.Narrator, d.NewGame, d.Changelog = mf.License, mf.Requires, mf.Narrator, mf.NewGame, mf.Changelog
	d.Report = &latest.Report

	if m, err := s.Model(ctx, latest.ID); err == nil && m.Readme != "" {
		var buf bytes.Buffer
		// goldmark 默认不输出原始 HTML（安全）；前端展示前仍会经 DOMPurify 净化。
		if err := goldmark.Convert([]byte(m.Readme), &buf); err == nil {
			d.ReadmeHTML = buf.String()
		}
	}
	return d, nil
}

// Model 返回作品版本的编译结果（带缓存）。
func (s *Service) Model(ctx context.Context, versionID string) (*Model, error) {
	s.mu.Lock()
	m, ok := s.cache[versionID]
	s.mu.Unlock()
	if ok {
		return m, nil
	}

	v, err := getVersion(ctx, s.db.Read(), versionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.NotFound("作品版本不存在：%s", versionID)
	}
	if err != nil {
		return nil, err
	}
	files, err := versionFiles(ctx, s.db.Read(), versionID)
	if err != nil {
		return nil, err
	}
	set := NewFileSet()
	for _, f := range files {
		data, err := s.blobs.Get(ctx, s.db.Read(), f.BlobHash)
		if err != nil {
			return nil, err
		}
		if err := set.Add(f.Path, data); err != nil {
			return nil, err
		}
	}
	m, rep := Compile(set)
	if rep.HasErrors() {
		// 入库时校验通过，此处仍报错说明编译器规则变严了（程序升级）；带上报告便于排查。
		return nil, errs.New(errs.CodeWorkInvalid, "作品版本无法加载").WithDetails(rep)
	}
	m.VersionID, m.WorkID = v.ID, v.WorkID

	s.mu.Lock()
	s.cache[versionID] = m
	s.mu.Unlock()
	return m, nil
}

// LatestVersionID 返回作品的最新版本 ID。
func (s *Service) LatestVersionID(ctx context.Context, workID string) (string, error) {
	w, err := getWork(ctx, s.db.Read(), workID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errs.NotFound("作品不存在：%s", workID)
	}
	return w.LatestVersionID, err
}

// AssetData 读取作品素材内容。只返回作品文件引用过的 blob；不存在时返回 nil。
func (s *Service) AssetData(ctx context.Context, hash string) ([]byte, error) {
	var one int
	err := s.db.Read().QueryRowContext(ctx, `SELECT 1 FROM work_files WHERE blob_hash = ? LIMIT 1`, hash).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.blobs.Get(ctx, s.db.Read(), hash)
}

// AssetURL 返回素材的访问地址。
func AssetURL(a Asset) string {
	return "/api/v1/assets/" + strings.TrimPrefix(a.Blob, "sha256:")
}

// TODO(M1-4/M6-1)：Import(ctx, ImportRequest) 两段式导入（先解析校验返回报告与任务 ID，确认后写入）：
//   - zip：archive/zip 安全解压（路径穿越、符号链接、体积与数量上限）→ LocateRoot → FromFS；
//   - 目录：os.DirFS(path) → LocateRoot → FromFS；
//   - GitHub：链接解析、ref 歧义处理、codeload 归档下载、LFS 检测（技术方案 5.4）；
//   - URL：下载大小上限、超时（P1）。
// TODO(M6-2)：开发模式（dev_path 原地更新 + fsnotify 去抖 400ms + 推送 work.reloaded）。
// TODO(M6-5)：Export（确定性 zip）、Delete（有存档引用时提示）。

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
