package vfs

import (
	"context"
	"sync"
)

// MemorySource 是 /memory 挂载点的来源：以回合覆盖层为准——本回合写入的内容立即可读，
// 删除的文件立即消失（写入由 M3-2 实现）；未改动的文件按清单中的 blob 读取。
type MemorySource struct {
	overlay *Overlay
	load    func(ctx context.Context, blob string) ([]byte, error)

	mu        sync.Mutex
	summaries map[string]string // blob → 一行说明（内容寻址，结果不会过期）
}

// NewMemorySource 创建记忆来源。load 按 blob 哈希读取内容。
func NewMemorySource(o *Overlay, load func(ctx context.Context, blob string) ([]byte, error)) *MemorySource {
	return &MemorySource{overlay: o, load: load, summaries: map[string]string{}}
}

var _ Source = (*MemorySource)(nil)

// Files 返回全部记忆文件，附带一行说明（frontmatter summary 或首行，12.6）。
func (s *MemorySource) Files(ctx context.Context) ([]File, error) {
	paths := s.overlay.Paths()
	out := make([]File, 0, len(paths))
	for _, p := range paths {
		ref, data, _ := s.overlay.Get(p)
		f := File{Path: p, Size: ref.Size, Blob: ref.Blob}
		sum, err := s.summary(ctx, ref.Blob, data)
		if err != nil {
			return nil, err
		}
		f.Summary = sum
		out = append(out, f)
	}
	return out, nil
}

// Open 返回文件内容。
func (s *MemorySource) Open(ctx context.Context, f File) ([]byte, error) {
	ref, data, ok := s.overlay.Get(f.Path)
	if !ok {
		return nil, &PathError{Path: f.Path, Err: ErrNotFound}
	}
	if data != nil {
		return data, nil
	}
	return s.load(ctx, ref.Blob)
}

func (s *MemorySource) summary(ctx context.Context, blob string, data []byte) (string, error) {
	s.mu.Lock()
	sum, ok := s.summaries[blob]
	s.mu.Unlock()
	if ok {
		return sum, nil
	}
	if data == nil {
		var err error
		if data, err = s.load(ctx, blob); err != nil {
			return "", err
		}
	}
	if isText(data) {
		sum = Summary(string(data))
	}
	s.mu.Lock()
	s.summaries[blob] = sum
	s.mu.Unlock()
	return sum, nil
}
