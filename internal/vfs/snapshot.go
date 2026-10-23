package vfs

import (
	"context"
	"slices"
)

// Snapshot 固定挂载点与文件清单，复制可写记忆的内容；只读来源保留按内容寻址的读取能力。
// 必须在工具执行批次之间调用，返回的表只用于读取，可以与之后的工具执行并发使用。
func (t *Table) Snapshot(ctx context.Context) (*Table, []File, int, error) {
	var mounts []Mount
	var files []File
	size := 0
	for _, m := range t.mounts {
		if m.Source == nil {
			mounts = append(mounts, m)
			continue
		}
		list, err := m.Source.Files(ctx)
		if err != nil {
			return nil, nil, 0, err
		}
		s := &snapshotSource{files: list, source: m.Source}
		if _, writable := m.Source.(Writer); writable {
			s.data = map[string][]byte{}
			for _, f := range list {
				if f.IsDir {
					continue
				}
				b, err := m.Source.Open(ctx, f)
				if err != nil {
					return nil, nil, 0, err
				}
				s.data[f.Path] = slices.Clone(b)
				size += len(b)
			}
			s.source = nil
		}
		for _, f := range list {
			size += len(f.Path) + len(f.Summary) + 128
		}
		files = append(files, list...)
		mounts = append(mounts, Mount{Root: m.Root, Title: m.Title, Source: s})
	}
	slices.SortFunc(files, func(a, b File) int {
		if a.Path < b.Path {
			return -1
		}
		if a.Path > b.Path {
			return 1
		}
		return 0
	})
	return NewTable(mounts...), files, size, nil
}

type snapshotSource struct {
	files  []File
	source Source
	data   map[string][]byte
}

func (s *snapshotSource) Files(context.Context) ([]File, error) { return s.files, nil }
func (s *snapshotSource) Open(ctx context.Context, f File) ([]byte, error) {
	if s.data != nil {
		return slices.Clone(s.data[f.Path]), nil
	}
	return s.source.Open(ctx, f)
}
