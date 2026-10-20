package vfs

import (
	"maps"
	"slices"
)

// FileRef 是记忆文件在清单中的引用（与 save.MemoryFile 字段一致；本包不依赖 save 以保持层次）。
type FileRef struct {
	Blob string
	Size int
	Turn int
}

// Overlay 是记忆的写时复制覆盖层：回合内的写操作只进覆盖层，
// 回合提交时生成新清单，回合被丢弃则写操作作废（12.2）。不并发安全（工具写操作串行执行）。
type Overlay struct {
	base    map[string]FileRef
	changed map[string]FileRef
	removed map[string]bool
	content map[string][]byte // 本回合新写入的内容（提交时写 blob）
}

// NewOverlay 以父节点的记忆清单为底创建覆盖层。base 不会被修改。
func NewOverlay(base map[string]FileRef) *Overlay {
	return &Overlay{base: base, changed: map[string]FileRef{}, removed: map[string]bool{}, content: map[string][]byte{}}
}

// Get 返回文件引用与本回合写入的内容（若有）。
func (o *Overlay) Get(p string) (ref FileRef, data []byte, ok bool) {
	if o.removed[p] {
		return FileRef{}, nil, false
	}
	if r, ok := o.changed[p]; ok {
		return r, o.content[p], true
	}
	r, ok := o.base[p]
	return r, nil, ok
}

// Put 写入文件（内容哈希由调用方计算，提交时写 blob）。
func (o *Overlay) Put(p string, data []byte, blob string, turn int) {
	delete(o.removed, p)
	o.changed[p] = FileRef{Blob: blob, Size: len(data), Turn: turn}
	o.content[p] = data
}

// Remove 删除文件。
func (o *Overlay) Remove(p string) {
	delete(o.changed, p)
	delete(o.content, p)
	if _, ok := o.base[p]; ok {
		o.removed[p] = true
	}
}

// Dirty 表示本回合是否改过记忆（未改时节点直接复用父节点的 memory_hash）。
func (o *Overlay) Dirty() bool { return len(o.changed) > 0 || len(o.removed) > 0 }

// Paths 返回当前全部文件路径（已排序）。
func (o *Overlay) Paths() []string {
	all := maps.Clone(o.base)
	maps.Copy(all, o.changed)
	for p := range o.removed {
		delete(all, p)
	}
	return slices.Sorted(maps.Keys(all))
}

// Result 返回合并后的清单与本回合新写入的内容（供提交）。
func (o *Overlay) Result() (files map[string]FileRef, written map[string][]byte) {
	files = maps.Clone(o.base)
	maps.Copy(files, o.changed)
	for p := range o.removed {
		delete(files, p)
	}
	return files, maps.Clone(o.content)
}
